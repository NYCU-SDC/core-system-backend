package user

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/file"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// GetFromContext extracts the authenticated user from request context
func GetFromContext(ctx context.Context) (*User, bool) {
	userData, ok := ctx.Value(internal.UserContextKey).(*User)
	return userData, ok
}

func (u User) GetID() uuid.UUID {
	return u.ID
}

type Querier interface {
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	Get(ctx context.Context, id uuid.UUID) (UsersWithEmail, error)
	GetIDByAuth(ctx context.Context, arg GetIDByAuthParams) (uuid.UUID, error)
	ExistsByAuth(ctx context.Context, arg ExistsByAuthParams) (bool, error)
	Create(ctx context.Context, arg CreateParams) (User, error)
	CreateAuth(ctx context.Context, arg CreateAuthParams) (Auth, error)
	Update(ctx context.Context, arg UpdateParams) (User, error)
	GetEmails(ctx context.Context, userID uuid.UUID) ([]string, error)
	CreateEmail(ctx context.Context, arg CreateEmailParams) error
	GetIDByEmail(ctx context.Context, value string) (uuid.UUID, error)
	GetWithEarliestProviderByEmail(ctx context.Context, value string) (GetWithEarliestProviderByEmailRow, error)
}

// FileOperator defines the interface for file operations needed by user service
// Following Go best practice: interfaces are defined by the consumer, not the provider
type FileOperator interface {
	SaveFile(ctx context.Context, fileContent io.Reader, originalFilename, contentType string, uploadedBy *uuid.UUID, opts ...file.ValidatorOption) (file.File, error)
	DownloadFromURL(ctx context.Context, url string, filename string, uploadedBy *uuid.UUID, opts ...file.ValidatorOption) (file.File, error)
}

type onboardingChecker interface {
	AllowedOnboarding(email string) bool
}

type Service struct {
	logger            *zap.Logger
	queries           Querier
	tracer            trace.Tracer
	fileOperator      FileOperator
	orgWriter         OrgMemberWriter
	orgResolver       OrgSlugResolver
	onboardingChecker onboardingChecker
}

type Profile struct {
	ID        uuid.UUID
	Name      string
	Username  string
	AvatarURL string
	Emails    []string
}

type OrgMemberWriter interface {
	AddMemberWithRole(
		ctx context.Context,
		unitID uuid.UUID,
		memberID uuid.UUID,
		role string,
	) error
}

type OrgSlugResolver interface {
	GetOrgIDBySlug(ctx context.Context, slug string) (uuid.UUID, error)
}

func NewService(logger *zap.Logger, db DBTX, fileOperator FileOperator, orgWriter OrgMemberWriter, orgResolver OrgSlugResolver, checker onboardingChecker) *Service {
	return &Service{
		logger:            logger,
		queries:           New(db),
		tracer:            otel.Tracer("user/service"),
		fileOperator:      fileOperator,
		orgWriter:         orgWriter,
		orgResolver:       orgResolver,
		onboardingChecker: checker,
	}
}

func (s *Service) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	traceCtx, span := s.tracer.Start(ctx, "Exists")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exists, err := s.queries.Exists(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get user by id")
		span.RecordError(err)
		return false, err
	}
	return exists, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (UsersWithEmail, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	user, err := s.queries.Get(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get user by id")
		span.RecordError(err)
		return UsersWithEmail{}, err
	}
	return user, nil
}

func resolveAvatarUrl(name, avatarUrl string) string {
	if avatarUrl == "" {
		return "https://ui-avatars.com/api/?name=" + url.QueryEscape(name)
	}
	return avatarUrl
}

// FindOrCreateResult is the result of FindOrCreate.
// If ExistingProvider is non-empty, it means a different provider already has the same email,
// and the caller should trigger the account binding confirmation flow.
// In that case, ExistingName, ExistingProvider, ExistingProviderID, and UserID are populated.
// Otherwise, UserID is set and the Existing* fields are empty.
type FindOrCreateResult struct {
	UserID             uuid.UUID
	ExistingName       string
	ExistingProvider   string
	ExistingProviderID string
}

func (s *Service) FindOrCreate(ctx context.Context, name, username, avatarUrl string, email string, role []string, oauthProvider, oauthProviderID string) (FindOrCreateResult, error) {
	traceCtx, span := s.tracer.Start(ctx, "FindOrCreate")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Same provider returning user → direct login
	exists, err := s.queries.ExistsByAuth(traceCtx, ExistsByAuthParams{
		Provider:   oauthProvider,
		ProviderID: oauthProviderID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check user existence by auth")
		span.RecordError(err)
		return FindOrCreateResult{}, err
	}

	if exists {
		existingUserID, err := s.queries.GetIDByAuth(traceCtx, GetIDByAuthParams{
			Provider:   oauthProvider,
			ProviderID: oauthProviderID,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "get user by auth")
			span.RecordError(err)
			return FindOrCreateResult{}, err
		}

		logger.Debug("Returning user via same provider", zap.String("provider", oauthProvider), zap.String("user_id", existingUserID.String()))
		return FindOrCreateResult{UserID: existingUserID}, nil
	}

	// Same email as an existing user: either first OAuth for a pre-provisioned user,
	// or different provider, same email → binding confirmation required
	if email != "" {
		existingUser, err := s.queries.GetWithEarliestProviderByEmail(traceCtx, email)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				err = databaseutil.WrapDBError(err, logger, "check user existence by email")
				span.RecordError(err)
				return FindOrCreateResult{}, err
			}
			// No user with this email — fall through to create a new user below.
		} else {
			// Does not create a new user if the user has been initialized
			if !existingUser.Provider.Valid {
				logger.Info("User has been initialized",
					zap.String("name", name),
					zap.String("email", email))
				_, err = s.queries.CreateAuth(traceCtx, CreateAuthParams{
					UserID:     existingUser.ID,
					Provider:   oauthProvider,
					ProviderID: oauthProviderID,
				})
				if err != nil {
					err = databaseutil.WrapDBError(err, logger, "create auth for pre-provisioned user")
					span.RecordError(err)
					return FindOrCreateResult{}, err
				}
				return FindOrCreateResult{UserID: existingUser.ID}, nil
			}
		}
		if err == nil {
			// Found a user with the same email under a different provider
			logger.Info("Email already exists under different provider, binding confirmation required",
				zap.String("name", existingUser.Name.String),
				zap.String("email", email),
				zap.String("existing_provider", existingUser.Provider.String),
				zap.String("new_provider", oauthProvider),
			)
			return FindOrCreateResult{
				UserID:             existingUser.ID,
				ExistingName:       existingUser.Name.String,
				ExistingProvider:   existingUser.Provider.String,
				ExistingProviderID: existingUser.ProviderID.String,
			}, nil
		}
	}

	// User not exists -> create new user
	logger.Info("User not found, creating new user", zap.String("provider", oauthProvider), zap.String("provider_id", oauthProviderID))

	defaultRoles := DefaultGlobalRoles(email)

	roleSet := map[string]struct{}{}

	for _, r := range role {
		roleSet[r] = struct{}{}
	}

	for _, r := range defaultRoles {
		roleSet[r] = struct{}{}
	}

	var finalRoles []string
	for r := range roleSet {
		finalRoles = append(finalRoles, r)
	}

	if len(finalRoles) == 0 {
		finalRoles = []string{"user"}
	}

	logger.Info("Final roles for new user", zap.Strings("roles", finalRoles))

	// Create user first with a placeholder avatar
	placeholderAvatar := resolveAvatarUrl(name, "")
	newUser, err := s.queries.Create(traceCtx, CreateParams{
		Name: pgtype.Text{String: name, Valid: name != ""},
		//Username:  pgtype.Text{String: username, Valid: username != ""},
		AvatarUrl: pgtype.Text{String: placeholderAvatar, Valid: true},
		Role:      finalRoles,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create user")
		span.RecordError(err)
		return FindOrCreateResult{}, err
	}

	logger.Info("Created new user", zap.String("user_id", newUser.ID.String()), zap.String("username", newUser.Username.String))

	// Create email entry
	err = s.CreateEmail(traceCtx, newUser.ID, email)
	if err != nil {
		span.RecordError(err)
		return FindOrCreateResult{}, err
	}

	// Create auth entry
	_, err = s.queries.CreateAuth(traceCtx, CreateAuthParams{
		UserID:     newUser.ID,
		Provider:   oauthProvider,
		ProviderID: oauthProviderID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create auth")
		span.RecordError(err)
		return FindOrCreateResult{}, err
	}

	// Try to download and save avatar if provided
	if avatarUrl != "" && s.fileOperator != nil {
		backendAvatarURL := s.downloadAndSaveAvatar(traceCtx, avatarUrl, newUser.ID)
		if backendAvatarURL != "" {
			// Update user with backend avatar URL
			_, err = s.queries.Update(traceCtx, UpdateParams{
				ID:   newUser.ID,
				Name: newUser.Name,
				// Todo: Disable username update for now, need to implement invalidation for username
				//Username:  newUser.Username,
				AvatarUrl: pgtype.Text{String: backendAvatarURL, Valid: true},
			})
			if err != nil {
				// Log warning but don't fail the user creation
				logger.Warn("Failed to update user avatar URL after download",
					zap.String("user_id", newUser.ID.String()),
					zap.Error(err))
			}
		}
	}

	defaultOrgRole, ok := DefaultOrgRole(email)

	if ok && s.orgWriter != nil && s.orgResolver != nil {
		const defaultOrgSlug = "SDC"
		defaultOrgID, resolveErr := s.orgResolver.GetOrgIDBySlug(traceCtx, defaultOrgSlug)
		if resolveErr != nil {
			logger.Warn("failed to resolve default org slug",
				zap.String("slug", defaultOrgSlug),
				zap.Error(resolveErr))
		} else {
			err := s.orgWriter.AddMemberWithRole(
				traceCtx,
				defaultOrgID,
				newUser.ID,
				defaultOrgRole,
			)

			if err != nil {
				logger.Warn("failed to apply default org role",
					zap.Error(err))
			}
		}
	}

	return FindOrCreateResult{UserID: newUser.ID}, nil
}

func (s *Service) FindOrCreateByEmail(ctx context.Context, email string, globalRole []string) (uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "FindOrCreateByEmail")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	id, err := s.queries.GetIDByEmail(traceCtx, email)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			err = databaseutil.WrapDBError(err, logger, "get user id existence by email")
			span.RecordError(err)
			return uuid.UUID{}, err
		}
		// No user with this email
		logger.Info("User not found, creating new user", zap.String("email", email))

		defaultRoles := DefaultGlobalRoles(email)

		roleSet := map[string]struct{}{}

		for _, r := range globalRole {
			roleSet[r] = struct{}{}
		}

		for _, r := range defaultRoles {
			roleSet[r] = struct{}{}
		}

		var finalRoles []string
		for r := range roleSet {
			finalRoles = append(finalRoles, r)
		}

		if len(finalRoles) == 0 {
			finalRoles = []string{"user"}
		}

		logger.Info("Final global roles for new user", zap.Strings("roles", finalRoles))

		newUser, err := s.queries.Create(traceCtx, CreateParams{AvatarUrl: pgtype.Text{String: "", Valid: true}, Role: finalRoles})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "create user")
			span.RecordError(err)
			return uuid.UUID{}, err
		}
		logger.Info("Created new user", zap.String("user_id", newUser.ID.String()))

		err = s.queries.CreateEmail(traceCtx, CreateEmailParams{
			UserID: newUser.ID,
			Value:  email})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "create email")
			span.RecordError(err)
			return uuid.UUID{}, err
		}

		return newUser.ID, nil
	}

	logger.Debug("Found existing user", zap.String("user_id", id.String()))
	span.RecordError(err)
	return id, nil
}

// downloadAndSaveAvatar downloads an avatar from a URL and saves it to the file service
// Returns the backend URL for the saved avatar, or empty string if failed
func (s *Service) downloadAndSaveAvatar(ctx context.Context, avatarURL string, userID uuid.UUID) string {
	logger := logutil.WithContext(ctx, s.logger)

	// Skip if no avatar URL
	if avatarURL == "" {
		return ""
	}

	// Skip if already a backend URL
	if strings.HasPrefix(avatarURL, "/api/files/") {
		return avatarURL
	}

	// Generate filename for avatar
	filename := fmt.Sprintf("avatar-%s", userID.String())

	// Build validation options for avatar images
	const maxAvatarSize = 5 * 1024 * 1024 // 5MB
	validationOpts := []file.ValidatorOption{
		file.WithMaxSize(maxAvatarSize),
		file.WithImageFormats(), // Accept JPEG, PNG, or WebP
	}

	// Use file service to download and save avatar
	savedFile, err := s.fileOperator.DownloadFromURL(ctx, avatarURL, filename, &userID, validationOpts...)
	if err != nil {
		logger.Warn("Failed to download and save avatar",
			zap.String("url", avatarURL),
			zap.Error(err))
		return ""
	}

	// Return backend URL
	backendURL := fmt.Sprintf("/api/files/%s", savedFile.ID.String())
	logger.Info("Successfully downloaded and saved avatar",
		zap.String("original_url", avatarURL),
		zap.String("backend_url", backendURL),
		zap.String("user_id", userID.String()))

	return backendURL
}

// CreateAuth validates that the given userID actually owns the existingProvider/existingProviderID
// entry before creating the new auth record, preventing callers from arbitrarily
// linking a provider to a user they do not control.
func (s *Service) CreateAuth(ctx context.Context, userID uuid.UUID, provider, providerID, existingProvider, existingProviderID string) error {
	traceCtx, span := s.tracer.Start(ctx, "CreateAuth")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	if existingProvider == "" && existingProviderID == "" {
		_, err := s.queries.CreateAuth(traceCtx, CreateAuthParams{
			UserID:     userID,
			Provider:   provider,
			ProviderID: providerID,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "create auth")
			span.RecordError(err)
			return err
		}
	}
	// Verify the target user actually owns the claimed existing auth entry.
	ownerID, err := s.queries.GetIDByAuth(traceCtx, GetIDByAuthParams{
		Provider:   existingProvider,
		ProviderID: existingProviderID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "verify existing auth ownership")
		span.RecordError(err)
		return err
	}
	if ownerID != userID {
		return internal.ErrInvalidAuthUser
	}

	_, err = s.queries.CreateAuth(traceCtx, CreateAuthParams{
		UserID:     userID,
		Provider:   provider,
		ProviderID: providerID,
	})
	if err != nil {
		if errors.Is(err, databaseutil.ErrUniqueViolation) {
			logger.Info("The auth entry of the user already exists", zap.Error(err))
			return nil
		}
		err = databaseutil.WrapDBError(err, logger, "create auth")
		span.RecordError(err)
		return err
	}

	logger.Info("Created auth entry", zap.String("user_id", userID.String()), zap.String("provider", provider))
	return nil
}

func (s *Service) CreateEmail(ctx context.Context, userID uuid.UUID, email string) error {
	traceCtx, span := s.tracer.Start(ctx, "CreateEmail")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Create email record
	err := s.queries.CreateEmail(traceCtx, CreateEmailParams{
		UserID: userID,
		Value:  email,
	})
	if err != nil {
		// Log the specific error for debugging
		logger.Error("Failed to create email record",
			zap.String("user_id", userID.String()),
			zap.String("email", email),
			zap.Error(err))

		err = databaseutil.WrapDBError(err, logger, "create email")
		span.RecordError(err)
		return err
	}

	logger.Info("Successfully created email record",
		zap.String("user_id", userID.String()),
		zap.String("email", email))
	return nil
}

func (s *Service) GetEmails(ctx context.Context, userID uuid.UUID) ([]string, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetEmailsByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	emails, err := s.queries.GetEmails(traceCtx, userID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get emails by user id")
		span.RecordError(err)

		return nil, err
	}

	return emails, nil
}

func (s *Service) Onboarding(ctx context.Context, id uuid.UUID, name, username string) (User, error) {
	traceCtx, span := s.tracer.Start(ctx, "Onboarding")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	userInfo, err := s.queries.Get(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get user by id")
		span.RecordError(err)
		return User{}, err
	}

	userEmails, err := s.GetEmails(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get user emails by id")
		span.RecordError(err)
		return User{}, err
	}
	isAllowed := false
	for _, userEmail := range userEmails {
		if s.onboardingChecker.AllowedOnboarding(userEmail) {
			isAllowed = true
			break
		}
	}
	if !isAllowed {
		err := internal.ErrUserNotInAllowedList
		logger.Warn(fmt.Sprintf("%s: user_id=%s", err.Error(), id.String()))
		return User{}, err
	}

	if userInfo.IsOnboarded {
		err := internal.ErrUserOnboarded
		logger.Warn(fmt.Sprintf("%s: user_id=%s", err.Error(), id.String()))
		return User{}, err
	}
	user, err := s.queries.Update(traceCtx, UpdateParams{
		ID: id,
		Name: pgtype.Text{
			String: name,
			Valid:  name != "",
		},
		Username: pgtype.Text{
			String: username,
			Valid:  username != "",
		},
		AvatarUrl:   userInfo.AvatarUrl,
		IsOnboarded: true,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update user information")
		if errors.Is(err, databaseutil.ErrUniqueViolation) {
			return User{}, internal.ErrUsernameConflict
		}
		span.RecordError(err)
		return User{}, err
	}
	return user, nil
}

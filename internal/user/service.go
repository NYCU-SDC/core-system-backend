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
	GetByAuth(ctx context.Context, arg GetByAuthParams) (uuid.UUID, error)
	ExistsByAuth(ctx context.Context, arg ExistsByAuthParams) (bool, error)
	Create(ctx context.Context, arg CreateParams) (User, error)
	CreateWithID(ctx context.Context, arg CreateWithIDParams) (User, error)
	CreateAuth(ctx context.Context, arg CreateAuthParams) (Auth, error)
	Update(ctx context.Context, arg UpdateParams) (User, error)
	GetEmails(ctx context.Context, userID uuid.UUID) ([]string, error)
	UpsertEmail(ctx context.Context, arg UpsertEmailParams) (uuid.UUID, error)
	GetByEmail(ctx context.Context, value string) (uuid.UUID, error)
	GetByEmailForUpdate(ctx context.Context, value string) (uuid.UUID, error)
	GetWithEarliestProviderByEmail(ctx context.Context, value string) (GetWithEarliestProviderByEmailRow, error)
	GetLoginProfile(ctx context.Context, userID uuid.UUID) ([]byte, error)
	WithTx(tx pgx.Tx) *Queries
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
	db                DBTX
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
		db:                db,
		queries:           New(db),
		tracer:            otel.Tracer("user/service"),
		fileOperator:      fileOperator,
		orgWriter:         orgWriter,
		orgResolver:       orgResolver,
		onboardingChecker: checker,
	}
}

func (s *Service) WithTx(tx pgx.Tx) *Service {
	return &Service{
		logger:            s.logger,
		db:                tx,
		queries:           s.queries.WithTx(tx),
		tracer:            s.tracer,
		fileOperator:      s.fileOperator,
		orgWriter:         s.orgWriter,
		orgResolver:       s.orgResolver,
		onboardingChecker: s.onboardingChecker,
	}
}

// withTransaction runs fn inside a pgx transaction. If s.db is already a pgx.Tx, fn
// runs on that transaction without begin/commit/rollback. Otherwise s.db must
// implement TxBeginner.
func (s *Service) withTransaction(ctx context.Context, fn func(*Queries) error) error {
	return internal.WithTransaction(ctx, s.db, s.logger, func(tx pgx.Tx) error {
		return fn(s.queries.WithTx(tx))
	})
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

func resolveAvatarURL(name, avatarURL string) string {
	if avatarURL == "" {
		return "https://ui-avatars.com/api/?name=" + url.QueryEscape(name)
	}
	return avatarURL
}

// FindOrCreateParams holds OAuth profile data for FindOrCreate.
type FindOrCreateParams struct {
	Name            string
	Username        string
	AvatarURL       string
	Email           string
	Role            []string
	OAuthProvider   string
	OAuthProviderID string
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

func (s *Service) FindOrCreate(ctx context.Context, params FindOrCreateParams) (FindOrCreateResult, error) {
	traceCtx, span := s.tracer.Start(ctx, "FindOrCreate")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	existingUserID, found, err := s.GetByOAuthProvider(traceCtx, params.OAuthProvider, params.OAuthProviderID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check user existence by auth")
		span.RecordError(err)
		return FindOrCreateResult{}, err
	}
	if found {
		logger.Debug("Returning user via same provider", zap.String("provider", params.OAuthProvider), zap.String("user_id", existingUserID.String()))
		return FindOrCreateResult{UserID: existingUserID}, nil
	}

	if params.Email != "" {
		outcome, result, err := s.resolveOAuthByEmail(traceCtx, params)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "check user existence by email")
			span.RecordError(err)
			return FindOrCreateResult{}, err
		}
		switch outcome {
		case linkedEmailUser:
			return result, nil
		case bindingRequired:
			return result, nil
		case emailNotFound:
			// continue to create account
		}
	}

	logger.Info("User not found, creating new user", zap.String("provider", params.OAuthProvider), zap.String("provider_id", params.OAuthProviderID))

	result, createdNew, err := s.createWithAuth(traceCtx, params)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create oauth user")
		span.RecordError(err)
		return FindOrCreateResult{}, err
	}

	if result.ExistingProvider != "" {
		return result, nil
	}

	if createdNew {
		logger.Info("Created new user", zap.String("user_id", result.UserID.String()))
		s.finishSignup(traceCtx, result.UserID, params.AvatarURL, params.Email)
	} else {
		logger.Debug("Returning recovered OAuth user from concurrent signup",
			zap.String("provider", params.OAuthProvider),
			zap.String("user_id", result.UserID.String()))
	}
	return result, nil
}

// FindOrCreateByEmail returns the user ID associated with the given email.
// If the email already exists, it returns the existing user ID.
// If userID is provided and does not match the existing email owner, it returns a conflict error.
// If the email does not exist, it creates a new user and links the email to that user.
// When userID is provided for creation, the user is created with that ID only if it does not already exist.
func (s *Service) FindOrCreateByEmail(ctx context.Context, email string, globalRoles []string, userID *uuid.UUID) (uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "FindOrCreateByEmail")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	existingID, err := s.queries.GetByEmail(traceCtx, email)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			err = databaseutil.WrapDBError(err, logger, "get user id existence by email")
			span.RecordError(err)
			return uuid.UUID{}, err
		}

		finalRoles := buildGlobalRoleSet(globalRoles, email)

		// Email is not registered yet, create a new user with roles.
		id, err := s.createForEmail(traceCtx, email, finalRoles, userID)
		if err != nil {
			// Concurrent create claimed the email — return the winner unless caller requested a different ID.
			if errors.Is(err, internal.ErrEmailConflict) {
				existingID, lookupErr := s.queries.GetByEmail(traceCtx, email)
				if lookupErr == nil {
					if userID != nil && existingID != *userID {
						return uuid.UUID{}, internal.ErrEmailConflict
					}
					return existingID, nil
				}
			}
			span.RecordError(err)
			return uuid.UUID{}, err
		}

		logger.Info(
			"Created new user by email",
			zap.String("email", email),
			zap.String("user_id", id.String()),
			zap.Strings("roles", finalRoles),
		)
		return id, nil
	}

	// Email already exists. If a requested userID is given, it must match the existing owner.
	if userID != nil && existingID != *userID {
		err := internal.ErrEmailConflict
		logger.Warn(
			"email already exists with a different user ID",
			zap.Error(err),
			zap.String("email", email),
			zap.String("existing_user_id", existingID.String()),
			zap.String("requested_user_id", userID.String()),
		)
		span.RecordError(err)
		return uuid.UUID{}, err
	}

	logger.Debug(
		"Found existing user by email",
		zap.String("email", email),
		zap.String("user_id", existingID.String()),
	)

	return existingID, nil
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
		logger.Info("Created auth entry", zap.String("user_id", userID.String()), zap.String("provider", provider))
		return nil
	}
	// Verify the target user actually owns the claimed existing auth entry.
	ownerID, err := s.queries.GetByAuth(traceCtx, GetByAuthParams{
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
		wrapped := databaseutil.WrapDBError(err, logger, "create auth")
		if errors.Is(wrapped, databaseutil.ErrUniqueViolation) {
			logger.Info("The auth entry of the user already exists", zap.Error(err))
			return nil
		}
		span.RecordError(wrapped)
		return wrapped
	}

	logger.Info("Created auth entry", zap.String("user_id", userID.String()), zap.String("provider", provider))
	return nil
}

func (s *Service) CreateEmail(ctx context.Context, userID uuid.UUID, email string) error {
	traceCtx, span := s.tracer.Start(ctx, "CreateEmail")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	_, err := s.queries.UpsertEmail(traceCtx, UpsertEmailParams{
		UserID: userID,
		Value:  email,
	})
	if err != nil {
		logger.Error("Failed to create email record",
			zap.String("user_id", userID.String()),
			zap.String("email", email),
			zap.Error(err))

		err = databaseutil.WrapDBError(err, logger, "create email")
		span.RecordError(err)
		return err
	}

	err = validateEmailOwner(traceCtx, s.queries, email, userID)
	if err != nil {
		logger.Warn("Email belongs to another user",
			zap.String("user_id", userID.String()),
			zap.String("email", email),
			zap.Error(err))
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

func buildGlobalRoleSet(globalRoles []string, email string) []string {
	defaultRoles := DefaultGlobalRoles(email)
	roleSet := map[string]struct{}{}

	for _, r := range globalRoles {
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

	return finalRoles
}

// createForEmail creates an account and links the given email.
// If userID is provided, it first verifies that the requested ID is not already used.
// This function assumes the email does not already belong to another account.
func (s *Service) createForEmail(ctx context.Context, email string, roles []string, userID *uuid.UUID) (uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "createForEmail")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	newUserID, err := s.createWithEmailOnly(traceCtx, email, roles, userID)
	if err != nil {
		if errors.Is(err, internal.ErrUserIDAlreadyExists) {
			logFields := []zap.Field{zap.Error(err), zap.String("email", email)}

			if userID != nil {
				logFields = append(logFields, zap.String("requested_user_id", userID.String()))
			}
			logger.Warn("Requested user ID already exists", logFields...)
			span.RecordError(err)
			return uuid.UUID{}, err
		}
		err = databaseutil.WrapDBError(err, logger, "create user for email")
		span.RecordError(err)

		return uuid.UUID{}, err
	}

	return newUserID, nil
}

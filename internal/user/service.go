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
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
	GetByID(ctx context.Context, id uuid.UUID) (UsersWithEmail, error)
	GetIDByAuth(ctx context.Context, arg GetIDByAuthParams) (uuid.UUID, error)
	ExistsByAuth(ctx context.Context, arg ExistsByAuthParams) (bool, error)
	Create(ctx context.Context, arg CreateParams) (User, error)
	CreateAuth(ctx context.Context, arg CreateAuthParams) (Auth, error)
	Update(ctx context.Context, arg UpdateParams) (User, error)
	GetEmailsByID(ctx context.Context, userID uuid.UUID) ([]string, error)
	CreateEmail(ctx context.Context, arg CreateEmailParams) error
}

// FileOperator defines the interface for file operations needed by user service
// Following Go best practice: interfaces are defined by the consumer, not the provider
type FileOperator interface {
	SaveFile(ctx context.Context, fileContent io.Reader, originalFilename, contentType string, uploadedBy *uuid.UUID, opts ...file.ValidatorOption) (file.File, error)
	DownloadFromURL(ctx context.Context, url string, filename string, uploadedBy *uuid.UUID, opts ...file.ValidatorOption) (file.File, error)
}

type Service struct {
	logger       *zap.Logger
	queries      Querier
	tracer       trace.Tracer
	fileOperator FileOperator
}

type Profile struct {
	ID        uuid.UUID
	Name      string
	Username  string
	AvatarURL string
	Emails    []string
}

func NewService(logger *zap.Logger, db DBTX, fileOperator FileOperator) *Service {
	return &Service{
		logger:       logger,
		queries:      New(db),
		tracer:       otel.Tracer("user/service"),
		fileOperator: fileOperator,
	}
}

func (s *Service) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	traceCtx, span := s.tracer.Start(ctx, "ExistsByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exists, err := s.queries.ExistsByID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get user by id")
		span.RecordError(err)
		return false, err
	}
	return exists, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (UsersWithEmail, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	user, err := s.queries.GetByID(traceCtx, id)
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

func (s *Service) FindOrCreate(ctx context.Context, name, username, avatarUrl string, role []string, oauthProvider, oauthProviderID string) (uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "FindOrCreate")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exists, err := s.queries.ExistsByAuth(traceCtx, ExistsByAuthParams{
		Provider:   oauthProvider,
		ProviderID: oauthProviderID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check user existence by auth")
		span.RecordError(err)
		return uuid.UUID{}, err
	}

	if exists {
		existingUserID, err := s.queries.GetIDByAuth(traceCtx, GetIDByAuthParams{
			Provider:   oauthProvider,
			ProviderID: oauthProviderID,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "get user by auth")
			span.RecordError(err)
			return uuid.UUID{}, err
		}

		// For existing users, only update name and username, keep existing avatar
		_, err = s.queries.Update(traceCtx, UpdateParams{
			ID:       existingUserID,
			Name:     pgtype.Text{String: name, Valid: name != ""},
			Username: pgtype.Text{String: username, Valid: username != ""},
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "update existing user")
			span.RecordError(err)
			return uuid.UUID{}, err
		}

		logger.Debug("Updated existing user", zap.String("provider", oauthProvider), zap.String("provider_id", oauthProviderID), zap.String("user_id", existingUserID.String()))
		return existingUserID, nil
	}

	// User doesn't exist, create new user
	logger.Info("User not found, creating new user", zap.String("provider", oauthProvider), zap.String("provider_id", oauthProviderID))

	if len(role) == 0 {
		role = []string{"user"}
	}

	// Create user first with a placeholder avatar
	placeholderAvatar := resolveAvatarUrl(name, "")
	newUser, err := s.queries.Create(traceCtx, CreateParams{
		Name:      pgtype.Text{String: name, Valid: name != ""},
		Username:  pgtype.Text{String: username, Valid: username != ""},
		AvatarUrl: pgtype.Text{String: placeholderAvatar, Valid: true},
		Role:      role,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create user")
		span.RecordError(err)
		return uuid.UUID{}, err
	}

	logger.Debug("Created new user", zap.String("user_id", newUser.ID.String()), zap.String("username", newUser.Username.String))

	// Create auth entry
	_, err = s.queries.CreateAuth(traceCtx, CreateAuthParams{
		UserID:     newUser.ID,
		Provider:   oauthProvider,
		ProviderID: oauthProviderID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create auth")
		span.RecordError(err)
		return uuid.UUID{}, err
	}

	logger.Debug("Created auth entry", zap.String("user_id", newUser.ID.String()), zap.String("provider", oauthProvider), zap.String("provider_id", oauthProviderID))

	// Try to download and save avatar if provided
	if avatarUrl != "" && s.fileOperator != nil {
		backendAvatarURL := s.downloadAndSaveAvatar(traceCtx, avatarUrl, newUser.ID)
		if backendAvatarURL != "" {
			// Update user with backend avatar URL
			_, err = s.queries.Update(traceCtx, UpdateParams{
				ID:        newUser.ID,
				Name:      newUser.Name,
				Username:  newUser.Username,
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

	return newUser.ID, nil
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

func (s *Service) GetEmailsByID(ctx context.Context, userID uuid.UUID) ([]string, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetEmailsByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	emails, err := s.queries.GetEmailsByID(traceCtx, userID)
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

	userInfo, err := s.queries.GetByID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get user by id")
		span.RecordError(err)
		return User{}, internal.ErrDatabaseError
	}

	userEmails, err := s.GetEmailsByID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get user emails by id")
		span.RecordError(err)
		return User{}, internal.ErrDatabaseError
	}
	isAllowed := false
	for _, userEmail := range userEmails {
		if IsAllowed(userEmail) {
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
		return User{}, internal.ErrDatabaseError
	}
	return user, nil
}

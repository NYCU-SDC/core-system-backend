package user

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"errors"
	"fmt"
	"net/url"
	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

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

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer
}

type Profile struct {
	ID        uuid.UUID
	Name      string
	Username  string
	AvatarURL string
	Emails    []string
}

func NewService(logger *zap.Logger, db DBTX) *Service {
	return &Service{
		logger:  logger,
		queries: New(db),
		tracer:  otel.Tracer("user/service"),
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

		avatarUrl = resolveAvatarUrl(name, avatarUrl)
		_, err = s.queries.Update(traceCtx, UpdateParams{
			ID:        existingUserID,
			Name:      pgtype.Text{String: name, Valid: name != ""},
			Username:  pgtype.Text{String: username, Valid: username != ""},
			AvatarUrl: pgtype.Text{String: avatarUrl, Valid: avatarUrl != ""},
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
	logger.Debug("User not found, creating new user", zap.String("provider", oauthProvider), zap.String("provider_id", oauthProviderID))
	avatarUrl = resolveAvatarUrl(name, avatarUrl)
	if len(role) == 0 {
		role = []string{"user"}
	}

	newUser, err := s.queries.Create(traceCtx, CreateParams{
		Name:      pgtype.Text{String: name, Valid: name != ""},
		Username:  pgtype.Text{String: username, Valid: username != ""},
		AvatarUrl: pgtype.Text{String: avatarUrl, Valid: avatarUrl != ""},
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
	return newUser.ID, err
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

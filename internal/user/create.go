package user

import (
	"context"
	"errors"

	"NYCU-SDC/core-system-backend/internal"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// linkEmailTx assigns email to accountID inside an existing transaction, enforcing global email uniqueness.
func (s *Service) linkEmailTx(ctx context.Context, qtx *Queries, userID uuid.UUID, email string) error {
	if email == "" {
		return nil
	}

	logger := logutil.WithContext(ctx, s.logger)

	err := qtx.validateEmailFree(ctx, email)
	if err != nil {
		if errors.Is(err, internal.ErrEmailConflict) {
			return err
		}
		return databaseutil.WrapDBError(err, logger, "validate email availability")
	}

	err = qtx.UpsertEmail(ctx, UpsertEmailParams{
		UserID: userID,
		Email:  email,
	})
	if err != nil {
		return databaseutil.WrapDBError(err, logger, "upsert email")
	}

	err = validateEmailOwner(ctx, qtx, email, userID)
	if err != nil {
		if errors.Is(err, internal.ErrEmailConflict) {
			return err
		}
		return databaseutil.WrapDBError(err, logger, "validate email owner")
	}

	return nil
}

// createEmailOnlyTx creates an email-only user (no auth row) and links email within a transaction.
func (s *Service) createEmailOnlyTx(
	ctx context.Context,
	qtx *Queries,
	email string,
	roles []string,
	userID *uuid.UUID,
) (uuid.UUID, error) {
	logger := logutil.WithContext(ctx, s.logger)
	var newUserID uuid.UUID

	if userID != nil {
		newUser, err := qtx.CreateWithID(ctx, CreateWithIDParams{
			ID:          *userID,
			Name:        pgtype.Text{},
			Username:    pgtype.Text{},
			AvatarUrl:   pgtype.Text{String: "", Valid: true},
			Role:        roles,
			IsOnboarded: false,
		})
		if err != nil {
			return uuid.UUID{}, databaseutil.WrapDBError(err, logger, "create user with id")
		}

		newUserID = newUser.ID
	} else {
		newUser, err := qtx.Create(ctx, CreateParams{
			Name:        pgtype.Text{},
			Username:    pgtype.Text{},
			AvatarUrl:   pgtype.Text{String: "", Valid: true},
			Role:        roles,
			IsOnboarded: false,
		})
		if err != nil {
			return uuid.UUID{}, databaseutil.WrapDBError(err, logger, "create user")
		}

		newUserID = newUser.ID
	}

	err := s.linkEmailTx(ctx, qtx, newUserID, email)
	if err != nil {
		return uuid.UUID{}, err
	}

	return newUserID, nil
}

// createOAuthParams groups fields required to create a user plus its first OAuth auth row.
type createOAuthParams struct {
	Name              string
	AvatarPlaceholder string
	Roles             []string
	Email             string
	OAuthProvider     string
	OAuthProviderID   string
}

// createOAuthTx creates the user, email link, and auth row inside the caller's transaction.
func (s *Service) createOAuthTx(ctx context.Context, qtx *Queries, params createOAuthParams) (uuid.UUID, error) {
	logger := logutil.WithContext(ctx, s.logger)

	newUser, err := qtx.Create(ctx, CreateParams{
		Name:      pgtype.Text{String: params.Name, Valid: params.Name != ""},
		AvatarUrl: pgtype.Text{String: params.AvatarPlaceholder, Valid: true},
		Role:      params.Roles,
	})
	if err != nil {
		return uuid.UUID{}, databaseutil.WrapDBError(err, logger, "create oauth user")
	}

	err = s.linkEmailTx(ctx, qtx, newUser.ID, params.Email)
	if err != nil {
		return uuid.UUID{}, err
	}

	_, err = qtx.CreateAuth(ctx, CreateAuthParams{
		UserID:     newUser.ID,
		Provider:   params.OAuthProvider,
		ProviderID: params.OAuthProviderID,
	})
	if err != nil {
		return uuid.UUID{}, databaseutil.WrapDBError(err, logger, "create oauth auth")
	}

	return newUser.ID, nil
}

// createOAuth creates a user with OAuth credentials in a single transaction.
func (s *Service) createOAuth(ctx context.Context, params createOAuthParams) (uuid.UUID, error) {
	var accountID uuid.UUID
	err := s.withTransaction(ctx, func(qtx *Queries) error {
		var txErr error
		accountID, txErr = s.createOAuthTx(ctx, qtx, params)
		return txErr
	})
	return accountID, err
}

// createWithEmailOnly creates an email-only user (no auth row) in a single transaction.
func (s *Service) createWithEmailOnly(
	ctx context.Context,
	email string,
	roles []string,
	userID *uuid.UUID,
) (uuid.UUID, error) {
	logger := logutil.WithContext(ctx, s.logger)
	var newUserID uuid.UUID
	err := s.withTransaction(ctx, func(qtx *Queries) error {
		if userID != nil {
			_, err := qtx.Get(ctx, *userID)
			if err == nil {
				return internal.ErrUserIDAlreadyExists
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return databaseutil.WrapDBError(err, logger, "check user id exists")
			}
		}

		var txErr error
		newUserID, txErr = s.createEmailOnlyTx(ctx, qtx, email, roles, userID)
		return txErr
	})
	return newUserID, err
}

package user

import (
	"context"

	"NYCU-SDC/core-system-backend/internal"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// linkEmailTx assigns email to accountID inside an existing transaction, enforcing global email uniqueness.
func (s *Service) linkEmailTx(ctx context.Context, qtx *Queries, accountID uuid.UUID, email string) error {
	if email == "" {
		return nil
	}

	err := qtx.validateEmailFree(ctx, email)
	if err != nil {
		return err
	}

	_, err = qtx.UpsertEmail(ctx, UpsertEmailParams{
		UserID: accountID,
		Value:  email,
	})
	if err != nil {
		return err
	}

	return validateEmailOwner(ctx, qtx, email, accountID)
}

// createEmailOnlyTx creates an email-only user (no auth row) and links email within a transaction.
func (s *Service) createEmailOnlyTx(
	ctx context.Context,
	qtx *Queries,
	email string,
	roles []string,
	userID *uuid.UUID,
) (uuid.UUID, error) {
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
			return uuid.UUID{}, err
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
			return uuid.UUID{}, err
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
	newUser, err := qtx.Create(ctx, CreateParams{
		Name:      pgtype.Text{String: params.Name, Valid: params.Name != ""},
		AvatarUrl: pgtype.Text{String: params.AvatarPlaceholder, Valid: true},
		Role:      params.Roles,
	})
	if err != nil {
		return uuid.UUID{}, err
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
		return uuid.UUID{}, err
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
	var newUserID uuid.UUID
	err := s.withTransaction(ctx, func(qtx *Queries) error {
		if userID != nil {
			exists, err := qtx.Exists(ctx, *userID)
			if err != nil {
				return err
			}
			if exists {
				return internal.ErrUserIDAlreadyExists
			}
		}

		var txErr error
		newUserID, txErr = s.createEmailOnlyTx(ctx, qtx, email, roles, userID)
		return txErr
	})
	return newUserID, err
}

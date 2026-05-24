package user

import (
	"context"
	"errors"

	"NYCU-SDC/core-system-backend/internal"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func userEmailIDParam(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}

	return pgtype.UUID{Bytes: id, Valid: true}
}

// linkEmailTx assigns email to accountID inside an existing transaction, enforcing global email uniqueness.
// It returns the user_emails row id when email is non-empty.
func (s *Service) linkEmailTx(ctx context.Context, qtx *Queries, userID uuid.UUID, email string) (uuid.UUID, error) {
	if email == "" {
		return uuid.Nil, nil
	}

	err := qtx.validateEmailFree(ctx, email)
	if err != nil {
		return uuid.Nil, err
	}

	emailID, err := qtx.UpsertEmail(ctx, UpsertEmailParams{
		UserID: userID,
		Value:  email,
	})
	if err != nil {
		return uuid.Nil, err
	}

	err = validateEmailOwner(ctx, qtx, email, userID)
	if err != nil {
		return uuid.Nil, err
	}

	return emailID, nil
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

	_, err := s.linkEmailTx(ctx, qtx, newUserID, email)
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

	emailID, err := s.linkEmailTx(ctx, qtx, newUser.ID, params.Email)
	if err != nil {
		return uuid.UUID{}, err
	}

	_, err = qtx.CreateAuth(ctx, CreateAuthParams{
		UserID:      newUser.ID,
		UserEmailID: userEmailIDParam(emailID),
		Provider:    params.OAuthProvider,
		ProviderID:  params.OAuthProviderID,
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
			_, err := qtx.Get(ctx, *userID)
			if err == nil {
				return internal.ErrUserIDAlreadyExists
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}

		var txErr error
		newUserID, txErr = s.createEmailOnlyTx(ctx, qtx, email, roles, userID)
		return txErr
	})
	return newUserID, err
}

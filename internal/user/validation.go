package user

import (
	"context"
	"errors"

	"NYCU-SDC/core-system-backend/internal"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// validateEmailFree locks the email row and returns ErrEmailConflict if it is already registered.
func (q *Queries) validateEmailFree(ctx context.Context, email string) error {
	_, err := q.GetByEmailForUpdate(ctx, email)
	if err == nil {
		return internal.ErrEmailConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

// validateEmailOwner returns ErrEmailConflict when the global email is owned by another account.
func validateEmailOwner(ctx context.Context, q interface {
	GetByEmail(context.Context, string) (uuid.UUID, error)
}, email string, accountID uuid.UUID) error {
	ownerID, err := q.GetByEmail(ctx, email)
	if err != nil {
		return err
	}
	if ownerID != accountID {
		return internal.ErrEmailConflict
	}
	return nil
}

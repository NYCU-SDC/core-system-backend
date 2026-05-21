package shared

import (
	"context"

	"NYCU-SDC/core-system-backend/internal"

	"github.com/google/uuid"
)

type ResponseStore interface {
	GetEditInfo(ctx context.Context, id uuid.UUID) (progress string, allowEditResponse bool, err error)
}

func ValidateResponseEditable(ctx context.Context, getter ResponseStore, responseID uuid.UUID) error {
	progress, allowEditResponse, err := getter.GetEditInfo(ctx, responseID)
	if err != nil {
		return err
	}

	if progress == "submitted" && !allowEditResponse {
		return internal.ErrResponseEditNotAllowed
	}

	return nil
}

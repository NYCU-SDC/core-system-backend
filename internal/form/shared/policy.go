package shared

import (
	"context"

	"NYCU-SDC/core-system-backend/internal"

	"github.com/google/uuid"
)

type ResponseStore interface {
	GetEditInfo(ctx context.Context, id uuid.UUID) (progress string, allowEditResponse bool, err error)
}

const ResponseProgressSubmitted = "submitted"

func ValidateResponseEditable(ctx context.Context, responseStore ResponseStore, responseID uuid.UUID) error {
	progress, allowEditResponse, err := responseStore.GetEditInfo(ctx, responseID)
	if err != nil {
		return err
	}

	if progress == ResponseProgressSubmitted && !allowEditResponse {
		return internal.ErrResponseEditNotAllowed
	}

	return nil
}

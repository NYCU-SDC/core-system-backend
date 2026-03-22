package resolver

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type UnitIdResolver interface {
	ResolveUnitID(ctx context.Context, r *http.Request) (uuid.UUID, error)
}

type FormIdResolver interface {
	ResolveFormID(ctx context.Context, r *http.Request) (uuid.UUID, error)
}

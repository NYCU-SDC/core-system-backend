package resolver

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type UnitIDResolver interface {
	ResolveUnitID(ctx context.Context, r *http.Request) (uuid.UUID, error)
}

type FormIDResolver interface {
	ResolveFormID(ctx context.Context, r *http.Request) (uuid.UUID, error)
}

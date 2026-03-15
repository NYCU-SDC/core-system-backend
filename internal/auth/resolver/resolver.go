package resolver

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type Resolver interface {
	ResolveUnitID(ctx context.Context, r *http.Request) (uuid.UUID, error)
}

package unit

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"

	"github.com/google/uuid"
)

type PathResolver struct{}

func NewPathResolver() *PathResolver {
	return &PathResolver{}
}

func (r *PathResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	unitIDStr := req.PathValue("unitId")
	if unitIDStr == "" {
		return uuid.Nil, internal.ErrMissingUnitID
	}

	unitID, err := uuid.Parse(unitIDStr)
	if err != nil {
		return uuid.Nil, internal.ErrInvalidUnitID
	}

	return unitID, nil
}

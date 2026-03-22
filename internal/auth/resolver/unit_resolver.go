package resolver

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"

	"github.com/google/uuid"
)

type UnitPathResolver struct{}

func NewUnitPathResolver() *UnitPathResolver {
	return &UnitPathResolver{}
}

func (r *UnitPathResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
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

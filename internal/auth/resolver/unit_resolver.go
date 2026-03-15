package resolver

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

type UnitResolver struct{}

func NewUnitResolver() *UnitResolver {
	return &UnitResolver{}
}

func (r *UnitResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	unitIDStr := req.PathValue("unitId")
	if unitIDStr == "" {
		return uuid.Nil, errors.New("unitId not provided")
	}

	unitID, err := uuid.Parse(unitIDStr)
	if err != nil {
		return uuid.Nil, err
	}

	return unitID, nil
}

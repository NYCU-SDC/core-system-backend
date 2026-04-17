package responseresolver

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"

	"github.com/google/uuid"
)

type ResponseService interface {
	GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

type PathResolver struct {
	service ResponseService
}

func NewPathResolver(service ResponseService) *PathResolver {
	return &PathResolver{
		service: service,
	}
}

func (r *PathResolver) ResolveFormID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	responseIDStr := req.PathValue("responseId")
	if responseIDStr == "" {
		return uuid.Nil, internal.ErrMissingResponseID
	}

	responseID, err := uuid.Parse(responseIDStr)
	if err != nil {
		return uuid.Nil, internal.ErrInvalidResponseID
	}

	formID, err := r.service.GetFormIDByID(ctx, responseID)
	if err != nil {
		return uuid.Nil, err
	}

	return formID, nil
}

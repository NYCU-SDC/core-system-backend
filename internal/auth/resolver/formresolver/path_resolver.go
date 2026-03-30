package formresolver

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"

	"github.com/google/uuid"
)

type FormService interface {
	GetUnitIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

type PathResolver struct {
	service FormService
}

func NewPathResolver(service FormService) *PathResolver {
	return &PathResolver{
		service: service,
	}
}

func (r *PathResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	formIDStr := req.PathValue("formId")
	if formIDStr == "" {
		return uuid.Nil, internal.ErrMissingFormID
	}

	formID, err := uuid.Parse(formIDStr)
	if err != nil {
		return uuid.Nil, internal.ErrInvalidFormID
	}

	unitID, err := r.service.GetUnitIDByID(ctx, formID)
	if err != nil {
		return uuid.Nil, err
	}

	return unitID, nil
}

func (r *PathResolver) ResolveFormID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	formIDStr := req.PathValue("formId")
	if formIDStr == "" {
		return uuid.Nil, internal.ErrMissingFormID
	}

	formID, err := uuid.Parse(formIDStr)
	if err != nil {
		return uuid.Nil, internal.ErrInvalidFormID
	}

	return formID, nil
}

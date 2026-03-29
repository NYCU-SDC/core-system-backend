package resolver

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"

	"github.com/google/uuid"
)

type FormService interface {
	GetUnitIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

type FormPathResolver struct {
	service FormService
}

func NewFormPathResolver(service FormService) *FormPathResolver {
	return &FormPathResolver{
		service: service,
	}
}

func (r *FormPathResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
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

func (r *FormPathResolver) ResolveFormID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
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

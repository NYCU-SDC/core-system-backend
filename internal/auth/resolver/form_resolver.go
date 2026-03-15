package resolver

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

type FormService interface {
	GetUnitIDByFormID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

type FormResolver struct {
	service FormService
}

func NewFormResolver(service FormService) *FormResolver {
	return &FormResolver{
		service: service,
	}
}

func (r *FormResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	formIDStr := req.PathValue("formId")
	if formIDStr == "" {
		return uuid.Nil, errors.New("formId not provided")
	}

	formID, err := uuid.Parse(formIDStr)
	if err != nil {
		return uuid.Nil, err
	}

	unitID, err := r.service.GetUnitIDByFormID(ctx, formID)
	if err != nil {
		return uuid.Nil, err
	}

	return unitID, nil
}

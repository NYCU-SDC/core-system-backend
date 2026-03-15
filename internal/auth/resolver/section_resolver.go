package resolver

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

type SectionService interface {
	GetUnitIDBySectionID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

type SectionResolver struct {
	service SectionService
}

func NewSectionResolver(service SectionService) *SectionResolver {
	return &SectionResolver{
		service: service,
	}
}

func (r *SectionResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	sectionIDStr := req.PathValue("sectionId")
	if sectionIDStr == "" {
		return uuid.Nil, errors.New("sectionId not provided")
	}

	sectionID, err := uuid.Parse(sectionIDStr)
	if err != nil {
		return uuid.Nil, err
	}

	unitID, err := r.service.GetUnitIDBySectionID(ctx, sectionID)
	if err != nil {
		return uuid.Nil, err
	}

	return unitID, nil
}

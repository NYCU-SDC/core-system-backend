package resolver

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"

	"github.com/google/uuid"
)

type SectionService interface {
	GetUnitIDBySectionID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	GetIDBySectionID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

type SectionPathResolver struct {
	service SectionService
}

func NewSectionPathResolver(service SectionService) *SectionPathResolver {
	return &SectionPathResolver{
		service: service,
	}
}

func (r *SectionPathResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	sectionIDStr := req.PathValue("sectionId")
	if sectionIDStr == "" {
		return uuid.Nil, internal.ErrMissingSectionID
	}

	sectionID, err := uuid.Parse(sectionIDStr)
	if err != nil {
		return uuid.Nil, internal.ErrInvalidSectionID
	}

	unitID, err := r.service.GetUnitIDBySectionID(ctx, sectionID)
	if err != nil {
		return uuid.Nil, err
	}

	return unitID, nil
}

func (r *SectionPathResolver) ResolveFormID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	sectionIDStr := req.PathValue("sectionId")
	if sectionIDStr == "" {
		return uuid.Nil, internal.ErrMissingSectionID
	}

	sectionID, err := uuid.Parse(sectionIDStr)
	if err != nil {
		return uuid.Nil, internal.ErrInvalidSectionID
	}

	formID, err := r.service.GetIDBySectionID(ctx, sectionID)
	if err != nil {
		return uuid.Nil, err
	}

	return formID, nil
}

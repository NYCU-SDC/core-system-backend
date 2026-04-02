package sectionresolver

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

type PathResolver struct {
	service SectionService
}

func NewPathResolver(service SectionService) *PathResolver {
	return &PathResolver{
		service: service,
	}
}

func (r *PathResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
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

func (r *PathResolver) ResolveFormID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
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

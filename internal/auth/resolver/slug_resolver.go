package resolver

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"

	"github.com/google/uuid"
)

type TenantService interface {
	GetSlugStatus(ctx context.Context, slug string) (bool, uuid.UUID, error)
}

type SlugPathResolver struct {
	service TenantService
}

func NewSlugPathResolver(service TenantService) *SlugPathResolver {
	return &SlugPathResolver{
		service: service,
	}
}

func (r *SlugPathResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	slug := req.PathValue("slug")
	if slug == "" {
		return uuid.Nil, internal.ErrMissingSlug
	}

	exist, orgID, err := r.service.GetSlugStatus(ctx, slug)
	if err != nil {
		return uuid.Nil, err
	}

	if !exist {
		return uuid.Nil, internal.ErrOrgSlugNotFound
	}

	return orgID, nil
}

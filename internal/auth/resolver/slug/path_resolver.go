package slug

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"

	"github.com/google/uuid"
)

type TenantService interface {
	GetSlugStatus(ctx context.Context, slug string) (bool, uuid.UUID, error)
}

type PathResolver struct {
	service TenantService
}

func NewPathResolver(service TenantService) *PathResolver {
	return &PathResolver{
		service: service,
	}
}

func (r *PathResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
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

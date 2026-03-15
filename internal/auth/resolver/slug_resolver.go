package resolver

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

type TenantService interface {
	GetSlugStatus(ctx context.Context, slug string) (bool, uuid.UUID, error)
}

type SlugResolver struct {
	service TenantService
}

func NewSlugResolver(service TenantService) *SlugResolver {
	return &SlugResolver{
		service: service,
	}
}

func (r *SlugResolver) ResolveUnitID(ctx context.Context, req *http.Request) (uuid.UUID, error) {
	slug := req.PathValue("slug")
	if slug == "" {
		return uuid.Nil, errors.New("slug not provided")
	}

	exist, orgID, err := r.service.GetSlugStatus(ctx, slug)
	if err != nil {
		return uuid.Nil, err
	}

	if !exist {
		return uuid.Nil, errors.New("org slug not found")
	}

	return orgID, nil
}

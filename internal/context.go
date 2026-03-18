package internal

import (
	"context"

	"github.com/google/uuid"
)

type Identity interface {
	GetID() uuid.UUID
}

// GetUserIDFromContext extracts the authenticated user id from request context
func GetUserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userData := ctx.Value(UserContextKey)
	if userData == nil {
		return uuid.Nil, false
	}

	identity, ok := userData.(Identity)
	if !ok {
		return uuid.Nil, false
	}

	return identity.GetID(), true
}

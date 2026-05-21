package form

import (
	"NYCU-SDC/core-system-backend/internal/user"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserFromProfileFields builds user.User from discrete profile columns (as returned
// from form queries that join users / users_with_emails) for API mapping.
func UserFromProfileFields(id uuid.UUID, name, username, avatar pgtype.Text) user.User {
	return user.User{
		ID:        id,
		Name:      name,
		Username:  username,
		AvatarUrl: avatar,
	}
}

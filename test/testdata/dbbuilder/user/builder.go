package userbuilder

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"NYCU-SDC/core-system-backend/test/testdata"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

type FactoryParams struct {
	Name      string
	Username  string
	AvatarURL string
	Role      []string
	Email     []string
}

type Builder struct {
	t  *testing.T
	db dbbuilder.DBTX
}

func New(t *testing.T, db dbbuilder.DBTX) *Builder {
	return &Builder{t: t, db: db}
}

func (b Builder) Queries() *user.Queries {
	return user.New(b.db)
}

func (b Builder) Create(opts ...Option) user.User {
	queries := b.Queries()

	p := &FactoryParams{
		Name:      testdata.RandomFullName(),
		Username:  testdata.RandomName(),
		AvatarURL: testdata.RandomURL(),
		Role:      []string{"user"}, // Default role is "user"
		Email:     []string{testdata.RandomEmail()},
	}
	for _, opt := range opts {
		opt(p)
	}

	userRow, err := queries.Create(context.Background(), user.CreateParams{
		Name:      pgtype.Text{String: p.Name, Valid: true},
		Username:  pgtype.Text{String: p.Username, Valid: true},
		AvatarUrl: pgtype.Text{String: p.AvatarURL, Valid: true},
		Role:      p.Role,
		Email:     p.Email,
	})
	require.NoError(b.t, err)

	return userRow
}

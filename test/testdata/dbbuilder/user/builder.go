package userbuilder

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"NYCU-SDC/core-system-backend/test/testdata"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

type FactoryParams struct {
	Name        string
	Username    string
	AvatarURL   string
	Role        []string
	IsOnboarded bool
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
		Name:        testdata.RandomFullName(),
		Username:    fmt.Sprintf("%s-%s", testdata.RandomName(), uuid.NewString()),
		AvatarURL:   testdata.RandomURL(),
		Role:        []string{"user"}, // Default role is "user"
		IsOnboarded: false,
	}
	for _, opt := range opts {
		opt(p)
	}

	userRow, err := queries.Create(context.Background(), user.CreateParams{
		Name:        pgtype.Text{String: p.Name, Valid: true},
		Username:    pgtype.Text{String: p.Username, Valid: true},
		AvatarUrl:   pgtype.Text{String: p.AvatarURL, Valid: true},
		Role:        p.Role,
		IsOnboarded: p.IsOnboarded,
	})
	require.NoError(b.t, err)

	return userRow
}

// CreateEmail creates an email record for a user
func (b Builder) CreateEmail(userID uuid.UUID, email string) {
	queries := b.Queries()
	_, err := queries.UpsertEmail(context.Background(), user.UpsertEmailParams{
		UserID: userID,
		Value:  email,
	})
	require.NoError(b.t, err)
}

// CreateAuth links an OAuth provider to an account on the given email address.
func (b Builder) CreateAuth(accountID uuid.UUID, email, provider, providerID string) user.Auth {
	queries := b.Queries()
	emailID, err := queries.UpsertEmail(context.Background(), user.UpsertEmailParams{
		UserID: accountID,
		Value:  email,
	})
	require.NoError(b.t, err)

	auth, err := queries.CreateAuth(context.Background(), user.CreateAuthParams{
		UserID:      accountID,
		UserEmailID: pgtype.UUID{Bytes: emailID, Valid: true},
		Provider:    provider,
		ProviderID:  providerID,
	})
	require.NoError(b.t, err)
	return auth
}

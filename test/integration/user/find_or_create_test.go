package user

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestFindOrCreate(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	testCases := []struct {
		name     string
		setup    func(t *testing.T, db dbbuilder.DBTX) (params user.FindOrCreateParams, expectUserID uuid.UUID)
		validate func(t *testing.T, db dbbuilder.DBTX, params user.FindOrCreateParams, result user.FindOrCreateResult, err error, expectUserID uuid.UUID)
	}{
		{
			name: "same provider returning user",
			setup: func(t *testing.T, db dbbuilder.DBTX) (user.FindOrCreateParams, uuid.UUID) {
				builder := userbuilder.New(t, db)
				account := builder.Create()
				email := fmt.Sprintf("same-provider-%s@example.com", uuid.NewString())
				builder.CreateEmail(account.ID, email)
				provider := "google"
				providerID := uuid.NewString()
				builder.CreateAuth(account.ID, email, provider, providerID)
				return user.FindOrCreateParams{
					Name:            "Test User",
					Email:           email,
					Role:            []string{"user"},
					OAuthProvider:   provider,
					OAuthProviderID: providerID,
				}, account.ID
			},
			validate: func(t *testing.T, db dbbuilder.DBTX, params user.FindOrCreateParams, result user.FindOrCreateResult, err error, expectUserID uuid.UUID) {
				require.NoError(t, err)
				require.Equal(t, expectUserID, result.UserID)
				require.Empty(t, result.ExistingProvider)
			},
		},
		{
			name:  "email-only account attaches OAuth",
			setup: setupEmailOnlyAccountFindOrCreate,
			validate: func(t *testing.T, db dbbuilder.DBTX, params user.FindOrCreateParams, result user.FindOrCreateResult, err error, expectUserID uuid.UUID) {
				require.NoError(t, err)
				require.Equal(t, expectUserID, result.UserID)
				require.Empty(t, result.ExistingProvider)

				queries := user.New(db)
				_, authErr := queries.GetByAuth(context.Background(), user.GetByAuthParams{
					Provider:   params.OAuthProvider,
					ProviderID: params.OAuthProviderID,
				})
				require.NoError(t, authErr)
			},
		},
		{
			name: "cross-provider binding required",
			setup: func(t *testing.T, db dbbuilder.DBTX) (user.FindOrCreateParams, uuid.UUID) {
				builder := userbuilder.New(t, db)
				account := builder.Create(userbuilder.WithName("Existing User"))
				email := fmt.Sprintf("binding-%s@example.com", uuid.NewString())
				builder.CreateEmail(account.ID, email)
				builder.CreateAuth(account.ID, email, "github", uuid.NewString())
				return user.FindOrCreateParams{
					Name:            "Test User",
					Email:           email,
					Role:            []string{"user"},
					OAuthProvider:   "google",
					OAuthProviderID: uuid.NewString(),
				}, account.ID
			},
			validate: func(t *testing.T, db dbbuilder.DBTX, params user.FindOrCreateParams, result user.FindOrCreateResult, err error, expectUserID uuid.UUID) {
				require.NoError(t, err)
				require.Equal(t, expectUserID, result.UserID)
				require.Equal(t, "github", result.ExistingProvider)
				require.NotEmpty(t, result.ExistingProviderID)
				require.Equal(t, "Existing User", result.ExistingName)

				_, authErr := user.New(db).GetByAuth(context.Background(), user.GetByAuthParams{
					Provider:   params.OAuthProvider,
					ProviderID: params.OAuthProviderID,
				})
				require.ErrorIs(t, authErr, pgx.ErrNoRows)
			},
		},
		{
			name: "new signup with unknown email",
			setup: func(t *testing.T, db dbbuilder.DBTX) (user.FindOrCreateParams, uuid.UUID) {
				return user.FindOrCreateParams{
					Name:            testdata.RandomFullName(),
					Email:           fmt.Sprintf("new-%s@example.com", uuid.NewString()),
					Role:            []string{"user"},
					OAuthProvider:   "google",
					OAuthProviderID: uuid.NewString(),
				}, uuid.Nil
			},
			validate: func(t *testing.T, db dbbuilder.DBTX, params user.FindOrCreateParams, result user.FindOrCreateResult, err error, expectUserID uuid.UUID) {
				require.NoError(t, err)
				require.NotEqual(t, uuid.Nil, result.UserID)
				require.Empty(t, result.ExistingProvider)

				_, authErr := user.New(db).GetByAuth(context.Background(), user.GetByAuthParams{
					Provider:   params.OAuthProvider,
					ProviderID: params.OAuthProviderID,
				})
				require.NoError(t, authErr)
			},
		},
		{
			name: "empty email creates new account",
			setup: func(t *testing.T, db dbbuilder.DBTX) (user.FindOrCreateParams, uuid.UUID) {
				return user.FindOrCreateParams{
					Name:            testdata.RandomFullName(),
					Email:           "",
					Role:            []string{"user"},
					OAuthProvider:   "google",
					OAuthProviderID: uuid.NewString(),
				}, uuid.Nil
			},
			validate: func(t *testing.T, db dbbuilder.DBTX, params user.FindOrCreateParams, result user.FindOrCreateResult, err error, expectUserID uuid.UUID) {
				require.NoError(t, err)
				require.NotEqual(t, uuid.Nil, result.UserID)
				require.Empty(t, result.ExistingProvider)

				_, authErr := user.New(db).GetByAuth(context.Background(), user.GetByAuthParams{
					Provider:   params.OAuthProvider,
					ProviderID: params.OAuthProviderID,
				})
				require.NoError(t, authErr)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			require.NoError(t, err)
			defer rollback()

			params, expectUserID := tc.setup(t, db)
			svc := newUserService(t, db, logger)

			result, err := svc.FindOrCreate(context.Background(), params)
			tc.validate(t, db, params, result, err, expectUserID)
		})
	}
}

func TestFindOrCreate_emailOnlyThenCrossProviderBinding(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	db, rollback, err := resourceManager.SetupPostgres()
	require.NoError(t, err)
	defer rollback()

	builder := userbuilder.New(t, db)
	account := builder.Create(userbuilder.WithName("Pre User"))
	email := fmt.Sprintf("pre-then-bind-%s@example.com", uuid.NewString())
	builder.CreateEmail(account.ID, email)
	svc := newUserService(t, db, logger)

	githubID := uuid.NewString()
	first, err := svc.FindOrCreate(context.Background(), user.FindOrCreateParams{
		Name:            "Pre User",
		Email:           email,
		Role:            []string{"user"},
		OAuthProvider:   "github",
		OAuthProviderID: githubID,
	})
	require.NoError(t, err)
	require.Equal(t, account.ID, first.UserID)
	require.Empty(t, first.ExistingProvider)

	googleID := uuid.NewString()
	second, err := svc.FindOrCreate(context.Background(), user.FindOrCreateParams{
		Name:            "Pre User",
		Email:           email,
		Role:            []string{"user"},
		OAuthProvider:   "google",
		OAuthProviderID: googleID,
	})
	require.NoError(t, err)
	require.Equal(t, account.ID, second.UserID)
	require.Equal(t, "github", second.ExistingProvider)
	require.Equal(t, githubID, second.ExistingProviderID)

	_, err = user.New(db).GetByAuth(context.Background(), user.GetByAuthParams{
		Provider:   "google",
		ProviderID: googleID,
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestFindOrCreate_secondCallReturnsSameAccount(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	db, rollback, err := resourceManager.SetupPostgres()
	require.NoError(t, err)
	defer rollback()

	email := fmt.Sprintf("repeat-%s@example.com", uuid.NewString())
	provider := "google"
	providerID := uuid.NewString()
	svc := newUserService(t, db, logger)

	first, err := svc.FindOrCreate(context.Background(), user.FindOrCreateParams{
		Name:            "First",
		Email:           email,
		Role:            []string{"user"},
		OAuthProvider:   provider,
		OAuthProviderID: providerID,
	})
	require.NoError(t, err)
	require.Empty(t, first.ExistingProvider)

	second, err := svc.FindOrCreate(context.Background(), user.FindOrCreateParams{
		Name:            "Second",
		Email:           email,
		Role:            []string{"user"},
		OAuthProvider:   provider,
		OAuthProviderID: providerID,
	})
	require.NoError(t, err)
	require.Empty(t, second.ExistingProvider)
	require.Equal(t, first.UserID, second.UserID)
}

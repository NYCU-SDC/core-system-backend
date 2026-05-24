package user

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGetLoginProfileEntries(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	testCases := []struct {
		name     string
		setup    func(t *testing.T, builder *userbuilder.Builder) (accountID uuid.UUID, byEmail map[string][]string)
		validate func(t *testing.T, entries []user.EmailAuthEntry, byEmail map[string][]string)
	}{
		{
			name: "per-email providers from view",
			setup: func(t *testing.T, builder *userbuilder.Builder) (uuid.UUID, map[string][]string) {
				account := builder.Create()
				emailA := fmt.Sprintf("profile-a-%s@example.com", uuid.NewString())
				emailB := fmt.Sprintf("profile-b-%s@example.com", uuid.NewString())
				builder.CreateEmail(account.ID, emailA)
				builder.CreateEmail(account.ID, emailB)
				builder.CreateAuth(account.ID, emailA, "github", uuid.NewString())
				builder.CreateAuth(account.ID, emailB, "google", uuid.NewString())
				return account.ID, map[string][]string{
					emailA: {"github"},
					emailB: {"google"},
				}
			},
			validate: func(t *testing.T, entries []user.EmailAuthEntry, byEmail map[string][]string) {
				require.Len(t, entries, len(byEmail))
				got := make(map[string][]string, len(entries))
				for _, entry := range entries {
					got[entry.Email] = entry.AuthProviders
				}
				for email, expected := range byEmail {
					require.ElementsMatch(t, expected, got[email])
				}
			},
		},
		{
			name: "email-only setup user",
			setup: func(t *testing.T, builder *userbuilder.Builder) (uuid.UUID, map[string][]string) {
				account := builder.Create()
				email := fmt.Sprintf("setup-only-%s@example.com", uuid.NewString())
				builder.CreateEmail(account.ID, email)
				return account.ID, map[string][]string{email: {}}
			},
			validate: func(t *testing.T, entries []user.EmailAuthEntry, byEmail map[string][]string) {
				require.Len(t, entries, 1)
				for email, expected := range byEmail {
					require.Equal(t, email, entries[0].Email)
					require.ElementsMatch(t, expected, entries[0].AuthProviders)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			require.NoError(t, err)
			defer rollback()

			builder := userbuilder.New(t, db)
			accountID, byEmail := tc.setup(t, builder)

			svc := newUserService(t, db, logger)
			entries, err := svc.GetLoginProfileEntries(context.Background(), accountID)
			require.NoError(t, err)

			tc.validate(t, entries, byEmail)
		})
	}
}

func TestFindOrCreate_updatesLoginProfile(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	testCases := []struct {
		name              string
		setup             func(t *testing.T, db dbbuilder.DBTX) (accountID uuid.UUID, email string, params user.FindOrCreateParams)
		expectedProviders func(params user.FindOrCreateParams) []string
	}{
		{
			name: "email-only account OAuth bind",
			setup: func(t *testing.T, db dbbuilder.DBTX) (uuid.UUID, string, user.FindOrCreateParams) {
				params, accountID := setupEmailOnlyAccountFindOrCreate(t, db)
				return accountID, params.Email, params
			},
			expectedProviders: func(params user.FindOrCreateParams) []string {
				return []string{params.OAuthProvider}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			require.NoError(t, err)
			defer rollback()

			accountID, email, params := tc.setup(t, db)
			svc := newUserService(t, db, logger)

			result, err := svc.FindOrCreate(context.Background(), params)
			require.NoError(t, err)
			require.Equal(t, accountID, result.UserID)
			require.Empty(t, result.ExistingProvider)

			entries, err := svc.GetLoginProfileEntries(context.Background(), accountID)
			require.NoError(t, err)

			entry := loginProfileEntryForEmail(t, entries, email)
			require.NotNil(t, entry)
			require.ElementsMatch(t, tc.expectedProviders(params), entry.AuthProviders)
		})
	}
}

func TestCreateAuth_updatesLoginProfile(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	testCases := []struct {
		name              string
		setup             func(t *testing.T, db dbbuilder.DBTX, svc *user.Service) (ownerID uuid.UUID, email string)
		expectedProviders []string
		profileMsg        string
	}{
		{
			name: "link copies user_email_id to profile",
			setup: func(t *testing.T, db dbbuilder.DBTX, svc *user.Service) (uuid.UUID, string) {
				ownerID, email, existingProvider, existingProviderID, newProvider, newProviderID := setupAuthLinkScenario(t, db)
				err := svc.CreateAuth(
					context.Background(),
					ownerID,
					newProvider,
					newProviderID,
					existingProvider,
					existingProviderID,
				)
				require.NoError(t, err)
				return ownerID, email
			},
			expectedProviders: []string{"github", "google"},
		},
		{
			name: "without existing provider leaves profile empty",
			setup: func(t *testing.T, db dbbuilder.DBTX, svc *user.Service) (uuid.UUID, string) {
				ownerID, email, provider, providerID := setupEmailOnlyCreateAuthScenario(t, db)
				err := svc.CreateAuth(context.Background(), ownerID, provider, providerID, "", "")
				require.NoError(t, err)
				return ownerID, email
			},
			expectedProviders: nil,
			profileMsg:        "NULL user_email_id: setup-style email row stays without per-email providers until OAuth signup binds auth",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			require.NoError(t, err)
			defer rollback()

			svc := newUserService(t, db, logger)
			ownerID, email := tc.setup(t, db, svc)

			entries, err := svc.GetLoginProfileEntries(context.Background(), ownerID)
			require.NoError(t, err)

			entry := loginProfileEntryForEmail(t, entries, email)
			require.NotNil(t, entry, "email should appear in login profile")

			if tc.expectedProviders == nil {
				require.Empty(t, entry.AuthProviders, tc.profileMsg)
				return
			}
			require.ElementsMatch(t, tc.expectedProviders, entry.AuthProviders)
		})
	}
}

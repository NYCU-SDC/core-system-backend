package user

import (
	"NYCU-SDC/core-system-backend/internal"
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

func TestCreateAuth(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	testCases := []struct {
		name        string
		setup       func(t *testing.T, db dbbuilder.DBTX, svc *user.Service) error
		expectedErr error
	}{
		{
			name: "rejects link when user does not own existing provider",
			setup: func(t *testing.T, db dbbuilder.DBTX, svc *user.Service) error {
				builder := userbuilder.New(t, db)
				owner := builder.Create()
				impostor := builder.Create()
				email := fmt.Sprintf("auth-wrong-owner-%s@example.com", uuid.NewString())
				existingProvider := "github"
				existingProviderID := uuid.NewString()
				builder.CreateAuth(owner.ID, email, existingProvider, existingProviderID)

				return svc.CreateAuth(
					context.Background(),
					impostor.ID,
					"google",
					uuid.NewString(),
					existingProvider,
					existingProviderID,
				)
			},
			expectedErr: internal.ErrInvalidAuthUser,
		},
		{
			name: "links second provider for valid owner",
			setup: func(t *testing.T, db dbbuilder.DBTX, svc *user.Service) error {
				ownerID, _, existingProvider, existingProviderID, newProvider, newProviderID := setupAuthLinkScenario(t, db)

				err := svc.CreateAuth(
					context.Background(),
					ownerID,
					newProvider,
					newProviderID,
					existingProvider,
					existingProviderID,
				)
				if err != nil {
					return err
				}

				queries := user.New(db)
				_, authErr := queries.GetByAuth(context.Background(), user.GetByAuthParams{
					Provider:   newProvider,
					ProviderID: newProviderID,
				})
				require.NoError(t, authErr)

				linkedEmailID, err := queries.GetEmailIDByAuth(context.Background(), user.GetEmailIDByAuthParams{
					Provider:   newProvider,
					ProviderID: newProviderID,
				})
				require.NoError(t, err)
				require.True(t, linkedEmailID.Valid)

				existingEmailID, err := queries.GetEmailIDByAuth(context.Background(), user.GetEmailIDByAuthParams{
					Provider:   existingProvider,
					ProviderID: existingProviderID,
				})
				require.NoError(t, err)
				require.Equal(t, existingEmailID, linkedEmailID)
				return nil
			},
		},
		{
			name: "without existing provider creates unbound auth",
			setup: func(t *testing.T, db dbbuilder.DBTX, svc *user.Service) error {
				ownerID, _, provider, providerID := setupEmailOnlyCreateAuthScenario(t, db)

				err := svc.CreateAuth(context.Background(), ownerID, provider, providerID, "", "")
				if err != nil {
					return err
				}

				_, authErr := user.New(db).GetByAuth(context.Background(), user.GetByAuthParams{
					Provider:   provider,
					ProviderID: providerID,
				})
				require.NoError(t, authErr)
				return nil
			},
		},
		{
			name: "duplicate link create is idempotent",
			setup: func(t *testing.T, db dbbuilder.DBTX, svc *user.Service) error {
				ownerID, _, existingProvider, existingProviderID, newProvider, newProviderID := setupAuthLinkScenario(t, db)

				require.NoError(t, svc.CreateAuth(
					context.Background(),
					ownerID,
					newProvider,
					newProviderID,
					existingProvider,
					existingProviderID,
				))
				return svc.CreateAuth(
					context.Background(),
					ownerID,
					newProvider,
					newProviderID,
					existingProvider,
					existingProviderID,
				)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			require.NoError(t, err)
			defer rollback()

			svc := newUserService(t, db, logger)
			err = tc.setup(t, db, svc)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

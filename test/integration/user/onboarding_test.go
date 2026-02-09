package user

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/user"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUserService_Onboarding(t *testing.T) {
	type params struct {
		id       uuid.UUID
		name     string
		username string
	}
	testCases := []struct {
		name        string
		params      params
		setup       func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context
		validate    func(t *testing.T, params params, db dbbuilder.DBTX, result user.User, err error)
		expectedErr bool
	}{
		{
			name: "Onboarding successfully",
			params: params{
				name:     "test_name",
				username: "test_username",
			},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				userBuilder := userbuilder.New(t, db)
				user := userBuilder.Create()
				params.id = user.ID
				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, result user.User, err error) {
				require.NoError(t, err)

				require.Equal(t, params.name, result.Name.String)
				require.Equal(t, params.username, result.Username.String)
				require.True(t, result.IsOnboarded)

				dbUser, err := user.New(db).GetByID(context.Background(), params.id)
				require.NoError(t, err)
				require.Equal(t, params.name, dbUser.Name.String)
				require.Equal(t, params.username, dbUser.Username.String)
				require.True(t, dbUser.IsOnboarded)
			},
			expectedErr: false,
		},
		{
			name: "User already onboarded",
			params: params{
				name:     "test_name",
				username: "test_username",
			},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				userBuilder := userbuilder.New(t, db)
				user := userBuilder.Create(userbuilder.WithIsOnboarded(true))
				params.id = user.ID
				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, result user.User, err error) {
				require.ErrorIs(t, err, internal.ErrUserOnboarded)
			},
			expectedErr: true,
		},
		{
			name: "Username conflict",
			params: params{
				name:     "test_name",
				username: "test_username",
			},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				userBuilder := userbuilder.New(t, db)
				userBuilder.Create(userbuilder.WithUsername("test_username"))
				user := userBuilder.Create()
				params.id = user.ID
				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, result user.User, err error) {
				require.ErrorIs(t, err, internal.ErrUsernameConflict)
			},
			expectedErr: true,
		},
	}

	resourceManager, logger, err := integration.GetOrInitResource()
	if err != nil {
		t.Fatalf("failed to get resource manager: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			if err != nil {
				t.Fatalf("failed to setup postgres: %v", err)
			}
			defer rollback()

			ctx := context.Background()
			params := tc.params
			if tc.setup != nil {
				ctx = tc.setup(t, &params, db)
			}

			service := user.NewService(logger, db)
			result, err := service.Onboarding(ctx, params.id, params.name, params.username)
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)
			if tc.validate != nil {
				tc.validate(t, params, db, result, err)
			}
		})
	}
}

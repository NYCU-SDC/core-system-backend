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

func TestFindOrCreateByEmail(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	testCases := []struct {
		name        string
		setup       func(t *testing.T, db dbbuilder.DBTX) (email string, userID *uuid.UUID, expectID uuid.UUID)
		validate    func(t *testing.T, db dbbuilder.DBTX, email string, gotID uuid.UUID, err error, expectID uuid.UUID)
		expectedErr bool
	}{
		{
			name: "creates user for new email",
			setup: func(t *testing.T, db dbbuilder.DBTX) (string, *uuid.UUID, uuid.UUID) {
				email := fmt.Sprintf("fce-new-%s@example.com", uuid.NewString())
				return email, nil, uuid.Nil
			},
			validate: func(t *testing.T, db dbbuilder.DBTX, email string, gotID uuid.UUID, err error, expectID uuid.UUID) {
				require.NoError(t, err)
				require.NotEqual(t, uuid.Nil, gotID)
				owner, lookupErr := user.New(db).GetByEmail(context.Background(), email)
				require.NoError(t, lookupErr)
				require.Equal(t, gotID, owner)
			},
		},
		{
			name: "returns existing user for known email",
			setup: func(t *testing.T, db dbbuilder.DBTX) (string, *uuid.UUID, uuid.UUID) {
				builder := userbuilder.New(t, db)
				account := builder.Create()
				email := fmt.Sprintf("fce-existing-%s@example.com", uuid.NewString())
				builder.CreateEmail(account.ID, email)
				return email, nil, account.ID
			},
			validate: func(t *testing.T, db dbbuilder.DBTX, email string, gotID uuid.UUID, err error, expectID uuid.UUID) {
				require.NoError(t, err)
				require.Equal(t, expectID, gotID)
			},
		},
		{
			name: "rejects userID mismatch with existing email owner",
			setup: func(t *testing.T, db dbbuilder.DBTX) (string, *uuid.UUID, uuid.UUID) {
				builder := userbuilder.New(t, db)
				account := builder.Create()
				email := fmt.Sprintf("fce-mismatch-%s@example.com", uuid.NewString())
				builder.CreateEmail(account.ID, email)
				otherID := uuid.New()
				return email, &otherID, account.ID
			},
			validate: func(t *testing.T, db dbbuilder.DBTX, email string, gotID uuid.UUID, err error, expectID uuid.UUID) {
				require.ErrorIs(t, err, internal.ErrEmailConflict)
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			require.NoError(t, err)
			defer rollback()

			email, userID, expectID := tc.setup(t, db)
			svc := newUserService(t, db, logger)

			gotID, err := svc.FindOrCreateByEmail(context.Background(), email, []string{"user"}, userID)
			tc.validate(t, db, email, gotID, err, expectID)
		})
	}
}

func TestFindOrCreateByEmail_secondCallReturnsSameAccount(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	db, rollback, err := resourceManager.SetupPostgres()
	require.NoError(t, err)
	defer rollback()

	email := fmt.Sprintf("fce-repeat-%s@example.com", uuid.NewString())
	svc := newUserService(t, db, logger)

	first, err := svc.FindOrCreateByEmail(context.Background(), email, []string{"user"}, nil)
	require.NoError(t, err)

	second, err := svc.FindOrCreateByEmail(context.Background(), email, []string{"user"}, nil)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

package form

import (
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/markdown"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	formbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/form"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	resourceManager, _, err := integration.GetOrInitResource()
	if err != nil {
		panic(err)
	}

	_, rollback, err := resourceManager.SetupPostgres()
	if err != nil {
		panic(err)
	}

	code := m.Run()

	rollback()
	resourceManager.Cleanup()

	os.Exit(code)
}

func TestFormService_ListExcludeExpired(t *testing.T) {
	type params struct {
		noDeadlineID uuid.UUID
		expiredID    uuid.UUID
		expiredID2   uuid.UUID
		futureID     uuid.UUID
	}

	seedMixedDeadlineForms := func(t *testing.T, p *params, db dbbuilder.DBTX) {
		t.Helper()

		user := userbuilder.New(t, db).Create()
		builder := formbuilder.New(t, db)

		noDeadline := builder.Create(
			formbuilder.WithTitle("no-deadline-form"),
			formbuilder.WithLastEditor(user.ID),
		)
		expired := builder.Create(
			formbuilder.WithTitle("expired-form"),
			formbuilder.WithLastEditor(user.ID),
			formbuilder.WithDeadline(pgtype.Timestamptz{Time: time.Now().Add(-24 * time.Hour), Valid: true}),
		)
		future := builder.Create(
			formbuilder.WithTitle("future-deadline-form"),
			formbuilder.WithLastEditor(user.ID),
			formbuilder.WithDeadline(pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true}),
		)

		p.noDeadlineID = noDeadline.ID
		p.expiredID = expired.ID
		p.futureID = future.ID
	}

	listedIDs := func(listed []form.ListRow) map[uuid.UUID]struct{} {
		ids := make(map[uuid.UUID]struct{}, len(listed))
		for _, f := range listed {
			ids[f.ID] = struct{}{}
		}
		return ids
	}

	testCases := []struct {
		name           string
		excludeExpired bool
		setup          func(t *testing.T, params *params, db dbbuilder.DBTX)
		validate       func(t *testing.T, err error, params params, result []form.ListRow)
	}{
		{
			name:           "includes null-deadline forms and excludes expired ones",
			excludeExpired: true,
			setup:          seedMixedDeadlineForms,
			validate: func(t *testing.T, err error, params params, result []form.ListRow) {
				t.Helper()
				require.NoError(t, err)

				ids := listedIDs(result)
				require.Contains(t, ids, params.noDeadlineID, "forms without a deadline should remain visible when excluding expired")
				require.Contains(t, ids, params.futureID, "forms with a future deadline should remain visible")
				require.NotContains(t, ids, params.expiredID, "forms with a past deadline should be excluded")
			},
		},
		{
			name:           "returns expired forms when excludeExpired is false",
			excludeExpired: false,
			setup:          seedMixedDeadlineForms,
			validate: func(t *testing.T, err error, params params, result []form.ListRow) {
				t.Helper()
				require.NoError(t, err)

				ids := listedIDs(result)
				require.Contains(t, ids, params.noDeadlineID)
				require.Contains(t, ids, params.futureID)
				require.Contains(t, ids, params.expiredID, "expired forms should remain visible when excludeExpired is false")
			},
		},
		{
			name:           "returns no forms when every form is expired",
			excludeExpired: true,
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) {
				t.Helper()

				user := userbuilder.New(t, db).Create()
				builder := formbuilder.New(t, db)

				expired1 := builder.Create(
					formbuilder.WithTitle("expired-form-1"),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithDeadline(pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}),
				)
				expired2 := builder.Create(
					formbuilder.WithTitle("expired-form-2"),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithDeadline(pgtype.Timestamptz{Time: time.Now().Add(-1 * time.Hour), Valid: true}),
				)

				params.expiredID = expired1.ID
				params.expiredID2 = expired2.ID
			},
			validate: func(t *testing.T, err error, params params, listed []form.ListRow) {
				t.Helper()
				require.NoError(t, err)

				ids := listedIDs(listed)
				require.NotContains(t, ids, params.expiredID)
				require.NotContains(t, ids, params.expiredID2)
			},
		},
		{
			name:           "excludes every expired form when several exist",
			excludeExpired: true,
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) {
				t.Helper()

				user := userbuilder.New(t, db).Create()
				builder := formbuilder.New(t, db)

				expired1 := builder.Create(
					formbuilder.WithTitle("expired-form-1"),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithDeadline(pgtype.Timestamptz{Time: time.Now().Add(-72 * time.Hour), Valid: true}),
				)
				expired2 := builder.Create(
					formbuilder.WithTitle("expired-form-2"),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithDeadline(pgtype.Timestamptz{Time: time.Now().Add(-12 * time.Hour), Valid: true}),
				)
				future := builder.Create(
					formbuilder.WithTitle("future-deadline-form"),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithDeadline(pgtype.Timestamptz{Time: time.Now().Add(12 * time.Hour), Valid: true}),
				)

				params.expiredID = expired1.ID
				params.expiredID2 = expired2.ID
				params.futureID = future.ID
			},
			validate: func(t *testing.T, err error, params params, listed []form.ListRow) {
				t.Helper()
				require.NoError(t, err)

				ids := listedIDs(listed)
				require.NotContains(t, ids, params.expiredID)
				require.NotContains(t, ids, params.expiredID2)
				require.Contains(t, ids, params.futureID)
			},
		},
	}

	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			require.NoError(t, err)
			defer rollback()

			params := params{}
			tc.setup(t, &params, db)

			formService := form.NewService(logger, db, markdown.NewService(logger))
			result, err := formService.List(context.Background(), "", "", tc.excludeExpired)

			tc.validate(t, err, params, result)
		})
	}
}

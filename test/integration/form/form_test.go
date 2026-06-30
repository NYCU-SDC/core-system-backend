package form

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/markdown"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	formbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/form"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
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

func TestResponseService_CancelSubmission(t *testing.T) {
	type params struct {
		formID     uuid.UUID
		ownerID    uuid.UUID
		otherID    uuid.UUID
		responseID uuid.UUID
	}

	testCases := []struct {
		name      string
		setup     func(t *testing.T, params *params, db dbbuilder.DBTX, responseService *response.Service, queries *response.Queries)
		actUserID func(params params) uuid.UUID
		validate  func(t *testing.T, err error, params params, queries *response.Queries)
	}{
		{
			name: "reverts submitted response to draft",
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX, responseService *response.Service, queries *response.Queries) {
				created, err := responseService.Create(context.Background(), params.formID, params.ownerID)
				require.NoError(t, err)
				params.responseID = created.ID

				_, err = queries.UpdateSubmitted(context.Background(), created.ID)
				require.NoError(t, err)
			},
			actUserID: func(params params) uuid.UUID { return params.ownerID },
			validate: func(t *testing.T, err error, params params, queries *response.Queries) {
				t.Helper()
				require.NoError(t, err)

				result, err := queries.Get(context.Background(), response.GetParams{ID: params.responseID, FormID: params.formID})
				require.NoError(t, err)
				require.Equal(t, response.ResponseProgressDraft, result.Progress)
				require.False(t, result.SubmittedAt.Valid)
			},
		},
		{
			name: "rejects cancel when response is still draft",
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX, responseService *response.Service, queries *response.Queries) {
				created, err := responseService.Create(context.Background(), params.formID, params.ownerID)
				require.NoError(t, err)
				params.responseID = created.ID
			},
			actUserID: func(params params) uuid.UUID { return params.ownerID },
			validate: func(t *testing.T, err error, params params, queries *response.Queries) {
				t.Helper()
				require.ErrorIs(t, err, internal.ErrResponseNotSubmitted)
			},
		},
		{
			name: "rejects cancel from non-owner",
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX, responseService *response.Service, queries *response.Queries) {
				created, err := responseService.Create(context.Background(), params.formID, params.ownerID)
				require.NoError(t, err)
				params.responseID = created.ID

				_, err = queries.UpdateSubmitted(context.Background(), created.ID)
				require.NoError(t, err)
			},
			actUserID: func(params params) uuid.UUID { return params.otherID },
			validate: func(t *testing.T, err error, params params, queries *response.Queries) {
				t.Helper()
				require.ErrorIs(t, err, internal.ErrResponseNotOwned)
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

			owner := userbuilder.New(t, db).Create()
			other := userbuilder.New(t, db).Create()
			formRow := formbuilder.New(t, db).Create(formbuilder.WithLastEditor(owner.ID))

			params := params{
				formID:  formRow.ID,
				ownerID: owner.ID,
				otherID: other.ID,
			}

			formService := form.NewService(logger, db, markdown.NewService(logger))
			responseService := response.NewService(logger, db, nil, nil, nil, formService, nil)
			queries := response.New(db)

			tc.setup(t, &params, db, responseService, queries)

			err = responseService.CancelSubmission(context.Background(), params.responseID, tc.actUserID(params))

			tc.validate(t, err, params, queries)
		})
	}
}

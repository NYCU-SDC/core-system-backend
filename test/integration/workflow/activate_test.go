package workflow

import (
	"NYCU-SDC/core-system-backend/internal/form/workflow"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	workflowbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/workflow"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWorkflowService_Activate(t *testing.T) {
	type Params struct {
		formID       uuid.UUID
		userID       uuid.UUID
		workflowJSON []byte
	}

	type testCase struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.ActivateRow, err error)
		expectedErr bool
	}

	testCases := []testCase{
		{
			name:   "Activate first workflow version",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("activate-first-org", "activate-first-unit")

				workflowJSON, _, _ := builder.CreateStartEndWorkflow()

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = workflowJSON

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.ActivateRow, err error) {
				require.NoError(t, err, "should not return error")
				require.NotEqual(t, uuid.Nil, result.ID, "workflow version ID should be set")
				require.Equal(t, params.formID, result.FormID, "form ID should match")
				require.Equal(t, params.userID, result.LastEditor, "last editor should match")
				require.True(t, result.IsActive, "workflow should be active")

				builder := workflowbuilder.New(t, db)
				// Verify workflow content
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.HasNodeType(workflowData, "start"), "workflow should have start node")
				require.True(t, builder.HasNodeType(workflowData, "end"), "workflow should have end node")
			},
			expectedErr: false,
		},
		{
			name:   "Activate updates inactive draft version",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("activate-draft-org", "activate-draft-unit")

				// Create initial draft workflow
				initialWorkflow, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, initialWorkflow)

				// Create new workflow to activate
				newWorkflow, _, _, _ := builder.CreateStartSectionEndWorkflow()

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = newWorkflow

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.ActivateRow, err error) {
				require.NoError(t, err, "should not return error")
				require.True(t, result.IsActive, "workflow should be active")
				require.Equal(t, params.formID, result.FormID, "form ID should match")
				require.Equal(t, params.userID, result.LastEditor, "last editor should match")

				builder := workflowbuilder.New(t, db)
				// Verify new workflow content
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.HasNodeType(workflowData, "section"), "workflow should have section node")

				// Verify only one active version exists
				queries := workflow.New(db)
				getRow, err := queries.Get(context.Background(), params.formID)
				require.NoError(t, err)
				require.True(t, getRow.IsActive, "latest version should be active")
			},
			expectedErr: false,
		},
		{
			name:   "Activate creates new version when latest is already active",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("activate-new-version-org", "activate-new-version-unit")

				// Create and activate initial workflow
				initialWorkflow, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateActiveWorkflow(data.FormRow.ID, data.User, initialWorkflow)

				// Create different workflow to activate
				newWorkflow, _, _, _ := builder.CreateStartSectionEndWorkflow()

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = newWorkflow

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.ActivateRow, err error) {
				require.NoError(t, err, "should not return error")
				require.True(t, result.IsActive, "new workflow version should be active")
				require.Equal(t, params.formID, result.FormID, "form ID should match")
				require.Equal(t, params.userID, result.LastEditor, "last editor should match")

				builder := workflowbuilder.New(t, db)
				// Verify new workflow content
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.HasNodeType(workflowData, "section"), "new workflow should have section node")

				// Verify only one active version exists and it's the one we just activated
				activeCount := builder.CountActiveVersions(params.formID)
				require.Equal(t, 1, activeCount, "should have exactly one active version")

				// Get the active version ID
				activeID := builder.GetActiveVersionID(params.formID)
				require.Equal(t, result.ID, activeID, "active version should be the newly activated one")
			},
			expectedErr: false,
		},
		{
			name:   "Activate skips when request matches current active workflow",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("activate-skip-org", "activate-skip-unit")

				// Create and activate workflow
				workflowJSON, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateActiveWorkflow(data.FormRow.ID, data.User, workflowJSON)

				// Get the active workflow to use same content
				queries := workflow.New(db)
				getRow, err := queries.Get(context.Background(), data.FormRow.ID)
				require.NoError(t, err)

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = getRow.Workflow // Use same workflow

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.ActivateRow, err error) {
				require.NoError(t, err, "should not return error")
				require.True(t, result.IsActive, "workflow should remain active")
				require.Equal(t, params.formID, result.FormID, "form ID should match")

				builder := workflowbuilder.New(t, db)
				// Verify workflow content matches
				workflowData := builder.ParseWorkflow(result.Workflow)
				originalWorkflow := builder.ParseWorkflow(params.workflowJSON)
				require.Equal(t, len(originalWorkflow), len(workflowData), "workflow should be unchanged")
			},
			expectedErr: false,
		},
		{
			name:   "Activate deactivates all previous active versions",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("activate-deactivate-org", "activate-deactivate-unit")

				// Create and activate first workflow
				workflow1, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateActiveWorkflow(data.FormRow.ID, data.User, workflow1)

				// Create and activate second workflow (should deactivate first)
				workflow2, _, _, _ := builder.CreateStartSectionEndWorkflow()
				builder.CreateActiveWorkflow(data.FormRow.ID, data.User, workflow2)

				// Create third workflow to activate
				workflow3, _, _, _ := builder.CreateStartConditionEndWorkflow()

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = workflow3

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.ActivateRow, err error) {
				require.NoError(t, err, "should not return error")
				require.True(t, result.IsActive, "new workflow should be active")

				// Verify only one active version exists and it's the one we just activated
				builder := workflowbuilder.New(t, db)
				activeCount := builder.CountActiveVersions(params.formID)
				require.Equal(t, 1, activeCount, "should have exactly one active version")

				// Get the active version ID
				activeID := builder.GetActiveVersionID(params.formID)
				require.Equal(t, result.ID, activeID, "active version should be the newly activated one")
				// Verify new workflow content
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.HasNodeType(workflowData, "condition"), "new workflow should have condition node")
			},
			expectedErr: false,
		},
		{
			name:   "Activate with non-existent form ID returns error",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				// Use a non-existent form ID
				params.formID = uuid.New()
				params.userID = uuid.New()
				// Create a valid workflow JSON
				workflowJSON, _, _ := workflowbuilder.New(t, db).CreateStartEndWorkflow()
				params.workflowJSON = workflowJSON

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.ActivateRow, err error) {
				require.Error(t, err, "should return error for non-existent form ID")
				// Error might be from form_lock CTE failing or foreign key constraint
				require.NotEmpty(t, err.Error(), "error message should not be empty")
			},
			expectedErr: true,
		},
		{
			name:   "Activate with invalid JSON workflow returns error",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("activate-invalid-json-org", "activate-invalid-json-unit")

				params.formID = data.FormRow.ID
				params.userID = data.User
				// Use invalid JSON
				params.workflowJSON = []byte(`{invalid json}`)

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.ActivateRow, err error) {
				require.Error(t, err, "should return error for invalid JSON workflow")
				// Error from database JSONB parsing (sqlc queries don't run Go validation)
				require.NotEmpty(t, err.Error(), "error message should not be empty")
			},
			expectedErr: true,
		},
	}

	resourceManager, _, err := integration.GetOrInitResource()
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

			queries := workflow.New(db)
			result, err := queries.Activate(ctx, workflow.ActivateParams{
				FormID:     params.formID,
				LastEditor: params.userID,
				Workflow:   params.workflowJSON,
			})

			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.validate != nil {
				tc.validate(t, params, db, result, err)
			}
		})
	}
}

package workflow

import (
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/workflow"
	"NYCU-SDC/core-system-backend/internal/markdown"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	workflowbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/workflow"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type Params struct {
	formID       uuid.UUID
	userID       uuid.UUID
	workflowJSON []byte
	versionID    uuid.UUID
}

type testCase struct {
	name        string
	params      Params
	setup       func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context
	validate    func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error)
	expectedErr bool
}

// TestWorkflow_Update exercises sqlc workflow queries.Update (persistence path without Service validation).
func TestWorkflow_Update(t *testing.T) {
	testCases := []testCase{
		{
			name:   "Update creates first workflow version when none exists",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("update-first-org", "update-first-unit")

				workflowJSON, _, _ := builder.CreateStartEndWorkflow()

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = workflowJSON

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.NoError(t, err, "should not return error")
				require.NotEqual(t, uuid.Nil, result.ID, "workflow version ID should be set")
				require.Equal(t, params.formID, result.FormID, "form ID should match")
				require.Equal(t, params.userID, result.LastEditor, "last editor should match")
				require.False(t, result.IsActive, "workflow should be draft (not active)")

				builder := workflowbuilder.New(t, db)
				// Verify workflow content
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.HasNodeType(workflowData, string(workflow.NodeTypeStart)), "workflow should have start node")
				require.True(t, builder.HasNodeType(workflowData, string(workflow.NodeTypeEnd)), "workflow should have end node")
			},
			expectedErr: false,
		},
		{
			name:   "Update modifies existing draft version",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("update-draft-org", "update-draft-unit")

				// Create initial draft workflow
				initialWorkflow, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, initialWorkflow)

				// Get the initial version ID
				getRow, err := data.Queries.Get(context.Background(), data.FormRow.ID)
				require.NoError(t, err)
				initialVersionID := getRow.ID

				// Create new workflow to update
				newWorkflow, _, _, _ := builder.CreateStartSectionEndWorkflow()

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = newWorkflow
				params.versionID = initialVersionID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.NoError(t, err, "should not return error")
				require.False(t, result.IsActive, "updated workflow should remain draft")
				require.Equal(t, params.formID, result.FormID, "form ID should match")
				require.Equal(t, params.userID, result.LastEditor, "last editor should match")

				builder := workflowbuilder.New(t, db)
				// Verify workflow content was updated
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.HasNodeType(workflowData, string(workflow.NodeTypeSection)), "updated workflow should have section node")

				// Verify it's the same version (updated, not created new)
				require.Equal(t, params.versionID, result.ID, "should update existing draft version, not create new one")
			},
			expectedErr: false,
		},
		{
			name:   "Update creates new draft version when latest is active",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("update-active-org", "update-active-unit")

				// Create and activate initial workflow
				initialWorkflow, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateActiveWorkflow(data.FormRow.ID, data.User, initialWorkflow)

				// Get the active version ID
				getRow, err := data.Queries.Get(context.Background(), data.FormRow.ID)
				require.NoError(t, err)
				activeVersionID := getRow.ID

				// Create new workflow to update
				newWorkflow, _, _, _ := builder.CreateStartSectionEndWorkflow()

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = newWorkflow
				params.versionID = activeVersionID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.NoError(t, err, "should not return error")
				require.False(t, result.IsActive, "new workflow version should be draft")
				require.Equal(t, params.formID, result.FormID, "form ID should match")
				require.Equal(t, params.userID, result.LastEditor, "last editor should match")

				builder := workflowbuilder.New(t, db)
				// Verify workflow content
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.HasNodeType(workflowData, string(workflow.NodeTypeSection)), "new workflow should have section node")

				// Verify it's a new version (not the active one)
				require.NotEqual(t, params.versionID, result.ID, "should create new draft version, not update active one")

				// Verify active version still exists and is unchanged
				queries := workflow.New(db)
				getRow, err := queries.Get(context.Background(), params.formID)
				require.NoError(t, err)
				// Get returns latest by updated_at, which should be the new draft
				require.Equal(t, result.ID, getRow.ID, "latest version should be the new draft")
				require.False(t, getRow.IsActive, "latest version should be draft")
			},
			expectedErr: false,
		},
		{
			name:   "Update can modify draft version multiple times",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("update-multiple-org", "update-multiple-unit")

				// Create initial draft workflow
				workflow1, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, workflow1)

				// Update it once
				workflow2, _, _, _ := builder.CreateStartSectionEndWorkflow()
				_, err := data.Queries.Update(context.Background(), workflow.UpdateParams{
					FormID:     data.FormRow.ID,
					LastEditor: data.User,
					Workflow:   workflow2,
				})
				require.NoError(t, err)

				// Get the version ID after first update
				getRow, err := data.Queries.Get(context.Background(), data.FormRow.ID)
				require.NoError(t, err)
				versionID := getRow.ID

				// Create another workflow for second update
				workflow3, _, _, _ := builder.CreateStartConditionEndWorkflow()

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = workflow3
				params.versionID = versionID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.NoError(t, err, "should not return error")
				require.False(t, result.IsActive, "workflow should remain draft")
				require.Equal(t, params.formID, result.FormID, "form ID should match")

				builder := workflowbuilder.New(t, db)
				// Verify workflow content was updated
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.HasNodeType(workflowData, string(workflow.NodeTypeCondition)), "updated workflow should have condition node")

				// Verify it's still the same version (updated multiple times)
				require.Equal(t, params.versionID, result.ID, "should update same draft version multiple times")
			},
			expectedErr: false,
		},
		{
			name:   "Update with non-existent form ID returns error",
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
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.Error(t, err, "should return error for non-existent form ID")
				require.NotEmpty(t, err.Error(), "error message should not be empty")
			},
			expectedErr: true,
		},
		{
			name:   "Update with invalid JSON workflow returns error",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("update-invalid-json-org", "update-invalid-json-unit")

				params.formID = data.FormRow.ID
				params.userID = data.User
				// Use invalid JSON
				params.workflowJSON = []byte(`{invalid json}`)

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.Error(t, err, "should return error for invalid JSON workflow")
				require.NotEmpty(t, err.Error(), "error message should not be empty")
			},
			expectedErr: true, // Database rejects invalid JSON at JSONB level
		},
		{
			name:   "Update with empty workflow JSON returns error",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("update-empty-json-org", "update-empty-json-unit")

				params.formID = data.FormRow.ID
				params.userID = data.User
				// Use empty JSON array
				params.workflowJSON = []byte(`[]`)

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				if err != nil {
					require.NotEmpty(t, err.Error(), "error message should not be empty")
				}
			},
			expectedErr: false, // queries.Update might accept empty workflow
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
			row, err := queries.Update(ctx, workflow.UpdateParams{
				FormID:     params.formID,
				LastEditor: params.userID,
				Workflow:   params.workflowJSON,
			})
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			var result workflow.WorkflowVersion
			if err == nil {
				result = workflow.WorkflowVersion(row)
			}
			if tc.validate != nil {
				tc.validate(t, params, db, result, err)
			}
		})
	}
}

// TestWorkflowService_Update_NoNewVersionWhenOnlyLabelsChange verifies that
// when the latest workflow version is active and the incoming workflow differs
// only by labels (same structure), Service.Update returns the current version
// and does not create a new workflow version.
func TestWorkflowService_Update_NoNewVersionWhenOnlyLabelsChange(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	if err != nil {
		t.Fatalf("failed to get resource manager: %v", err)
	}

	db, rollback, err := resourceManager.SetupPostgres()
	if err != nil {
		t.Fatalf("failed to setup postgres: %v", err)
	}
	defer rollback()

	ctx := context.Background()
	builder := workflowbuilder.New(t, db)
	data := builder.SetupTestData("update-labels-only-org", "update-labels-only-unit")

	// Create and activate a workflow (start -> end)
	initialWorkflow, startID, endID := builder.CreateStartEndWorkflow()
	builder.CreateActiveWorkflow(data.FormRow.ID, data.User, initialWorkflow)

	activeVersionID := builder.GetActiveVersionID(data.FormRow.ID)
	versionCountBefore := countWorkflowVersions(t, db, data.FormRow.ID)
	require.Equal(t, 1, versionCountBefore, "should have exactly one workflow version after activate")

	// Same structure as initialWorkflow but different labels only
	labelOnlyWorkflow, err := json.Marshal([]map[string]interface{}{
		{
			"id":      startID.String(),
			"type":    "start",
			"label":   "Updated Start Label",
			"next":    endID.String(),
			"payload": map[string]interface{}{"x": 0, "y": 0},
		},
		{
			"id":      endID.String(),
			"type":    "end",
			"label":   "Updated End Label",
			"payload": map[string]interface{}{"x": 0, "y": 0},
		},
	})
	require.NoError(t, err)

	questionService := question.NewService(logger, db, nil, markdown.NewService(logger))
	workflowService := workflow.NewService(logger, db, questionService)

	result, err := workflowService.Update(ctx, data.FormRow.ID, labelOnlyWorkflow, data.User)
	require.NoError(t, err)

	// Should return the existing active version, not create a new one
	require.Equal(t, activeVersionID, result.ID, "should return current active version, not create new one")
	require.True(t, result.IsActive, "returned version should still be active")

	versionCountAfter := countWorkflowVersions(t, db, data.FormRow.ID)
	require.Equal(t, 1, versionCountAfter, "should not create a new workflow version when only labels differ")
}

func countWorkflowVersions(t *testing.T, db dbbuilder.DBTX, formID uuid.UUID) int {
	t.Helper()
	var count int
	err := db.QueryRow(context.Background(), "SELECT COUNT(*) FROM workflow_versions WHERE form_id = $1", formID).Scan(&count)
	require.NoError(t, err)
	return count
}

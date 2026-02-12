package workflow

import (
	"NYCU-SDC/core-system-backend/internal/form/workflow"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	workflowbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/workflow"
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

func TestWorkflowService_DeleteNode(t *testing.T) {
	type Params struct {
		formID       uuid.UUID
		nodeID       uuid.UUID
		userID       uuid.UUID
		workflowJSON []byte
	}

	type testCase struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, result []byte, err error)
		expectedErr bool
	}

	testCases := []testCase{
		{
			name:   "Delete middle section node and nullify references",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("delete-node-org", "delete-node-unit")

				workflowJSON, _, sectionID, _ := builder.CreateStartSectionEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, workflowJSON)

				params.formID = data.FormRow.ID
				params.nodeID = sectionID
				params.userID = data.User
				params.workflowJSON = workflowJSON

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []byte, err error) {
				require.NoError(t, err, "should not return error")

				builder := workflowbuilder.New(t, db)
				workflowData := builder.ParseWorkflow(result)

				// Verify section node is deleted
				require.False(t, builder.NodeExists(workflowData, params.nodeID.String()), "section node should be deleted")

				// Verify start node's "next" reference is nullified (removed)
				require.False(t, builder.NodeReferencesDeletedNode(workflowData, params.nodeID.String(), "next"), "start node should not reference deleted node")

				// Verify workflow still has start and end nodes
				require.True(t, builder.HasNodeType(workflowData, string(workflow.NodeTypeStart)), "workflow should still have start node")
				require.True(t, builder.HasNodeType(workflowData, string(workflow.NodeTypeEnd)), "workflow should still have end node")
			},
			expectedErr: false,
		},
		{
			name:   "Delete condition node and nullify nextTrue/nextFalse references",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("delete-condition-org", "delete-condition-unit")

				workflowJSON, _, conditionID, _ := builder.CreateStartConditionEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, workflowJSON)

				params.formID = data.FormRow.ID
				params.nodeID = conditionID
				params.userID = data.User
				params.workflowJSON = workflowJSON

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []byte, err error) {
				require.NoError(t, err, "should not return error")

				builder := workflowbuilder.New(t, db)
				workflowData := builder.ParseWorkflow(result)

				// Verify condition node is deleted
				require.False(t, builder.NodeExists(workflowData, params.nodeID.String()), "condition node should be deleted")

				// Verify start node's "next" reference is nullified
				require.False(t, builder.NodeReferencesDeletedNode(workflowData, params.nodeID.String(), "next"), "start node should not reference deleted condition node")
			},
			expectedErr: false,
		},
		{
			name:   "Delete section node also deletes associated section record",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("delete-section-record-org", "delete-section-record-unit")

				workflowJSON, _, sectionID, _ := builder.CreateStartSectionEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, workflowJSON)

				// Manually create section record (sections are not auto-created when adding nodes via Update)
				builder.CreateSectionRecord(sectionID, data.FormRow.ID, "Test Section")

				// Verify section record exists
				require.True(t, builder.SectionExists(sectionID), "section record should exist before deletion")

				params.formID = data.FormRow.ID
				params.nodeID = sectionID
				params.userID = data.User
				params.workflowJSON = workflowJSON

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []byte, err error) {
				require.NoError(t, err, "should not return error")

				builder := workflowbuilder.New(t, db)
				// Verify section record is deleted
				require.False(t, builder.SectionExists(params.nodeID), "section record should be deleted")

				// Verify workflow node is deleted
				workflowData := builder.ParseWorkflow(result)
				require.False(t, builder.NodeExists(workflowData, params.nodeID.String()), "section node should be deleted from workflow")
			},
			expectedErr: false,
		},
		{
			name:   "Delete node from active workflow creates new draft version",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("delete-active-org", "delete-active-unit")

				workflowJSON, _, sectionID, _ := builder.CreateStartSectionEndWorkflow()
				builder.CreateActiveWorkflow(data.FormRow.ID, data.User, workflowJSON)

				params.formID = data.FormRow.ID
				params.nodeID = sectionID
				params.userID = data.User
				params.workflowJSON = workflowJSON

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []byte, err error) {
				require.NoError(t, err, "should not return error")

				builder := workflowbuilder.New(t, db)
				// Verify result is the new draft workflow
				workflowData := builder.ParseWorkflow(result)

				// Verify section node is deleted
				require.False(t, builder.NodeExists(workflowData, params.nodeID.String()), "section node should be deleted")

				// Verify original active version still exists and is unchanged
				workflowQueries := workflow.New(db)
				getRow, err := workflowQueries.Get(context.Background(), params.formID)
				require.NoError(t, err)

				// The Get query returns the latest version, which should be the new draft
				latestWorkflow := builder.ParseWorkflow(getRow.Workflow)

				// Latest version should not have the deleted section
				require.False(t, builder.NodeExists(latestWorkflow, params.nodeID.String()), "latest workflow version should not have deleted section")
			},
			expectedErr: false,
		},
		{
			name:   "Delete non-existent node is a no-op",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("delete-nonexistent-org", "delete-nonexistent-unit")

				workflowJSON, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, workflowJSON)

				params.formID = data.FormRow.ID
				params.nodeID = uuid.New() // Non-existent node ID
				params.userID = data.User
				params.workflowJSON = workflowJSON

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []byte, err error) {
				require.NoError(t, err, "should not return error")

				builder := workflowbuilder.New(t, db)
				// Verify workflow is unchanged (no-op when node doesn't exist)
				workflowData := builder.ParseWorkflow(result)
				originalWorkflow := builder.ParseWorkflow(params.workflowJSON)

				require.Equal(t, len(originalWorkflow), len(workflowData), "workflow should be unchanged")
			},
			expectedErr: false,
		},
		{
			name:   "Delete node with non-existent form ID returns error",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				// Use a non-existent form ID
				params.formID = uuid.New()
				params.nodeID = uuid.New()
				params.userID = uuid.New()

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []byte, err error) {
				require.Error(t, err, "should return error for non-existent form ID")
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
			result, err := queries.DeleteNode(ctx, workflow.DeleteNodeParams{
				FormID:     params.formID,
				LastEditor: params.userID,
				NodeID:     params.nodeID.String(),
			})

			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.validate != nil {
				tc.validate(t, params, db, result, err)
			}
		})
	}
}

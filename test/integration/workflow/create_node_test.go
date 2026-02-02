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

func TestWorkflowService_CreateNode(t *testing.T) {
	type Params struct {
		formID    uuid.UUID
		userID    uuid.UUID
		nodeType  workflow.NodeType
		versionID uuid.UUID // Used to track version IDs for validation
	}

	type testCase struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.CreateNodeRow, err error)
		expectedErr bool
	}

	testCases := []testCase{
		{
			name:   "Create section node in empty workflow",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("create-node-empty-org", "create-node-empty-unit")

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.nodeType = workflow.NodeTypeSection

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.CreateNodeRow, err error) {
				require.NoError(t, err, "should not return error")
				require.NotEqual(t, uuid.Nil, result.NodeID, "node ID should be set")
				require.Equal(t, workflow.NodeTypeSection, result.NodeType, "node type should be section")

				builder := workflowbuilder.New(t, db)
				// Verify workflow contains the new node
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.NodeExists(workflowData, result.NodeID.String()), "workflow should contain the new section node")
				require.True(t, builder.HasNodeType(workflowData, "section"), "workflow should have section node type")

				// Verify section record was created
				require.True(t, builder.SectionExists(result.NodeID), "section record should be created for section node")
			},
			expectedErr: false,
		},
		{
			name:   "Create section node in existing draft workflow",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("create-node-draft-org", "create-node-draft-unit")

				// Create initial draft workflow
				initialWorkflow, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, initialWorkflow)

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.nodeType = workflow.NodeTypeSection

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.CreateNodeRow, err error) {
				require.NoError(t, err, "should not return error")
				require.NotEqual(t, uuid.Nil, result.NodeID, "node ID should be set")
				require.Equal(t, workflow.NodeTypeSection, result.NodeType, "node type should be section")

				builder := workflowbuilder.New(t, db)
				// Verify workflow contains both existing nodes and new node
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.NodeExists(workflowData, result.NodeID.String()), "workflow should contain the new section node")
				require.True(t, builder.HasNodeType(workflowData, "start"), "workflow should still have start node")
				require.True(t, builder.HasNodeType(workflowData, "end"), "workflow should still have end node")
				require.True(t, builder.HasNodeType(workflowData, "section"), "workflow should have section node")

				// Verify section record was created
				require.True(t, builder.SectionExists(result.NodeID), "section record should be created for section node")
			},
			expectedErr: false,
		},
		{
			name:   "Create condition node in existing draft workflow",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("create-condition-draft-org", "create-condition-draft-unit")

				// Create initial draft workflow
				initialWorkflow, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, initialWorkflow)

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.nodeType = workflow.NodeTypeCondition

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.CreateNodeRow, err error) {
				require.NoError(t, err, "should not return error")
				require.NotEqual(t, uuid.Nil, result.NodeID, "node ID should be set")
				require.Equal(t, workflow.NodeTypeCondition, result.NodeType, "node type should be condition")

				builder := workflowbuilder.New(t, db)
				// Verify workflow contains both existing nodes and new node
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.NodeExists(workflowData, result.NodeID.String()), "workflow should contain the new condition node")
				require.True(t, builder.HasNodeType(workflowData, "start"), "workflow should still have start node")
				require.True(t, builder.HasNodeType(workflowData, "end"), "workflow should still have end node")
				require.True(t, builder.HasNodeType(workflowData, "condition"), "workflow should have condition node")

				// Verify no section record was created for condition node
				require.False(t, builder.SectionExists(result.NodeID), "section record should not be created for condition node")
			},
			expectedErr: false,
		},
		{
			name:   "Create section node creates new draft version when latest is active",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("create-node-active-org", "create-node-active-unit")

				// Create and activate initial workflow
				initialWorkflow, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateActiveWorkflow(data.FormRow.ID, data.User, initialWorkflow)

				// Get the active version ID
				getRow, err := data.Queries.Get(context.Background(), data.FormRow.ID)
				require.NoError(t, err)
				activeVersionID := getRow.ID

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.nodeType = workflow.NodeTypeSection
				params.versionID = activeVersionID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.CreateNodeRow, err error) {
				require.NoError(t, err, "should not return error")
				require.NotEqual(t, uuid.Nil, result.NodeID, "node ID should be set")
				require.Equal(t, workflow.NodeTypeSection, result.NodeType, "node type should be section")

				builder := workflowbuilder.New(t, db)
				// Verify workflow contains the new node
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.NodeExists(workflowData, result.NodeID.String()), "workflow should contain the new section node")

				// Verify active version still exists and is unchanged
				queries := workflow.New(db)
				getRow, err := queries.Get(context.Background(), params.formID)
				require.NoError(t, err)
				// Get returns latest by updated_at, which should be the new draft
				require.Equal(t, result.Workflow, getRow.Workflow, "latest version should be the new draft with the node")
				require.False(t, getRow.IsActive, "latest version should be draft")
				require.NotEqual(t, params.versionID, getRow.ID, "new draft version should be different from active version")

				// Verify section record was created
				require.True(t, builder.SectionExists(result.NodeID), "section record should be created for section node")
			},
			expectedErr: false,
		},
		{
			name:   "Create multiple section nodes",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("create-multiple-nodes-org", "create-multiple-nodes-unit")

				// Create initial draft workflow
				initialWorkflow, _, _ := builder.CreateStartEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, initialWorkflow)

				// Create first section node
				_, err := data.Queries.CreateNode(context.Background(), workflow.CreateNodeParams{
					FormID:     data.FormRow.ID,
					LastEditor: data.User,
					Type:       workflow.NodeTypeSection,
				})
				require.NoError(t, err)

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.nodeType = workflow.NodeTypeSection

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.CreateNodeRow, err error) {
				require.NoError(t, err, "should not return error")
				require.NotEqual(t, uuid.Nil, result.NodeID, "node ID should be set")
				require.Equal(t, workflow.NodeTypeSection, result.NodeType, "node type should be section")

				builder := workflowbuilder.New(t, db)
				// Verify workflow contains multiple section nodes
				workflowData := builder.ParseWorkflow(result.Workflow)
				sectionCount := 0
				for _, node := range workflowData {
					nodeType, ok := node["type"].(string)
					if ok && nodeType == string(workflow.NodeTypeSection) {
						sectionCount++
					}
				}
				require.Equal(t, 2, sectionCount, "workflow should have 2 section nodes")

				// Verify section record was created
				require.True(t, builder.SectionExists(result.NodeID), "section record should be created for section node")
			},
			expectedErr: false,
		},
		{
			name:   "Create node with non-existent form ID returns error",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				// Use a non-existent form ID
				params.formID = uuid.New()
				params.userID = uuid.New()
				params.nodeType = workflow.NodeTypeSection

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.CreateNodeRow, err error) {
				require.Error(t, err, "should return error for non-existent form ID")
				require.NotEmpty(t, err.Error(), "error message should not be empty")
			},
			expectedErr: true,
		},
		{
			name:   "Create node with start node type succeeds at query level",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("create-node-invalid-type-org", "create-node-invalid-type-unit")

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.nodeType = workflow.NodeTypeStart

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.CreateNodeRow, err error) {
				// Note: sqlc queries don't validate node types - NodeTypeStart is a valid enum value.
				// The service layer (Service.CreateNode) validates that only 'section' and 'condition' types are allowed.
				// This integration test verifies the query layer behavior.
				require.NoError(t, err, "raw query should succeed with valid enum value")
				require.Equal(t, workflow.NodeTypeStart, result.NodeType, "node type should match input")
			},
			expectedErr: false,
		},
		{
			name:   "Create node with empty node type returns error",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("create-node-empty-type-org", "create-node-empty-type-unit")

				params.formID = data.FormRow.ID
				params.userID = data.User
				// Use empty node type
				params.nodeType = workflow.NodeType("")

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.CreateNodeRow, err error) {
				require.Error(t, err, "should return error for empty node type")
				require.NotEmpty(t, err.Error(), "error message should not be empty")
			},
			expectedErr: true,
		},
		{
			name:   "Create node with unknown node type returns error",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("create-node-unknown-type-org", "create-node-unknown-type-unit")

				params.formID = data.FormRow.ID
				params.userID = data.User
				// Use unknown node type
				params.nodeType = workflow.NodeType("unknown")

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.CreateNodeRow, err error) {
				require.Error(t, err, "should return error for unknown node type")
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
			result, err := queries.CreateNode(ctx, workflow.CreateNodeParams{
				FormID:     params.formID,
				LastEditor: params.userID,
				Type:       params.nodeType,
			})

			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.validate != nil {
				tc.validate(t, params, db, result, err)
			}
		})
	}
}

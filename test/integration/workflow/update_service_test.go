package workflow

import (
	"NYCU-SDC/core-system-backend/internal"
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

// nodeIDString returns n["id"] as a non-empty string or fails the test (avoids panics from type assertions).
func nodeIDString(t *testing.T, n map[string]interface{}) string {
	t.Helper()
	raw, ok := n["id"]
	require.True(t, ok, "node must have id")
	id, ok := raw.(string)
	require.True(t, ok, "node id must be string, got %T", raw)
	require.NotEmpty(t, id)
	return id
}

// assertNoIncomingEdgesFromOthers fails if any node other than targetID references targetID via next / nextTrue / nextFalse.
func assertNoIncomingEdgesFromOthers(t *testing.T, targetID string, byID map[string]map[string]interface{}) {
	t.Helper()
	for fromID, n := range byID {
		if fromID == targetID {
			continue
		}
		for _, key := range []string{"next", "nextTrue", "nextFalse"} {
			ref, ok := n[key].(string)
			if ok && ref != "" && ref == targetID {
				t.Fatalf("expected orphan with no incoming edges, but node %q references %q via %s", fromID, targetID, key)
			}
		}
	}
}

type serviceUpdateParams struct {
	formID        uuid.UUID
	userID        uuid.UUID
	workflowJSON  []byte
	versionID     uuid.UUID
	removedNodeID uuid.UUID
	startNodeID   uuid.UUID
	sectionNodeID uuid.UUID
	endNodeID     uuid.UUID
}

type serviceUpdateTestCase struct {
	name        string
	params      serviceUpdateParams
	setup       func(t *testing.T, params *serviceUpdateParams, db dbbuilder.DBTX) context.Context
	validate    func(t *testing.T, params serviceUpdateParams, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error)
	expectedErr bool
}

// TestWorkflowService_Update exercises workflow.Service.Update (same path as HTTP handlers: validation + persistence).
func TestWorkflowService_Update(t *testing.T) {
	testCases := []serviceUpdateTestCase{
		{
			name:   "Update rejects partial workflow node list (Service validates full graph contract)",
			params: serviceUpdateParams{},
			setup: func(t *testing.T, params *serviceUpdateParams, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("partial-node-set-org", "partial-node-set-unit")

				fullJSON, startID, sectionID, endID := builder.CreateStartSectionEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, fullJSON)

				partialJSON, err := json.Marshal([]map[string]interface{}{
					{
						"id":      startID.String(),
						"type":    "start",
						"label":   "Start",
						"next":    endID.String(),
						"payload": map[string]interface{}{"x": 0.0, "y": 0.0},
					},
					{
						"id":      endID.String(),
						"type":    "end",
						"label":   "End",
						"payload": map[string]interface{}{"x": 0.0, "y": 0.0},
					},
				})
				require.NoError(t, err)

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = partialJSON
				params.removedNodeID = sectionID
				return context.Background()
			},
			validate: func(t *testing.T, params serviceUpdateParams, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, internal.ErrWorkflowValidationFailed)
				require.ErrorContains(t, err, params.removedNodeID.String())
			},
			expectedErr: true,
		},
		{
			name:   "Update with full graph same node ids preserves all nodes on GET (Service)",
			params: serviceUpdateParams{},
			setup: func(t *testing.T, params *serviceUpdateParams, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("full-graph-preserve-org", "full-graph-preserve-unit")

				initialJSON, startID, sectionID, endID := builder.CreateStartSectionEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, initialJSON)

				updatedJSON, err := json.Marshal([]map[string]interface{}{
					{
						"id":      startID.String(),
						"type":    "start",
						"label":   "Start relabeled",
						"next":    sectionID.String(),
						"payload": map[string]interface{}{"x": 1.0, "y": 2.0},
					},
					{
						"id":      sectionID.String(),
						"type":    "section",
						"label":   "Section relabeled",
						"next":    endID.String(),
						"payload": map[string]interface{}{"x": 1.0, "y": 2.0},
					},
					{
						"id":      endID.String(),
						"type":    "end",
						"label":   "End relabeled",
						"payload": map[string]interface{}{"x": 1.0, "y": 2.0},
					},
				})
				require.NoError(t, err)

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = updatedJSON
				params.startNodeID = startID
				params.sectionNodeID = sectionID
				params.endNodeID = endID
				return context.Background()
			},
			validate: func(t *testing.T, params serviceUpdateParams, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.NoError(t, err)
				require.JSONEq(t, string(params.workflowJSON), string(result.Workflow),
					"Service.Update return value should carry the persisted workflow bytes")

				ctx := context.Background()
				row, err := workflow.New(db).Get(ctx, params.formID)
				require.NoError(t, err)
				require.JSONEq(t, string(result.Workflow), string(row.Workflow),
					"GET latest workflow should match what Update returned")

				var nodes []map[string]interface{}
				require.NoError(t, json.Unmarshal(row.Workflow, &nodes))
				require.Len(t, nodes, 3, "GET after full-graph update must still contain all nodes")
				seen := map[string]bool{}
				for _, n := range nodes {
					seen[nodeIDString(t, n)] = true
				}
				require.True(t, seen[params.startNodeID.String()])
				require.True(t, seen[params.sectionNodeID.String()])
				require.True(t, seen[params.endNodeID.String()])

				byID := map[string]map[string]interface{}{}
				for _, n := range nodes {
					byID[nodeIDString(t, n)] = n
				}
				require.Equal(t, "Start relabeled", byID[params.startNodeID.String()]["label"])
				require.Equal(t, "Section relabeled", byID[params.sectionNodeID.String()]["label"])
				require.Equal(t, "End relabeled", byID[params.endNodeID.String()]["label"])
			},
			expectedErr: false,
		},
		{
			name:   "Update succeeds when a node is an orphan (no incoming edges, Service draft)",
			params: serviceUpdateParams{},
			setup: func(t *testing.T, params *serviceUpdateParams, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("unreachable-node-org", "unreachable-node-unit")

				initialJSON, startID, sectionID, endID := builder.CreateStartSectionEndWorkflow()
				builder.CreateDraftWorkflow(data.FormRow.ID, data.User, initialJSON)

				ctx := context.Background()
				getRow, err := data.Queries.Get(ctx, data.FormRow.ID)
				require.NoError(t, err)
				draftVersionID := getRow.ID

				// Same three node ids; start points directly to end. No other node references section (section is an orphan).
				// Section still has `next` because draft validation requires it for section nodes.
				unreachableJSON, err := json.Marshal([]map[string]interface{}{
					{
						"id":      startID.String(),
						"type":    "start",
						"label":   "Start",
						"next":    endID.String(),
						"payload": map[string]interface{}{"x": 0.0, "y": 0.0},
					},
					{
						"id":      sectionID.String(),
						"type":    "section",
						"label":   "Section",
						"next":    endID.String(),
						"payload": map[string]interface{}{"x": 0.0, "y": 0.0},
					},
					{
						"id":      endID.String(),
						"type":    "end",
						"label":   "End",
						"payload": map[string]interface{}{"x": 0.0, "y": 0.0},
					},
				})
				require.NoError(t, err)

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = unreachableJSON
				params.versionID = draftVersionID
				params.startNodeID = startID
				params.sectionNodeID = sectionID
				params.endNodeID = endID
				return context.Background()
			},
			validate: func(t *testing.T, params serviceUpdateParams, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.NoError(t, err)
				require.Equal(t, params.versionID, result.ID, "draft should be updated in place")
				require.False(t, result.IsActive)
				require.Equal(t, 1, countWorkflowVersions(t, db, params.formID))
				require.JSONEq(t, string(params.workflowJSON), string(result.Workflow))

				ctx := context.Background()
				row, err := workflow.New(db).Get(ctx, params.formID)
				require.NoError(t, err)
				require.JSONEq(t, string(result.Workflow), string(row.Workflow))

				var nodes []map[string]interface{}
				require.NoError(t, json.Unmarshal(result.Workflow, &nodes))
				byID := map[string]map[string]interface{}{}
				for _, n := range nodes {
					byID[nodeIDString(t, n)] = n
				}
				require.Contains(t, byID, params.sectionNodeID.String(), "orphan node remains in the workflow JSON")
				assertNoIncomingEdgesFromOthers(t, params.sectionNodeID.String(), byID)
			},
			expectedErr: false,
		},
		{
			name:   "Update when latest is active and only labels change returns same version (Service)",
			params: serviceUpdateParams{},
			setup: func(t *testing.T, params *serviceUpdateParams, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("update-labels-only-org", "update-labels-only-unit")

				initialWorkflow, startID, endID := builder.CreateStartEndWorkflow()
				builder.CreateActiveWorkflow(data.FormRow.ID, data.User, initialWorkflow)

				activeVersionID := builder.GetActiveVersionID(data.FormRow.ID)
				require.Equal(t, 1, countWorkflowVersions(t, db, data.FormRow.ID), "should have exactly one workflow version after activate")

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

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = labelOnlyWorkflow
				params.versionID = activeVersionID
				return context.Background()
			},
			validate: func(t *testing.T, params serviceUpdateParams, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.NoError(t, err)
				require.Equal(t, params.versionID, result.ID, "should return current active version, not create new one")
				require.True(t, result.IsActive, "returned version should still be active")
				require.Equal(t, 1, countWorkflowVersions(t, db, params.formID), "should not create a new workflow version when only labels differ")
			},
			expectedErr: false,
		},
		{
			name:   "Update when latest is active and structure differs creates new draft version (Service)",
			params: serviceUpdateParams{},
			setup: func(t *testing.T, params *serviceUpdateParams, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("update-active-structural-org", "update-active-structural-unit")

				initialJSON, startID, sectionID, endID := builder.CreateStartSectionEndWorkflow()
				builder.CreateActiveWorkflow(data.FormRow.ID, data.User, initialJSON)

				activeVersionID := builder.GetActiveVersionID(data.FormRow.ID)
				require.Equal(t, 1, countWorkflowVersions(t, db, data.FormRow.ID))

				// Same three node ids as activated workflow; start now points to end (structural change vs start→section→end).
				structuralJSON, err := json.Marshal([]map[string]interface{}{
					{
						"id":      startID.String(),
						"type":    "start",
						"label":   "Start",
						"next":    endID.String(),
						"payload": map[string]interface{}{"x": 0.0, "y": 0.0},
					},
					{
						"id":      sectionID.String(),
						"type":    "section",
						"label":   "Section",
						"next":    endID.String(),
						"payload": map[string]interface{}{"x": 0.0, "y": 0.0},
					},
					{
						"id":      endID.String(),
						"type":    "end",
						"label":   "End",
						"payload": map[string]interface{}{"x": 0.0, "y": 0.0},
					},
				})
				require.NoError(t, err)

				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = structuralJSON
				params.versionID = activeVersionID
				return context.Background()
			},
			validate: func(t *testing.T, params serviceUpdateParams, db dbbuilder.DBTX, result workflow.WorkflowVersion, err error) {
				require.NoError(t, err)
				require.NotEqual(t, params.versionID, result.ID, "should create a new workflow version, not return active")
				require.False(t, result.IsActive, "new version should be draft")
				require.Equal(t, 2, countWorkflowVersions(t, db, params.formID))

				ctx := context.Background()
				row, err := workflow.New(db).Get(ctx, params.formID)
				require.NoError(t, err)
				require.Equal(t, result.ID, row.ID, "latest row should be the new draft")
				require.JSONEq(t, string(params.workflowJSON), string(result.Workflow))
			},
			expectedErr: false,
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

			markdownService := markdown.NewService(logger)
			questionService := question.NewService(logger, db, nil, markdownService)
			workflowService := workflow.NewService(logger, db, questionService)
			result, updateErr := workflowService.Update(ctx, params.formID, params.workflowJSON, params.userID)
			require.Equal(t, tc.expectedErr, updateErr != nil, "expected error: %v, got: %v", tc.expectedErr, updateErr)
			if tc.validate != nil {
				tc.validate(t, params, db, result, updateErr)
			}
		})
	}
}

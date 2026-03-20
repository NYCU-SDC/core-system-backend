package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

func TestService_Activate(t *testing.T) {
	t.Parallel()

	type Params struct {
		workflowJSON []byte
	}

	type testCase struct {
		name   string
		params Params
	}

	testCases := []testCase{
		{
			name: "successful activation with simple workflow",
			params: Params{
				workflowJSON: createWorkflow_SimpleValid(t),
			},
		},
		{
			name: "successful activation with complex workflow",
			params: Params{
				workflowJSON: createWorkflow_ComplexValid(t),
			},
		},
		{
			name: "activation preserves node IDs and question IDs",
			params: Params{
				workflowJSON: createWorkflow_ConditionRule(t, uuid.New().String()),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			logger := zap.NewNop()
			tracer := noop.NewTracerProvider().Tracer("test")
			formID := uuid.New()
			userID := uuid.New()

			mockQuerier := new(mockQuerier)
			mockValidator := new(mockValidator)
			service := createTestService(t, logger, tracer, mockQuerier, mockValidator, nil)

			workflowJSON := tc.params.workflowJSON
			mockValidator.On("Activate", mock.Anything, formID, workflowJSON, mock.Anything).Return(nil).Once()

			activatedID := uuid.New()
			mockQuerier.On("Activate", mock.Anything, ActivateParams{
				FormID:     formID,
				LastEditor: userID,
				Workflow:   workflowJSON,
			}).Return(ActivateRow{
				ID:         activatedID,
				FormID:     formID,
				LastEditor: userID,
				IsActive:   true,
				Workflow:   workflowJSON,
			}, nil).Once()

			result, err := service.Activate(ctx, formID, userID, workflowJSON)

			require.NoError(t, err, "unexpected error: %v", err)
			require.NotNilf(t, result.ID, "result.ID is nil")
			require.Equal(t, formID, result.FormID, "formID mismatch")
			require.Equal(t, userID, result.LastEditor, "userID mismatch")
			require.True(t, result.IsActive, "result.IsActive is false")
			require.Equal(t, workflowJSON, result.Workflow, "workflow mismatch")
			// Node IDs, question IDs, and condition rules in the workflow must remain unchanged after activation.
			require.Equal(t, extractNodeIDs(t, workflowJSON), extractNodeIDs(t, result.Workflow), "node IDs must remain unchanged after activate")
			require.Equal(t, extractQuestionIDs(t, workflowJSON), extractQuestionIDs(t, result.Workflow), "question IDs in condition rules must remain unchanged after activate")
			require.Equal(t, extractConditionRules(t, workflowJSON), extractConditionRules(t, result.Workflow), "condition rules in condition nodes must remain unchanged after activate")

			mockValidator.AssertExpectations(t)
			mockQuerier.AssertExpectations(t)
		})
	}
}

func TestService_Update(t *testing.T) {
	t.Parallel()

	type Params struct {
		workflowJSON []byte
	}

	type testCase struct {
		name      string
		params    Params
		expectErr bool
	}

	testCases := []testCase{
		{
			name: "successful update with simple workflow",
			params: Params{
				workflowJSON: createWorkflow_SimpleValid(t),
			},
			expectErr: false,
		},
		{
			name: "draft update preserves version ID and workflow IDs",
			params: Params{
				workflowJSON: createWorkflow_ConditionRule(t, uuid.New().String()),
			},
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			logger := zap.NewNop()
			tracer := noop.NewTracerProvider().Tracer("test")
			formID := uuid.New()
			userID := uuid.New()

			mockQuerier := new(mockQuerier)
			realValidator := NewValidator()
			service := NewServiceForTesting(logger, tracer, mockQuerier, realValidator, nil)
			workflowJSON := tc.params.workflowJSON
			versionID := uuid.New()
			updateRow := UpdateRow{
				ID:         versionID,
				FormID:     formID,
				LastEditor: userID,
				IsActive:   false,
				Workflow:   workflowJSON,
			}

			currentWorkflowRow := WorkflowVersion{
				ID:         versionID,
				FormID:     formID,
				LastEditor: userID,
				IsActive:   false,
				Workflow:   workflowJSON, // Use same workflow for node ID validation to pass
			}
			mockQuerier.On("Get", mock.Anything, formID).Return(currentWorkflowRow, nil).Once()

			mockQuerier.On("Update", mock.Anything, UpdateParams{
				FormID:     formID,
				LastEditor: userID,
				Workflow:   workflowJSON,
			}).Return(updateRow, nil).Once()

			result, err := service.Update(ctx, formID, workflowJSON, userID)

			if tc.expectErr {
				require.Error(t, err, "expected error but got nil")
				mockQuerier.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
			} else {
				require.NoError(t, err, "unexpected error: %v", err)
				expectedVersion := WorkflowVersion(updateRow)
				require.Equal(t, expectedVersion, result)
				// When updating a draft, returned row ID must match current workflow version ID.
				require.Equal(t, currentWorkflowRow.ID, result.ID, "Service.Update should preserve draft version ID")
				// Node IDs, question IDs, and condition rules in the workflow must remain unchanged.
				require.Equal(t, extractNodeIDs(t, workflowJSON), extractNodeIDs(t, result.Workflow), "node IDs must remain unchanged after update")
				require.Equal(t, extractQuestionIDs(t, workflowJSON), extractQuestionIDs(t, result.Workflow), "question IDs in condition rules must remain unchanged after update")
				require.Equal(t, extractConditionRules(t, workflowJSON), extractConditionRules(t, result.Workflow), "condition rules in condition nodes must remain unchanged after update")
				mockQuerier.AssertExpectations(t)
			}
		})
	}
}

// extractNodeIDs returns sorted node IDs from workflow JSON (for comparison in tests).
func extractNodeIDs(t *testing.T, workflowJSON []byte) []string {
	t.Helper()
	var nodes []map[string]interface{}
	require.NoError(t, json.Unmarshal(workflowJSON, &nodes))

	// Extract node IDs from nodes
	var ids []string
	for _, n := range nodes {
		id, ok := n["id"].(string)
		if !ok || id == "" {
			continue
		}
		ids = append(ids, id)
	}

	sort.Strings(ids)
	return ids
}

// extractQuestionIDs returns sorted question IDs from condition rules in workflow JSON.
func extractQuestionIDs(t *testing.T, workflowJSON []byte) []string {
	t.Helper()
	var nodes []map[string]interface{}
	require.NoError(t, json.Unmarshal(workflowJSON, &nodes))

	// Extract question IDs from condition rules
	var ids []string
	for _, n := range nodes {
		// Extract condition rule from node
		cr, ok := n["conditionRule"].(map[string]interface{})
		if !ok || cr == nil {
			continue
		}

		// Extract question ID from condition rule
		q, ok := cr["question"].(string)
		if !ok || q == "" {
			continue
		}
		ids = append(ids, q)
	}
	sort.Strings(ids)
	return ids
}

// conditionRuleEntry holds a condition node ID and its conditionRule for comparison.
type conditionRuleEntry struct {
	NodeID string
	Rule   map[string]interface{}
}

// extractConditionRules returns condition rules from workflow JSON, sorted by node ID, for comparison.
func extractConditionRules(t *testing.T, workflowJSON []byte) []conditionRuleEntry {
	t.Helper()
	var nodes []map[string]interface{}
	require.NoError(t, json.Unmarshal(workflowJSON, &nodes))
	var out []conditionRuleEntry
	for _, n := range nodes {
		nodeID, ok := n["id"].(string)
		if !ok || nodeID == "" {
			continue
		}

		// Extract condition rule from node
		cr, ok := n["conditionRule"].(map[string]interface{})
		if !ok || cr == nil {
			continue
		}

		// Clone so we don't mutate the original
		rule := make(map[string]interface{}, len(cr))
		for questionID, pattern := range cr {
			rule[questionID] = pattern
		}

		// Add condition rule to output
		out = append(out, conditionRuleEntry{
			NodeID: nodeID,
			Rule:   rule,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}

func TestService_CreateNode(t *testing.T) {
	t.Parallel()
	tracer := noop.NewTracerProvider().Tracer("test")

	type Params struct {
		workflowJSON  []byte
		nodeType      NodeType
		questionStore QuestionStore
	}

	type testCase struct {
		name     string
		params   Params
		payload  NodePayload
		setup    func(t *testing.T, mq *mockQuerier, formID, userID uuid.UUID, params Params, payload NodePayload)
		validate func(t *testing.T, mq *mockQuerier, result CreateNodeRow, err error, params Params, payload NodePayload)
	}

	testCases := []testCase{
		{
			name: "invalid node type parameter - start",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      NodeTypeStart,
				questionStore: nil,
			},
			payload: payloadXY(0, 0),
			validate: func(t *testing.T, mq *mockQuerier, result CreateNodeRow, err error, params Params, payload NodePayload) {
				require.Error(t, err, "expected error but got nil")
				require.Equal(t, CreateNodeRow{}, result)
				mq.AssertNotCalled(t, "CreateNode")
			},
		},
		{
			name: "invalid node type parameter - end",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      NodeTypeEnd,
				questionStore: nil,
			},
			payload: payloadXY(0, 0),
			validate: func(t *testing.T, mq *mockQuerier, result CreateNodeRow, err error, params Params, payload NodePayload) {
				require.Error(t, err, "expected error but got nil")
				require.Equal(t, CreateNodeRow{}, result)
				mq.AssertNotCalled(t, "CreateNode")
			},
		},
		{
			name: "invalid node type parameter - empty string",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      NodeType(""),
				questionStore: nil,
			},
			payload: payloadXY(0, 0),
			validate: func(t *testing.T, mq *mockQuerier, result CreateNodeRow, err error, params Params, payload NodePayload) {
				require.Error(t, err, "expected error but got nil")
				require.Equal(t, CreateNodeRow{}, result)
				mq.AssertNotCalled(t, "CreateNode")
			},
		},
		{
			name: "invalid node type parameter - unknown type",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      NodeType("unknown"),
				questionStore: nil,
			},
			payload: payloadXY(0, 0),
			validate: func(t *testing.T, mq *mockQuerier, result CreateNodeRow, err error, params Params, payload NodePayload) {
				require.Error(t, err, "expected error but got nil")
				require.Equal(t, CreateNodeRow{}, result)
				mq.AssertNotCalled(t, "CreateNode")
			},
		},
		{
			name: "missing payload coordinates",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      NodeTypeSection,
				questionStore: nil,
			},
			payload: payloadNil(),
			validate: func(t *testing.T, mq *mockQuerier, result CreateNodeRow, err error, params Params, payload NodePayload) {
				require.Error(t, err, "expected error but got nil")
				require.Equal(t, CreateNodeRow{}, result)
				mq.AssertNotCalled(t, "CreateNode")
			},
		},
		{
			name: "valid workflow - simple section creation",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      NodeTypeSection,
				questionStore: nil,
			},
			payload: payloadXY(0, 0),
			setup: func(t *testing.T, mq *mockQuerier, formID, userID uuid.UUID, params Params, payload NodePayload) {
				expectedRow := CreateNodeRow{
					NodeID:    uuid.New(),
					NodeType:  params.nodeType,
					NodeLabel: nil,
					Workflow:  params.workflowJSON,
				}

				mq.On("CreateNode", mock.Anything, CreateNodeParams{
					FormID:     formID,
					LastEditor: userID,
					Type:       params.nodeType,
					PayloadX:   int32(*payload.X),
					PayloadY:   int32(*payload.Y),
				}).Return(expectedRow, nil).Once()
			},
			validate: func(t *testing.T, mq *mockQuerier, result CreateNodeRow, err error, params Params, payload NodePayload) {
				require.NoError(t, err, "unexpected error: %v", err)
				require.NotEqual(t, uuid.Nil, result.NodeID)
				require.Equal(t, params.nodeType, result.NodeType)
				require.NotEmpty(t, result.Workflow)
				mq.AssertExpectations(t)
			},
		},
		{
			name: "valid workflow - condition node creation",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      NodeTypeCondition,
				questionStore: nil,
			},
			payload: payloadXY(0, 0),
			setup: func(t *testing.T, mq *mockQuerier, formID, userID uuid.UUID, params Params, payload NodePayload) {
				expectedRow := CreateNodeRow{
					NodeID:    uuid.New(),
					NodeType:  params.nodeType,
					NodeLabel: nil,
					Workflow:  params.workflowJSON,
				}

				mq.On("CreateNode", mock.Anything, CreateNodeParams{
					FormID:     formID,
					LastEditor: userID,
					Type:       params.nodeType,
					PayloadX:   int32(*payload.X),
					PayloadY:   int32(*payload.Y),
				}).Return(expectedRow, nil).Once()
			},
			validate: func(t *testing.T, mq *mockQuerier, result CreateNodeRow, err error, params Params, payload NodePayload) {
				require.NoError(t, err, "unexpected error: %v", err)
				require.NotEqual(t, uuid.Nil, result.NodeID)
				require.Equal(t, params.nodeType, result.NodeType)
				require.NotEmpty(t, result.Workflow)
				mq.AssertExpectations(t)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			logger := zap.NewNop()
			formID := uuid.New()
			userID := uuid.New()

			mockQuerier := new(mockQuerier)
			realValidator := NewValidator()

			service := NewServiceForTesting(logger, tracer, mockQuerier, realValidator, tc.params.questionStore)

			if tc.setup != nil {
				tc.setup(t, mockQuerier, formID, userID, tc.params, tc.payload)
			}

			result, err := service.CreateNode(ctx, formID, tc.params.nodeType, tc.payload, userID)

			if tc.validate != nil {
				tc.validate(t, mockQuerier, result, err, tc.params, tc.payload)
			}
		})
	}
}

func payloadXY(x, y int) NodePayload {
	return NodePayload{X: &x, Y: &y}
}

func payloadNil() NodePayload {
	return NodePayload{X: nil, Y: nil}
}

func TestService_Get(t *testing.T) {
	t.Parallel()
	tracer := noop.NewTracerProvider().Tracer("test")

	type testCase struct {
		name      string
		formID    uuid.UUID
		setupMock func(*mockQuerier, uuid.UUID)
		expectErr bool
	}

	testCases := []testCase{
		{
			name:   "successful get",
			formID: uuid.New(),
			setupMock: func(mq *mockQuerier, formID uuid.UUID) {
				expectedRow := WorkflowVersion{
					ID:         uuid.New(),
					FormID:     formID,
					LastEditor: uuid.New(),
					IsActive:   false,
					Workflow:   createWorkflow_SimpleValid(t),
				}
				mq.On("Get", mock.Anything, formID).Return(expectedRow, nil).Once()
			},
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			logger := zap.NewNop()
			mockQuerier := new(mockQuerier)
			mockValidator := new(mockValidator)
			service := createTestService(t, logger, tracer, mockQuerier, mockValidator, nil)

			tc.setupMock(mockQuerier, tc.formID)

			result, err := service.Get(ctx, tc.formID)

			if tc.expectErr {
				require.Error(t, err)
				require.Equal(t, WorkflowVersion{}, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result.ID)
				require.Equal(t, tc.formID, result.FormID)
			}

			mockQuerier.AssertExpectations(t)
		})
	}
}

func TestService_DeleteNode(t *testing.T) {
	t.Parallel()

	type Params struct {
		workflowJSON  []byte
		nodeID        uuid.UUID
		questionStore QuestionStore
	}

	type testCase struct {
		name      string
		params    Params
		expectErr bool
	}

	testCases := []testCase{
		{
			name: "valid workflow - simple workflow after deletion",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeID:        uuid.New(),
				questionStore: nil,
			},
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			logger := zap.NewNop()
			tracer := noop.NewTracerProvider().Tracer("test")
			formID := uuid.New()
			userID := uuid.New()

			mockQuerier := new(mockQuerier)
			realValidator := NewValidator()
			service := NewServiceForTesting(logger, tracer, mockQuerier, realValidator, tc.params.questionStore)

			workflowJSON := tc.params.workflowJSON

			mockQuerier.On("DeleteNode", mock.Anything, DeleteNodeParams{
				FormID:     formID,
				LastEditor: userID,
				NodeID:     tc.params.nodeID.String(),
			}).Return(workflowJSON, nil).Once()

			result, err := service.DeleteNode(ctx, formID, tc.params.nodeID, userID)

			if tc.expectErr {
				require.Error(t, err, "expected error but got nil")
				mockQuerier.AssertExpectations(t)
			} else {
				require.NoError(t, err, "unexpected error: %v", err)
				require.Equal(t, workflowJSON, result)
				mockQuerier.AssertExpectations(t)
			}
		})
	}
}

// TestService_GetWorkflow_ValidationErrors tests the parseValidationErrors function
// using mocked errors to verify edge cases in error parsing logic.
func TestService_GetWorkflow_ValidationErrors(t *testing.T) {
	t.Parallel()
	tracer := noop.NewTracerProvider().Tracer("test")

	type testCase struct {
		name            string
		formID          uuid.UUID
		workflowJSON    []byte
		setupMock       func(*mockValidator, uuid.UUID, []byte)
		expectedInfoLen int
		expectedErr     bool
	}

	testCases := []testCase{
		{
			name:         "validation passes - returns empty info array",
			formID:       uuid.New(),
			workflowJSON: createWorkflow_SimpleValid(t),
			setupMock: func(mv *mockValidator, formID uuid.UUID, workflow []byte) {
				mv.On("Activate", mock.Anything, formID, workflow, mock.Anything).Return(nil).Once()
			},
			expectedInfoLen: 0,
			expectedErr:     false,
		},
		{
			name:         "parsing - nested joined errors",
			formID:       uuid.New(),
			workflowJSON: createWorkflow_SimpleValid(t),
			setupMock: func(mv *mockValidator, formID uuid.UUID, workflow []byte) {
				startID := uuid.New()
				err1 := fmt.Errorf("start node '%s' must have a 'next' field", startID.String())
				err2 := fmt.Errorf("workflow must contain exactly one start node, found 0")
				err3 := fmt.Errorf("workflow must contain exactly one end node, found 0")
				innerErr := errors.Join(err2, err3)
				outerErr := fmt.Errorf("workflow validation failed: %w", errors.Join(err1, innerErr))
				mv.On("Activate", mock.Anything, formID, workflow, mock.Anything).Return(outerErr).Once()
			},
			expectedInfoLen: 3, // 3 lines: 1 with node ID, 2 without
			expectedErr:     false,
		},
		{
			name:         "parsing - multiple unreachable nodes with individual node IDs",
			formID:       uuid.New(),
			workflowJSON: createWorkflow_SimpleValid(t),
			setupMock: func(mv *mockValidator, formID uuid.UUID, workflow []byte) {
				unreachableID1 := uuid.New()
				unreachableID2 := uuid.New()
				err1 := fmt.Errorf("node '%s' is unreachable from the start node", unreachableID1.String())
				err2 := fmt.Errorf("node '%s' is unreachable from the start node", unreachableID2.String())
				graphErr := fmt.Errorf("graph validation failed: %w", errors.Join(err1, err2))
				outerErr := fmt.Errorf("workflow validation failed: %w", graphErr)
				mv.On("Activate", mock.Anything, formID, workflow, mock.Anything).Return(outerErr).Once()
			},
			expectedInfoLen: 2, // 2 unique node IDs, each gets its own ValidationInfo with the same full message
			expectedErr:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			logger := zap.NewNop()
			mockQuerier := new(mockQuerier)
			mockValidator := new(mockValidator)
			service := createTestService(t, logger, tracer, mockQuerier, mockValidator, nil)

			tc.setupMock(mockValidator, tc.formID, tc.workflowJSON)

			validationInfos, err := service.GetValidationInfo(ctx, tc.formID, tc.workflowJSON)

			if tc.expectedErr {
				require.Error(t, err)
				require.Nil(t, validationInfos)
			} else {
				require.NoError(t, err)
				require.NotNil(t, validationInfos)
				require.Len(t, validationInfos, tc.expectedInfoLen)

				// Verify that node IDs are extracted correctly when present
				for _, info := range validationInfos {
					if info.NodeID != nil {
						_, parseErr := uuid.Parse(*info.NodeID)
						require.NoError(t, parseErr, "extracted node ID should be a valid UUID")
					}
					require.NotEmpty(t, info.Message)
				}
			}

			mockValidator.AssertExpectations(t)
		})
	}
}

package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/workflow"

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

			mockValidator.On("Activate", mock.Anything, formID, tc.params.workflowJSON, mock.Anything).Return(nil).Once()

			mockQuerier.On("Activate", mock.Anything, workflow.ActivateParams{
				FormID:     formID,
				LastEditor: userID,
				Workflow:   tc.params.workflowJSON,
			}).Return(workflow.ActivateRow{
				ID:         uuid.New(),
				FormID:     formID,
				LastEditor: userID,
				IsActive:   true,
				Workflow:   tc.params.workflowJSON,
			}, nil).Once()

			result, err := service.Activate(ctx, formID, userID, tc.params.workflowJSON)

			require.NoError(t, err, "unexpected error: %v", err)
			require.NotNilf(t, result.ID, "result.ID is nil")
			require.Equal(t, formID, result.FormID, "formID mismatch")
			require.Equal(t, userID, result.LastEditor, "userID mismatch")
			require.True(t, result.IsActive, "result.IsActive is false")
			require.Equal(t, tc.params.workflowJSON, result.Workflow, "workflow mismatch")

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
			realValidator := workflow.NewValidator()
			service := workflow.NewServiceForTesting(logger, tracer, mockQuerier, realValidator, nil)

			workflowJSON := tc.params.workflowJSON
			expectedRow := workflow.UpdateRow{
				FormID:     formID,
				LastEditor: userID,
				Workflow:   workflowJSON,
			}

			currentWorkflowRow := workflow.GetRow{
				ID:         uuid.New(),
				FormID:     formID,
				LastEditor: userID,
				IsActive:   false,
				Workflow:   workflowJSON, // Use same workflow for node ID validation to pass
			}
			mockQuerier.On("Get", mock.Anything, formID).Return(currentWorkflowRow, nil).Once()

			mockQuerier.On("Update", mock.Anything, workflow.UpdateParams{
				FormID:     formID,
				LastEditor: userID,
				Workflow:   workflowJSON,
			}).Return(expectedRow, nil).Once()

			result, err := service.Update(ctx, formID, workflowJSON, userID)

			if tc.expectErr {
				require.Error(t, err, "expected error but got nil")
				mockQuerier.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
			} else {
				require.NoError(t, err, "unexpected error: %v", err)
				require.Equal(t, expectedRow, result)
				mockQuerier.AssertExpectations(t)
			}
		})
	}
}

func TestService_CreateNode(t *testing.T) {
	t.Parallel()

	type Params struct {
		workflowJSON  []byte
		nodeType      workflow.NodeType
		questionStore workflow.QuestionStore
	}

	type testCase struct {
		name      string
		params    Params
		expectErr bool
	}

	testCases := []testCase{
		{
			name: "invalid node type parameter - start",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      workflow.NodeTypeStart,
				questionStore: nil,
			},
			expectErr: true,
		},
		{
			name: "invalid node type parameter - end",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      workflow.NodeTypeEnd,
				questionStore: nil,
			},
			expectErr: true,
		},
		{
			name: "invalid node type parameter - empty string",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      workflow.NodeType(""),
				questionStore: nil,
			},
			expectErr: true,
		},
		{
			name: "invalid node type parameter - unknown type",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      workflow.NodeType("unknown"),
				questionStore: nil,
			},
			expectErr: true,
		},
		{
			name: "valid workflow - simple section creation",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      workflow.NodeTypeSection,
				questionStore: nil,
			},
			expectErr: false,
		},
		{
			name: "valid workflow - condition node creation",
			params: Params{
				workflowJSON:  createWorkflow_SimpleValid(t),
				nodeType:      workflow.NodeTypeCondition,
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
			realValidator := workflow.NewValidator()

			service := workflow.NewServiceForTesting(logger, tracer, mockQuerier, realValidator, tc.params.questionStore)

			// Only set up mock if node type is valid (service will call querier)
			// Note: CreateNode calls the querier BEFORE validation, so we need to set up the mock
			// for all valid node types, even when validation will fail
			switch tc.params.nodeType {
			case workflow.NodeTypeSection, workflow.NodeTypeCondition:
				expectedRow := workflow.CreateNodeRow{
					NodeID:    uuid.New(),
					NodeType:  tc.params.nodeType,
					NodeLabel: nil,
					Workflow:  tc.params.workflowJSON,
				}

				mockQuerier.On("CreateNode", mock.Anything, workflow.CreateNodeParams{
					FormID:     formID,
					LastEditor: userID,
					Type:       tc.params.nodeType,
				}).Return(expectedRow, nil).Once()
			}

			result, err := service.CreateNode(ctx, formID, tc.params.nodeType, userID)

			if tc.expectErr {
				require.Error(t, err, "expected error but got nil")
				// For invalid node types, querier should not be called
				switch tc.params.nodeType {
				case workflow.NodeTypeSection, workflow.NodeTypeCondition:
					mockQuerier.AssertExpectations(t)
				default:
					mockQuerier.AssertNotCalled(t, "CreateNode")
				}
			} else {
				require.NoError(t, err, "unexpected error: %v", err)
				require.NotNil(t, result)
				mockQuerier.AssertExpectations(t)
			}
		})
	}
}

func TestService_Get(t *testing.T) {
	t.Parallel()

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
				expectedRow := workflow.GetRow{
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
			tracer := noop.NewTracerProvider().Tracer("test")
			mockQuerier := new(mockQuerier)
			mockValidator := new(mockValidator)
			service := createTestService(t, logger, tracer, mockQuerier, mockValidator, nil)

			tc.setupMock(mockQuerier, tc.formID)

			result, err := service.Get(ctx, tc.formID)

			if tc.expectErr {
				require.Error(t, err)
				require.Equal(t, workflow.GetRow{}, result)
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
		questionStore workflow.QuestionStore
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
			realValidator := workflow.NewValidator()

			service := workflow.NewServiceForTesting(logger, tracer, mockQuerier, realValidator, tc.params.questionStore)

			workflowJSON := tc.params.workflowJSON

			mockQuerier.On("DeleteNode", mock.Anything, workflow.DeleteNodeParams{
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
			tracer := noop.NewTracerProvider().Tracer("test")
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

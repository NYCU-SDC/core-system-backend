package workflow_test

import (
	"context"
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/workflow"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestActivate_InvalidNodeReferences tests that validateGraphConnectivity catches invalid node references
func TestActivate_InvalidNodeReferences(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name         string
		workflowJSON []byte
	}

	testCases := []testCase{
		{
			name:         "node references non-existent node ID in next field",
			workflowJSON: createWorkflow_InvalidNextRef(t),
		},
		{
			name:         "condition node references non-existent node ID in nextTrue field",
			workflowJSON: createWorkflow_InvalidNextTrueRef(t),
		},
		{
			name:         "condition node references non-existent node ID in nextFalse field",
			workflowJSON: createWorkflow_InvalidNextFalseRef(t),
		},
		{
			name:         "condition node references non-existent nodes in both nextTrue and nextFalse",
			workflowJSON: createWorkflow_InvalidConditionRefs(t),
		},
	}

	validator := workflow.NewValidator()
	ctx := context.Background()
	formID := uuid.New()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validator.Activate(ctx, formID, tc.workflowJSON, nil)

			require.Error(t, err, "expected validation error")
		})
	}
}

// TestActivate_ConditionRuleValidation tests strict condition rule validation
func TestActivate_ConditionRuleValidation(t *testing.T) {
	t.Parallel()

	formID := uuid.New()
	otherFormID := uuid.New()

	type testCase struct {
		name        string
		setup       func() ([]byte, workflow.QuestionStore)
		expectedErr bool
	}

	testCases := []testCase{
		{
			name: "condition rule with non-existent question ID",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				return createWorkflow_ConditionRule(t, questionID),
					&mockQuestionStore{questions: make(map[uuid.UUID]question.Answerable)}
			},
			expectedErr: true,
		},
		{
			name: "condition rule with question from different form",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRule(t, questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, otherFormID, question.QuestionTypeShortText),
						},
					}
			},
			expectedErr: true,
		},
		{
			name: "condition rule with source=choice but question type is short_text",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "choice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeShortText),
						},
					}
			},
			expectedErr: true,
		},
		{
			name: "condition rule with source=nonChoice but question type is single_choice",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
						},
					}
			},
			expectedErr: true,
		},
		{
			name: "valid condition rule with source=choice and single_choice question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "choice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
						},
					}
			},
			expectedErr: false,
		},
		{
			name: "valid condition rule with source=choice and multiple_choice question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "choice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeMultipleChoice),
						},
					}
			},
			expectedErr: false,
		},
		{
			name: "valid condition rule with source=nonChoice and short_text question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeShortText),
						},
					}
			},
			expectedErr: false,
		},
		{
			name: "valid condition rule with source=nonChoice and long_text question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeLongText),
						},
					}
			},
			expectedErr: false,
		},
		{
			name: "valid condition rule with source=nonChoice and date question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeDate),
						},
					}
			},
			expectedErr: false,
		},
		// Choice types: dropdown, detailed_multiple_choice, ranking
		{
			name: "valid condition rule with source=choice and dropdown question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "choice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeDropdown),
						},
					}
			},
			expectedErr: false,
		},
		{
			name: "valid condition rule with source=choice and detailed_multiple_choice question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "choice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeDetailedMultipleChoice),
						},
					}
			},
			expectedErr: false,
		},
		{
			name: "valid condition rule with source=choice and ranking question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "choice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeRanking),
						},
					}
			},
			expectedErr: false,
		},
		// NonChoice types: hyperlink, upload_file, oauth_connect, linear_scale, rating
		{
			name: "valid condition rule with source=nonChoice and hyperlink question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeHyperlink),
						},
					}
			},
			expectedErr: false,
		},
		{
			name: "valid condition rule with source=nonChoice and upload_file question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeUploadFile),
						},
					}
			},
			expectedErr: false,
		},
		{
			name: "valid condition rule with source=nonChoice and oauth_connect question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeOauthConnect),
						},
					}
			},
			expectedErr: false,
		},
		{
			name: "valid condition rule with source=nonChoice and linear_scale question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeLinearScale),
						},
					}
			},
			expectedErr: false,
		},
		{
			name: "valid condition rule with source=nonChoice and rating question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflow_ConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeRating),
						},
					}
			},
			expectedErr: false,
		},
	}

	validator := workflow.NewValidator()
	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workflowJSON, questionStore := tc.setup()
			err := validator.Activate(ctx, formID, workflowJSON, questionStore)

			if tc.expectedErr {
				require.Error(t, err, "expected validation error but got nil")
			} else {
				require.NoError(t, err, "expected validation to pass but got error: %v", err)
			}
		})
	}
}

// TestValidateUpdateNodeIDs tests the ValidateUpdateNodeIDs method
func TestValidateUpdateNodeIDs(t *testing.T) {
	t.Parallel()

	validator := workflow.NewValidator()
	ctx := context.Background()

	type testCase struct {
		name        string
		setup       func(*testing.T) ([]byte, []byte)
		expectedErr bool
	}

	testCases := []testCase{
		{
			name: "first update - nil current workflow",
			setup: func(t *testing.T) ([]byte, []byte) {
				return nil, createWorkflow_SimpleForNodeIDTest(t)
			},
			expectedErr: false,
		},
		{
			name: "node IDs unchanged - same workflow",
			setup: func(t *testing.T) ([]byte, []byte) {
				// Create once and reuse for both to ensure same IDs
				workflow := createWorkflow_SimpleForNodeIDTest(t)
				return workflow, workflow
			},
			expectedErr: false,
		},
		{
			name: "node IDs unchanged - workflow with same IDs but different properties",
			setup: func(t *testing.T) ([]byte, []byte) {
				// Create workflow with section and extract IDs
				currentWorkflow := createWorkflow_WithSection(t)
				var currentNodes []map[string]interface{}
				require.NoError(t, json.Unmarshal(currentWorkflow, &currentNodes))

				// Extract IDs from current workflow
				startID := currentNodes[0]["id"].(string)
				sectionID := currentNodes[1]["id"].(string)
				endID := currentNodes[2]["id"].(string)

				// Create modified workflow with same IDs
				modifiedWorkflow := createWorkflowJSON(t, []map[string]interface{}{
					{
						"id":    startID,
						"type":  "start",
						"label": "Start Modified",
						"next":  sectionID,
					},
					{
						"id":    sectionID,
						"type":  "section",
						"label": "Section Modified",
						"next":  endID,
					},
					{
						"id":    endID,
						"type":  "end",
						"label": "End Modified",
					},
				})
				return currentWorkflow, modifiedWorkflow
			},
			expectedErr: false,
		},
		{
			name: "error - node ID removed",
			setup: func(t *testing.T) ([]byte, []byte) {
				return createWorkflow_WithSection(t), createWorkflow_SimpleForNodeIDTest(t)
			},
			expectedErr: true,
		},
		{
			name: "error - node ID added",
			setup: func(t *testing.T) ([]byte, []byte) {
				return createWorkflow_SimpleForNodeIDTest(t), createWorkflow_WithSection(t)
			},
			expectedErr: true,
		},
		{
			name: "error - multiple node IDs removed",
			setup: func(t *testing.T) ([]byte, []byte) {
				return createWorkflow_MultipleNodes(t), createWorkflow_SimpleForNodeIDTest(t)
			},
			expectedErr: true,
		},
		{
			name: "error - multiple node IDs added",
			setup: func(t *testing.T) ([]byte, []byte) {
				return createWorkflow_SimpleForNodeIDTest(t), createWorkflow_MultipleNodes(t)
			},
			expectedErr: true,
		},
		{
			name: "error - invalid current workflow JSON",
			setup: func(t *testing.T) ([]byte, []byte) {
				return []byte(`{invalid json}`), createWorkflow_SimpleForNodeIDTest(t)
			},
			expectedErr: true,
		},
		{
			name: "error - invalid new workflow JSON",
			setup: func(t *testing.T) ([]byte, []byte) {
				return createWorkflow_SimpleForNodeIDTest(t), []byte(`{invalid json}`)
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			currentWorkflow, newWorkflow := tc.setup(t)
			err := validator.ValidateUpdateNodeIDs(ctx, currentWorkflow, newWorkflow)

			if tc.expectedErr {
				require.Error(t, err, "expected validation error")
			} else {
				require.NoError(t, err, "expected validation to pass but got error: %v", err)
			}
		})
	}
}

// TestValidate tests the Validate method which should reuse the same validation logic as Activate
func TestValidate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name        string
		setup       func() ([]byte, workflow.QuestionStore)
		expectedErr bool
	}

	formID := uuid.New()

	testCases := []testCase{
		{
			name:        "valid workflow",
			setup:       func() ([]byte, workflow.QuestionStore) { return createWorkflow_ValidWithEmptyStore(t) },
			expectedErr: false,
		},
		{
			name:        "invalid workflow - missing start node",
			setup:       func() ([]byte, workflow.QuestionStore) { return createWorkflow_MissingStartNode(t) },
			expectedErr: true,
		},
		{
			name:        "invalid workflow - duplicate node IDs",
			setup:       func() ([]byte, workflow.QuestionStore) { return createWorkflow_DuplicateNodeIDs(t) },
			expectedErr: true,
		},
		{
			name:        "unreachable node",
			setup:       func() ([]byte, workflow.QuestionStore) { return createWorkflow_UnreachableNode(t) },
			expectedErr: false,
		},
		{
			name:        "invalid workflow - invalid node reference",
			setup:       func() ([]byte, workflow.QuestionStore) { return createWorkflow_InvalidNextRefWithStore(t) },
			expectedErr: true,
		},
		{
			name: "invalid workflow - condition rule with non-existent question",
			setup: func() ([]byte, workflow.QuestionStore) {
				return createWorkflow_ConditionRuleWithEmptyStore(t, uuid.New().String())
			},
			expectedErr: true,
		},
	}

	validator := workflow.NewValidator()
	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workflowJSON, questionStore := tc.setup()
			err := validator.Validate(ctx, formID, workflowJSON, questionStore)

			if tc.expectedErr {
				require.Error(t, err, "expected validation error")
			} else {
				require.NoError(t, err, "expected validation to pass but got error: %v", err)
			}
		})
	}
}

// TestValidateDraft_ConditionSectionOrder tests that Validate detects when a condition
// references a section that comes after it in the graph traversal.
func TestValidateDraft_ConditionSectionOrder(t *testing.T) {
	t.Parallel()

	formID := uuid.New()

	type testCase struct {
		name        string
		setup       func(t *testing.T) ([]byte, workflow.QuestionStore)
		expectedErr bool
	}

	testCases := []testCase{
		{
			name: "error - condition references section that comes after it",
			setup: func(t *testing.T) ([]byte, workflow.QuestionStore) {
				return createWorkflow_ConditionRefsSectionAfter(t, formID)
			},
			expectedErr: true,
		},
		{
			name: "valid - condition references section that comes before it",
			setup: func(t *testing.T) ([]byte, workflow.QuestionStore) {
				return createWorkflow_ConditionRefsSectionBefore(t, formID)
			},
			expectedErr: false,
		},
		{
			name: "error - condition references itself via nodeId",
			setup: func(t *testing.T) ([]byte, workflow.QuestionStore) {
				return createWorkflow_ConditionRefsSelf(t, formID)
			},
			expectedErr: true,
		},
		{
			name:        "valid - condition without conditionRule",
			setup:       func(t *testing.T) ([]byte, workflow.QuestionStore) { return createWorkflow_ConditionNoRule(t) },
			expectedErr: false,
		},
		{
			name: "error - multiple conditions with invalid section references",
			setup: func(t *testing.T) ([]byte, workflow.QuestionStore) {
				return createWorkflow_MultipleConditionsInvalidSectionRefs(t, formID)
			},
			expectedErr: true,
		},
	}

	validator := workflow.NewValidator()
	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workflowJSON, questionStore := tc.setup(t)
			err := validator.Validate(ctx, formID, workflowJSON, questionStore)

			if tc.expectedErr {
				require.Error(t, err, "expected validation error")
			} else {
				require.NoError(t, err, "expected validation to pass but got error: %v", err)
			}
		})
	}
}

package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/question"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func createWorkflow_SimpleValid_WithoutNodePayload(t *testing.T) []byte {
	t.Helper()

	startID := uuid.New()
	endID := uuid.New()

	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(t, err)
	return workflowJSON
}

// TestActivate tests that Activate performs strict workflow validation.
func TestActivate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name         string
		workflowJSON []byte
		expectedErr  bool
	}

	testCases := []testCase{
		{
			name:         "node references non-existent node ID in next field",
			workflowJSON: createWorkflow_InvalidNextRef(t),
			expectedErr:  true,
		},
		{
			name:         "condition node references non-existent node ID in nextTrue field",
			workflowJSON: createWorkflow_InvalidNextTrueRef(t),
			expectedErr:  true,
		},
		{
			name:         "condition node references non-existent node ID in nextFalse field",
			workflowJSON: createWorkflow_InvalidNextFalseRef(t),
			expectedErr:  true,
		},
		{
			name:         "condition node references non-existent nodes in both nextTrue and nextFalse",
			workflowJSON: createWorkflow_InvalidConditionRefs(t),
			expectedErr:  true,
		},
		{
			name:         "valid payload object",
			workflowJSON: createWorkflow_SimpleValid_WithNodePayload(t, map[string]interface{}{"x": 1, "y": 2}),
			expectedErr:  false,
		},
		{
			name:         "missing payload field",
			workflowJSON: createWorkflow_SimpleValid_WithoutNodePayload(t),
			expectedErr:  true,
		},
		{
			name:         "invalid payload type (string)",
			workflowJSON: createWorkflow_SimpleValid_WithNodePayload(t, "not-an-object"),
			expectedErr:  true,
		},
		{
			name:         "invalid payload shape (missing y)",
			workflowJSON: createWorkflow_SimpleValid_WithNodePayload(t, map[string]interface{}{"x": 1}),
			expectedErr:  true,
		},
		{
			name:         "invalid payload shape (x out of float64 range)",
			workflowJSON: createWorkflow_SimpleValid_WithNodePayload(t, map[string]interface{}{"x": json.Number("1e1000"), "y": json.Number("2")}),
			expectedErr:  true,
		},
		{
			name:         "invalid payload shape (y out of float64 range)",
			workflowJSON: createWorkflow_SimpleValid_WithNodePayload(t, map[string]interface{}{"x": json.Number("1"), "y": json.Number("1e1000")}),
			expectedErr:  true,
		},
		{
			name:         "valid payload shape (float64 max value)",
			workflowJSON: createWorkflow_SimpleValid_WithNodePayload(t, map[string]interface{}{"x": json.Number("1.7976931348623157e+308"), "y": json.Number("2")}),
			expectedErr:  false,
		},
	}

	validator := NewValidator()
	ctx := context.Background()
	formID := uuid.New()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validator.Activate(ctx, formID, tc.workflowJSON, nil)

			if tc.expectedErr {
				require.Error(t, err, "expected validation error")
				return
			}
			require.NoError(t, err, "expected validation to pass")
		})
	}
}

// TestValidateConditionSectionOrder_ConditionBranchTraversalWithMissingSiblingBranch tests that Validate detects when a condition
// references a section that comes after it in the graph traversal.
func TestValidateConditionSectionOrder_ConditionBranchTraversalWithMissingSiblingBranch(t *testing.T) {
	t.Parallel()

	formID := uuid.New()
	startID := uuid.New()
	conditionAID := uuid.New()
	conditionBID := uuid.New()
	sectionID := uuid.New()
	endID := uuid.New()
	questionID := uuid.New()

	answerable, err := question.NewAnswerable(question.Question{
		ID:        questionID,
		SectionID: sectionID,
		Required:  false,
		Type:      question.QuestionTypeShortText,
		Title:     pgtype.Text{String: "Q1", Valid: true},
		Metadata:  []byte("{}"),
		Order:     1,
	}, formID)
	require.NoError(t, err)

	store := &mockQuestionStore{
		questions: map[uuid.UUID]question.Answerable{
			questionID: answerable,
		},
	}

	nodes := []map[string]interface{}{
		{"id": startID.String(), "type": "start", "label": "Start", "next": conditionAID.String()},
		{"id": conditionAID.String(), "type": "condition", "label": "Condition A", "nextTrue": conditionBID.String()},
		{
			"id":        conditionBID.String(),
			"type":      "condition",
			"label":     "Condition B",
			"nextTrue":  endID.String(),
			"nextFalse": endID.String(),
			"conditionRule": map[string]interface{}{
				"question": questionID.String(),
			},
		},
		{"id": sectionID.String(), "type": "section", "label": "Section"},
		{"id": endID.String(), "type": "end", "label": "End"},
	}

	err = validateConditionSectionOrder(context.Background(), formID, nodes, store)
	require.Error(t, err)
	require.Contains(t, err.Error(), conditionBID.String())
}

// TestActivate_ConditionRuleValidation tests strict condition rule validation
func TestActivate_ConditionRuleValidation(t *testing.T) {
	t.Parallel()

	formID := uuid.New()
	otherFormID := uuid.New()

	type testCase struct {
		name        string
		setup       func() ([]byte, QuestionStore)
		expectedErr bool
	}

	testCases := []testCase{
		{
			name: "condition rule with non-existent question ID",
			setup: func() ([]byte, QuestionStore) {
				questionID := uuid.New().String()
				return createWorkflow_ConditionRule(t, questionID),
					&mockQuestionStore{questions: make(map[uuid.UUID]question.Answerable)}
			},
			expectedErr: true,
		},
		{
			name: "condition rule with question from different form",
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
			setup: func() ([]byte, QuestionStore) {
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
		{
			name: "condition rule with source=choice and pattern referencing non-existent choice option",
			setup: func() ([]byte, QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				// Pattern contains a UUID that is not one of the question's choice option IDs
				nonExistentChoiceID := uuid.New()
				pattern := "^" + nonExistentChoiceID.String() + "$"
				return createWorkflow_ConditionRuleWithPattern(t, questionID, pattern),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
						},
					}
			},
			expectedErr: true,
		},
		{
			name: "condition rule with source=choice and pattern referencing valid choice option",
			setup: func() ([]byte, QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				choiceID := uuid.New()
				pattern := "^" + choiceID.String() + "$"
				return createWorkflow_ConditionRuleWithPattern(t, questionID, pattern),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerableWithChoiceIDs(t, formID, question.QuestionTypeSingleChoice, []uuid.UUID{choiceID}),
						},
					}
			},
			expectedErr: false,
		},
	}

	validator := NewValidator()
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

	validator := NewValidator()
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
		setup       func() ([]byte, QuestionStore)
		expectedErr bool
	}

	formID := uuid.New()

	testCases := []testCase{
		{
			name:        "valid workflow",
			setup:       func() ([]byte, QuestionStore) { return createWorkflow_ValidWithEmptyStore(t) },
			expectedErr: false,
		},
		{
			name:        "invalid workflow - missing start node",
			setup:       func() ([]byte, QuestionStore) { return createWorkflow_MissingStartNode(t) },
			expectedErr: true,
		},
		{
			name:        "invalid workflow - duplicate node IDs",
			setup:       func() ([]byte, QuestionStore) { return createWorkflow_DuplicateNodeIDs(t) },
			expectedErr: true,
		},
		{
			name:        "unreachable node",
			setup:       func() ([]byte, QuestionStore) { return createWorkflow_UnreachableNode(t) },
			expectedErr: false,
		},
		{
			name:        "invalid workflow - invalid node reference",
			setup:       func() ([]byte, QuestionStore) { return createWorkflow_InvalidNextRefWithStore(t) },
			expectedErr: true,
		},
		{
			name: "invalid workflow - condition rule with non-existent question",
			setup: func() ([]byte, QuestionStore) {
				return createWorkflow_ConditionRuleWithEmptyStore(t, uuid.New().String())
			},
			expectedErr: true,
		},
		{
			name:        "invalid workflow - invalid node payload type (number)",
			setup:       func() ([]byte, QuestionStore) { return createWorkflow_SimpleValid_WithNodePayload(t, 123), nil },
			expectedErr: true,
		},
		{
			name: "invalid workflow - invalid node payload shape (missing y)",
			setup: func() ([]byte, QuestionStore) {
				return createWorkflow_SimpleValid_WithNodePayload(t, map[string]interface{}{"x": 1}), nil
			},
			expectedErr: true,
		},
		{
			name: "invalid workflow - missing node payload field",
			setup: func() ([]byte, QuestionStore) {
				return createWorkflow_SimpleValid_WithoutNodePayload(t), nil
			},
			expectedErr: true,
		},
	}

	validator := NewValidator()
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
		setup       func(t *testing.T) ([]byte, QuestionStore)
		expectedErr bool
	}

	testCases := []testCase{
		{
			name:        "valid - condition without conditionRule",
			setup:       func(t *testing.T) ([]byte, QuestionStore) { return createWorkflow_ConditionNoRule(t) },
			expectedErr: false,
		},
	}

	validator := NewValidator()
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

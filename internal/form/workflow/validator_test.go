package workflow_test

import (
	"context"
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/workflow"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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
			workflowJSON: createWorkflowWithInvalidNextRef(t),
		},
		{
			name:         "condition node references non-existent node ID in nextTrue field",
			workflowJSON: createWorkflowWithInvalidNextTrueRef(t),
		},
		{
			name:         "condition node references non-existent node ID in nextFalse field",
			workflowJSON: createWorkflowWithInvalidNextFalseRef(t),
		},
		{
			name:         "condition node references non-existent nodes in both nextTrue and nextFalse",
			workflowJSON: createWorkflowWithInvalidConditionRefs(t),
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

// Helper functions to create test workflows with invalid references

func createWorkflowWithInvalidNextRef(t *testing.T) []byte {
	t.Helper()
	startID := uuid.New()
	endID := uuid.New()
	nonExistentID := uuid.New()

	return createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  nonExistentID.String(), // References non-existent node
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
}

func createWorkflowWithInvalidNextTrueRef(t *testing.T) []byte {
	t.Helper()
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	nonExistentID := uuid.New()
	sectionID := uuid.New()

	return createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  nonExistentID.String(), // References non-existent node
			"nextFalse": endID.String(),
			"conditionRule": map[string]interface{}{
				"source":  "choice",
				"nodeId":  sectionID.String(),
				"key":     uuid.New().String(),
				"pattern": "yes",
			},
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  conditionID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
}

func createWorkflowWithInvalidNextFalseRef(t *testing.T) []byte {
	t.Helper()
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	nonExistentID := uuid.New()
	sectionID := uuid.New()

	return createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  endID.String(),
			"nextFalse": nonExistentID.String(), // References non-existent node
			"conditionRule": map[string]interface{}{
				"source":  "choice",
				"nodeId":  sectionID.String(),
				"key":     uuid.New().String(),
				"pattern": "yes",
			},
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  conditionID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
}

func createWorkflowWithInvalidConditionRefs(t *testing.T) []byte {
	t.Helper()
	startID := uuid.New()
	conditionID := uuid.New()
	nonExistentID1 := uuid.New()
	nonExistentID2 := uuid.New()
	sectionID := uuid.New()

	return createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  nonExistentID1.String(), // References non-existent node
			"nextFalse": nonExistentID2.String(), // References non-existent node
			"conditionRule": map[string]interface{}{
				"source":  "non-choice",
				"nodeId":  sectionID.String(),
				"key":     uuid.New().String(),
				"pattern": "^no$",
			},
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  conditionID.String(),
		},
	})
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
				return createWorkflowWithConditionRule(t, questionID),
					&mockQuestionStore{questions: make(map[uuid.UUID]question.Answerable)}
			},
			expectedErr: true,
		},
		{
			name: "condition rule with question from different form",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				questionUUID := mustParseUUID(t, questionID)
				return createWorkflowWithConditionRule(t, questionID),
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
				return createWorkflowWithConditionRuleSourceWithQuestionID(t, "choice", questionID),
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
				return createWorkflowWithConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
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
				return createWorkflowWithConditionRuleSourceWithQuestionID(t, "choice", questionID),
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
				return createWorkflowWithConditionRuleSourceWithQuestionID(t, "choice", questionID),
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
				return createWorkflowWithConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
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
				return createWorkflowWithConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
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
				return createWorkflowWithConditionRuleSourceWithQuestionID(t, "nonChoice", questionID),
					&mockQuestionStore{
						questions: map[uuid.UUID]question.Answerable{
							questionUUID: createMockAnswerable(t, formID, question.QuestionTypeDate),
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

// mockQuestionStore is a mock implementation of workflow.QuestionStore for testing
type mockQuestionStore struct {
	questions map[uuid.UUID]question.Answerable
}

func (m *mockQuestionStore) GetByID(ctx context.Context, id uuid.UUID) (question.Answerable, error) {
	if q, ok := m.questions[id]; ok {
		return q, nil
	}
	return nil, internal.ErrQuestionNotFound
}

func (m *mockQuestionStore) ListByFormID(ctx context.Context, formID uuid.UUID) ([]question.Answerable, error) {
	var result []question.Answerable
	for _, q := range m.questions {
		if q.Question().FormID == formID {
			result = append(result, q)
		}
	}
	return result, nil
}

// Helper functions for condition rule validation tests

func createWorkflowWithConditionRule(t *testing.T, questionID string) []byte {
	t.Helper()
	if questionID == "" {
		questionID = uuid.New().String()
	}
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	sectionID := uuid.New()

	return createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  sectionID.String(),
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  endID.String(),
			"nextFalse": endID.String(),
			"conditionRule": map[string]interface{}{
				"source":  "choice",
				"nodeId":  sectionID.String(),
				"key":     questionID,
				"pattern": "yes",
			},
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
}

func createWorkflowWithConditionRuleSourceWithQuestionID(t *testing.T, source string, questionID string) []byte {
	t.Helper()
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	sectionID := uuid.New()

	return createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  sectionID.String(),
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  endID.String(),
			"nextFalse": endID.String(),
			"conditionRule": map[string]interface{}{
				"source":  source,
				"nodeId":  sectionID.String(),
				"key":     questionID,
				"pattern": "yes",
			},
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
}

func createMockAnswerable(t *testing.T, formID uuid.UUID, questionType question.QuestionType) question.Answerable {
	t.Helper()
	q := question.Question{
		ID:       uuid.New(),
		FormID:   formID,
		Required: false,
		Type:     questionType,
		Title:    pgtype.Text{String: "Test Question", Valid: true},
		Order:    1,
	}

	// Generate metadata for choice-based questions
	if questionType == question.QuestionTypeSingleChoice || questionType == question.QuestionTypeMultipleChoice {
		metadata, err := question.GenerateMetadata(string(questionType), []question.ChoiceOption{
			{Name: "Option 1"},
			{Name: "Option 2"},
		})
		require.NoError(t, err)
		q.Metadata = metadata
	} else {
		// For non-choice questions, use empty metadata
		q.Metadata = []byte("{}")
	}

	answerable, err := question.NewAnswerable(q)
	require.NoError(t, err)
	return answerable
}

// mustParseUUID parses a UUID string and fails the test if parsing fails
func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	require.NoError(t, err, "failed to parse UUID: %s", s)
	return id
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
				return nil, createSimpleWorkflowForNodeIDTest(t)
			},
			expectedErr: false,
		},
		{
			name: "node IDs unchanged - same workflow",
			setup: func(t *testing.T) ([]byte, []byte) {
				// Create once and reuse for both to ensure same IDs
				workflow := createSimpleWorkflowForNodeIDTest(t)
				return workflow, workflow
			},
			expectedErr: false,
		},
		{
			name: "node IDs unchanged - workflow with same IDs but different properties",
			setup: func(t *testing.T) ([]byte, []byte) {
				// Create workflow with section and extract IDs
				currentWorkflow := createWorkflowWithSection(t)
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
				return createWorkflowWithSection(t), createSimpleWorkflowForNodeIDTest(t)
			},
			expectedErr: true,
		},
		{
			name: "error - node ID added",
			setup: func(t *testing.T) ([]byte, []byte) {
				return createSimpleWorkflowForNodeIDTest(t), createWorkflowWithSection(t)
			},
			expectedErr: true,
		},
		{
			name: "error - multiple node IDs removed",
			setup: func(t *testing.T) ([]byte, []byte) {
				return createWorkflowWithMultipleNodes(t), createSimpleWorkflowForNodeIDTest(t)
			},
			expectedErr: true,
		},
		{
			name: "error - multiple node IDs added",
			setup: func(t *testing.T) ([]byte, []byte) {
				return createSimpleWorkflowForNodeIDTest(t), createWorkflowWithMultipleNodes(t)
			},
			expectedErr: true,
		},
		{
			name: "error - invalid current workflow JSON",
			setup: func(t *testing.T) ([]byte, []byte) {
				return []byte(`{invalid json}`), createSimpleWorkflowForNodeIDTest(t)
			},
			expectedErr: true,
		},
		{
			name: "error - invalid new workflow JSON",
			setup: func(t *testing.T) ([]byte, []byte) {
				return createSimpleWorkflowForNodeIDTest(t), []byte(`{invalid json}`)
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
			name: "valid workflow",
			setup: func() ([]byte, workflow.QuestionStore) {
				startID := uuid.New()
				endID := uuid.New()
				nodes := []map[string]interface{}{
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
				}
				return createWorkflowJSON(t, nodes), &mockQuestionStore{questions: make(map[uuid.UUID]question.Answerable)}
			},
			expectedErr: false,
		},
		{
			name: "invalid workflow - missing start node",
			setup: func() ([]byte, workflow.QuestionStore) {
				endID := uuid.New()
				nodes := []map[string]interface{}{
					{
						"id":    endID.String(),
						"type":  "end",
						"label": "End",
					},
				}
				return createWorkflowJSON(t, nodes), &mockQuestionStore{questions: make(map[uuid.UUID]question.Answerable)}
			},
			expectedErr: true,
		},
		{
			name: "invalid workflow - duplicate node IDs",
			setup: func() ([]byte, workflow.QuestionStore) {
				startID := uuid.New()
				endID := uuid.New()
				nodes := []map[string]interface{}{
					{
						"id":    startID.String(),
						"type":  "start",
						"label": "Start",
						"next":  endID.String(),
					},
					{
						"id":    startID.String(), // Duplicate ID
						"type":  "end",
						"label": "End",
					},
				}
				return createWorkflowJSON(t, nodes), &mockQuestionStore{questions: make(map[uuid.UUID]question.Answerable)}
			},
			expectedErr: true,
		},
		{
			name: "unreachable node",
			setup: func() ([]byte, workflow.QuestionStore) {
				startID := uuid.New()
				endID := uuid.New()
				orphanID := uuid.New()
				nodes := []map[string]interface{}{
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
					{
						"id":    orphanID.String(),
						"type":  "section",
						"label": "Orphan",
						// No connections - unreachable
					},
				}
				return createWorkflowJSON(t, nodes), &mockQuestionStore{questions: make(map[uuid.UUID]question.Answerable)}
			},
			expectedErr: false,
		},
		{
			name: "invalid workflow - invalid node reference",
			setup: func() ([]byte, workflow.QuestionStore) {
				return createWorkflowWithInvalidNextRef(t), &mockQuestionStore{questions: make(map[uuid.UUID]question.Answerable)}
			},
			expectedErr: true,
		},
		{
			name: "invalid workflow - condition rule with non-existent question",
			setup: func() ([]byte, workflow.QuestionStore) {
				questionID := uuid.New().String()
				return createWorkflowWithConditionRule(t, questionID),
					&mockQuestionStore{questions: make(map[uuid.UUID]question.Answerable)}
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

// Helper functions for ValidateUpdateNodeIDs tests

func createSimpleWorkflowForNodeIDTest(t *testing.T) []byte {
	t.Helper()
	startID := uuid.New()
	endID := uuid.New()

	return createWorkflowJSON(t, []map[string]interface{}{
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
}

func createWorkflowWithSection(t *testing.T) []byte {
	t.Helper()
	startID := uuid.New()
	sectionID := uuid.New()
	endID := uuid.New()

	return createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  sectionID.String(),
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
}

func createWorkflowWithMultipleNodes(t *testing.T) []byte {
	t.Helper()
	startID := uuid.New()
	sectionID1 := uuid.New()
	sectionID2 := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()

	return createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  sectionID1.String(),
		},
		{
			"id":    sectionID1.String(),
			"type":  "section",
			"label": "Section 1",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  sectionID2.String(),
			"nextFalse": endID.String(),
		},
		{
			"id":    sectionID2.String(),
			"type":  "section",
			"label": "Section 2",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
}

// TestValidateDraft_ConditionSectionOrder tests that ValidateDraft detects when a condition
// references a section that comes after it in the graph traversal.
func TestValidateDraft_ConditionSectionOrder(t *testing.T) {
	t.Parallel()

	formID := uuid.New()

	type testCase struct {
		name        string
		setup       func() ([]byte, workflow.QuestionStore)
		expectedErr bool
	}

	testCases := []testCase{
		{
			name: "error - condition references section that comes after it",
			setup: func() ([]byte, workflow.QuestionStore) {
				// Flow: start -> condition -> section -> end
				// Condition references section which hasn't been visited yet
				startID := uuid.New()
				conditionID := uuid.New()
				sectionID := uuid.New()
				endID := uuid.New()
				questionID := uuid.New()

				nodes := []map[string]interface{}{
					{
						"id":    startID.String(),
						"type":  "start",
						"label": "Start",
						"next":  conditionID.String(),
					},
					{
						"id":        conditionID.String(),
						"type":      "condition",
						"label":     "Condition",
						"nextTrue":  sectionID.String(),
						"nextFalse": endID.String(),
						"conditionRule": map[string]interface{}{
							"source":  "choice",
							"nodeId":  sectionID.String(), // References section that comes after
							"key":     questionID.String(),
							"pattern": "yes",
						},
					},
					{
						"id":    sectionID.String(),
						"type":  "section",
						"label": "Section",
						"next":  endID.String(),
					},
					{
						"id":    endID.String(),
						"type":  "end",
						"label": "End",
					},
				}

				questionStore := &mockQuestionStore{
					questions: map[uuid.UUID]question.Answerable{
						questionID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
					},
				}

				return createWorkflowJSON(t, nodes), questionStore
			},
			expectedErr: true,
		},
		{
			name: "valid - condition references section that comes before it",
			setup: func() ([]byte, workflow.QuestionStore) {
				// Flow: start -> section -> condition -> end
				// Condition references section which has been visited
				startID := uuid.New()
				sectionID := uuid.New()
				conditionID := uuid.New()
				endID := uuid.New()
				questionID := uuid.New()

				nodes := []map[string]interface{}{
					{
						"id":    startID.String(),
						"type":  "start",
						"label": "Start",
						"next":  sectionID.String(),
					},
					{
						"id":    sectionID.String(),
						"type":  "section",
						"label": "Section",
						"next":  conditionID.String(),
					},
					{
						"id":        conditionID.String(),
						"type":      "condition",
						"label":     "Condition",
						"nextTrue":  endID.String(),
						"nextFalse": endID.String(),
						"conditionRule": map[string]interface{}{
							"source":  "choice",
							"nodeId":  sectionID.String(), // References section that comes before
							"key":     questionID.String(),
							"pattern": "yes",
						},
					},
					{
						"id":    endID.String(),
						"type":  "end",
						"label": "End",
					},
				}

				questionStore := &mockQuestionStore{
					questions: map[uuid.UUID]question.Answerable{
						questionID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
					},
				}

				return createWorkflowJSON(t, nodes), questionStore
			},
			expectedErr: false,
		},
		{
			name: "error - condition references itself via nodeId",
			setup: func() ([]byte, workflow.QuestionStore) {
				// Flow: start -> section -> condition -> end
				// Condition references itself (invalid)
				startID := uuid.New()
				sectionID := uuid.New()
				conditionID := uuid.New()
				endID := uuid.New()
				questionID := uuid.New()

				nodes := []map[string]interface{}{
					{
						"id":    startID.String(),
						"type":  "start",
						"label": "Start",
						"next":  sectionID.String(),
					},
					{
						"id":    sectionID.String(),
						"type":  "section",
						"label": "Section",
						"next":  conditionID.String(),
					},
					{
						"id":        conditionID.String(),
						"type":      "condition",
						"label":     "Condition",
						"nextTrue":  endID.String(),
						"nextFalse": endID.String(),
						"conditionRule": map[string]interface{}{
							"source":  "choice",
							"nodeId":  conditionID.String(), // References itself
							"key":     questionID.String(),
							"pattern": "yes",
						},
					},
					{
						"id":    endID.String(),
						"type":  "end",
						"label": "End",
					},
				}

				questionStore := &mockQuestionStore{
					questions: map[uuid.UUID]question.Answerable{
						questionID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
					},
				}

				return createWorkflowJSON(t, nodes), questionStore
			},
			expectedErr: true,
		},
		{
			name: "valid - condition without conditionRule",
			setup: func() ([]byte, workflow.QuestionStore) {
				// Condition without conditionRule should be allowed in draft mode
				startID := uuid.New()
				conditionID := uuid.New()
				endID := uuid.New()

				nodes := []map[string]interface{}{
					{
						"id":    startID.String(),
						"type":  "start",
						"label": "Start",
						"next":  conditionID.String(),
					},
					{
						"id":        conditionID.String(),
						"type":      "condition",
						"label":     "Condition",
						"nextTrue":  endID.String(),
						"nextFalse": endID.String(),
						// No conditionRule - allowed in draft
					},
					{
						"id":    endID.String(),
						"type":  "end",
						"label": "End",
					},
				}

				return createWorkflowJSON(t, nodes), nil
			},
			expectedErr: false,
		},
		{
			name: "error - multiple conditions with invalid section references",
			setup: func() ([]byte, workflow.QuestionStore) {
				// Two conditions both referencing sections that come after them
				startID := uuid.New()
				condition1ID := uuid.New()
				condition2ID := uuid.New()
				section1ID := uuid.New()
				section2ID := uuid.New()
				endID := uuid.New()
				questionID := uuid.New()

				nodes := []map[string]interface{}{
					{
						"id":    startID.String(),
						"type":  "start",
						"label": "Start",
						"next":  condition1ID.String(),
					},
					{
						"id":        condition1ID.String(),
						"type":      "condition",
						"label":     "Condition 1",
						"nextTrue":  section1ID.String(),
						"nextFalse": condition2ID.String(),
						"conditionRule": map[string]interface{}{
							"source":  "choice",
							"nodeId":  section1ID.String(), // References section that comes after
							"key":     questionID.String(),
							"pattern": "yes",
						},
					},
					{
						"id":        condition2ID.String(),
						"type":      "condition",
						"label":     "Condition 2",
						"nextTrue":  section2ID.String(),
						"nextFalse": endID.String(),
						"conditionRule": map[string]interface{}{
							"source":  "choice",
							"nodeId":  section2ID.String(), // References section that comes after
							"key":     questionID.String(),
							"pattern": "no",
						},
					},
					{
						"id":    section1ID.String(),
						"type":  "section",
						"label": "Section 1",
						"next":  endID.String(),
					},
					{
						"id":    section2ID.String(),
						"type":  "section",
						"label": "Section 2",
						"next":  endID.String(),
					},
					{
						"id":    endID.String(),
						"type":  "end",
						"label": "End",
					},
				}

				questionStore := &mockQuestionStore{
					questions: map[uuid.UUID]question.Answerable{
						questionID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
					},
				}

				return createWorkflowJSON(t, nodes), questionStore
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

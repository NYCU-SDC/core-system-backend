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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// mockQuerier is a mock implementation of workflow.Querier interface
type mockQuerier struct {
	mock.Mock
}

func (m *mockQuerier) Get(ctx context.Context, formID uuid.UUID) (workflow.GetRow, error) {
	args := m.Called(ctx, formID)
	return args.Get(0).(workflow.GetRow), args.Error(1)
}

func (m *mockQuerier) Update(ctx context.Context, arg workflow.UpdateParams) (workflow.UpdateRow, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(workflow.UpdateRow), args.Error(1)
}

func (m *mockQuerier) CreateNode(ctx context.Context, arg workflow.CreateNodeParams) (workflow.CreateNodeRow, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(workflow.CreateNodeRow), args.Error(1)
}

func (m *mockQuerier) DeleteNode(ctx context.Context, arg workflow.DeleteNodeParams) ([]byte, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *mockQuerier) Activate(ctx context.Context, arg workflow.ActivateParams) (workflow.ActivateRow, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(workflow.ActivateRow), args.Error(1)
}

// mockValidator is a mock implementation of workflow.Validator interface
type mockValidator struct {
	mock.Mock
}

func (m *mockValidator) Activate(ctx context.Context, formID uuid.UUID, workflow []byte, questionStore workflow.QuestionStore) error {
	args := m.Called(ctx, formID, workflow, questionStore)
	return args.Error(0)
}

func (m *mockValidator) Validate(ctx context.Context, formID uuid.UUID, workflow []byte, questionStore workflow.QuestionStore) error {
	args := m.Called(ctx, formID, workflow, questionStore)
	return args.Error(0)
}

func (m *mockValidator) ValidateNodeIDsUnchanged(ctx context.Context, currentWorkflow, newWorkflow []byte) error {
	args := m.Called(ctx, currentWorkflow, newWorkflow)
	return args.Error(0)
}

func (m *mockValidator) ValidateUpdateNodeIDs(ctx context.Context, currentWorkflow, newWorkflow []byte) error {
	args := m.Called(ctx, currentWorkflow, newWorkflow)
	return args.Error(0)
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
		if q.FormID() == formID {
			result = append(result, q)
		}
	}
	return result, nil
}

// createTestService creates a workflow.Service with mocked dependencies
func createTestService(t *testing.T, logger *zap.Logger, tracer trace.Tracer, mockQuerier *mockQuerier, mockValidator *mockValidator, questionStore workflow.QuestionStore) *workflow.Service {
	t.Helper()
	return workflow.NewServiceForTesting(logger, tracer, mockQuerier, mockValidator, questionStore)
}

// createWorkflowJSON marshals nodes to JSON and fails the test on error
func createWorkflowJSON(t *testing.T, nodes []map[string]interface{}) []byte {
	t.Helper()
	jsonBytes, err := json.Marshal(nodes)
	require.NoError(t, err)
	return jsonBytes
}

// createWorkflow_SimpleValid returns a minimal valid workflow (start -> end)
func createWorkflow_SimpleValid(t *testing.T) []byte {
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

// createWorkflow_ComplexValid returns a workflow with start, section, condition, and end
func createWorkflow_ComplexValid(t *testing.T) []byte {
	t.Helper()
	startID := uuid.New()
	sectionID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	referenceNodeID := uuid.New()

	workflowJSON, err := json.Marshal([]map[string]interface{}{
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
				"nodeId":  referenceNodeID.String(),
				"key":     "answer",
				"pattern": "yes",
			},
		},
		{
			"id":    referenceNodeID.String(),
			"type":  "section",
			"label": "Reference Section",
			"next":  conditionID.String(),
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

// createWorkflow	_InvalidNextRef returns a workflow where start references a non-existent node in next
func createWorkflow_InvalidNextRef(t *testing.T) []byte {
	t.Helper()
	startID := uuid.New()
	endID := uuid.New()
	nonExistentID := uuid.New()

	return createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  nonExistentID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
}

// createWorkflow_InvalidNextTrueRef returns a workflow where condition references non-existent node in nextTrue
func createWorkflow_InvalidNextTrueRef(t *testing.T) []byte {
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
			"nextTrue":  nonExistentID.String(),
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

// createWorkflow_InvalidNextFalseRef returns a workflow where condition references non-existent node in nextFalse
func createWorkflow_InvalidNextFalseRef(t *testing.T) []byte {
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
			"nextFalse": nonExistentID.String(),
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

// createWorkflow_InvalidConditionRefs returns a workflow where condition references non-existent nodes in both nextTrue and nextFalse
func createWorkflow_InvalidConditionRefs(t *testing.T) []byte {
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
			"nextTrue":  nonExistentID1.String(),
			"nextFalse": nonExistentID2.String(),
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

// createWorkflow_ConditionRule returns a workflow with a condition rule using the given questionID as key
func createWorkflow_ConditionRule(t *testing.T, questionID string) []byte {
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

// createWorkflow_ConditionRuleSourceWithQuestionID returns a workflow with condition rule with given source and questionID
func createWorkflow_ConditionRuleSourceWithQuestionID(t *testing.T, source string, questionID string) []byte {
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

// createMockAnswerable creates a question.Answerable for testing
func createMockAnswerable(t *testing.T, formID uuid.UUID, questionType question.QuestionType) question.Answerable {
	t.Helper()
	q := question.Question{
		ID:       uuid.New(),
		Required: false,
		Type:     questionType,
		Title:    pgtype.Text{String: "Test Question", Valid: true},
		Order:    1,
	}

	switch {
	case question.ContainsType(question.ChoiceTypes, questionType):
		choiceOptions := []question.ChoiceOption{
			{Name: "Option 1"},
			{Name: "Option 2"},
		}
		if questionType == question.QuestionTypeDetailedMultipleChoice {
			choiceOptions[0] = question.ChoiceOption{Name: "Option 1", Description: "Description for option 1"}
		}
		metadata, err := question.GenerateChoiceMetadata(string(questionType), choiceOptions)
		require.NoError(t, err)
		q.Metadata = metadata
	case questionType == question.QuestionTypeLinearScale:
		metadata, err := question.GenerateLinearScaleMetadata(question.ScaleOption{MinVal: 1, MaxVal: 5})
		require.NoError(t, err)
		q.Metadata = metadata
	case questionType == question.QuestionTypeRating:
		metadata, err := question.GenerateRatingMetadata(question.ScaleOption{Icon: "star", MinVal: 1, MaxVal: 5})
		require.NoError(t, err)
		q.Metadata = metadata
	case questionType == question.QuestionTypeOauthConnect:
		metadata, err := question.GenerateOauthConnectMetadata("google")
		require.NoError(t, err)
		q.Metadata = metadata
	case questionType == question.QuestionTypeUploadFile:
		metadata, err := question.GenerateUploadFileMetadata(question.UploadFileOption{
			AllowedFileTypes: []string{"pdf"},
			MaxFileAmount:    1,
			MaxFileSizeLimit: 10485760, // 10 MB in bytes
		})
		require.NoError(t, err)
		q.Metadata = metadata
	case questionType == question.QuestionTypeDate:
		metadata, err := question.GenerateDateMetadata(question.DateOption{
			HasYear:  true,
			HasMonth: true,
			HasDay:   true,
		})
		require.NoError(t, err)
		q.Metadata = metadata
	default:
		q.Metadata = []byte("{}")
	}

	answerable, err := question.NewAnswerable(q, formID)
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

// createWorkflow_SimpleForNodeIDTest returns a minimal start->end workflow for node ID tests
func createWorkflow_SimpleForNodeIDTest(t *testing.T) []byte {
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

// createWorkflow_WithSection returns a workflow with start -> section -> end
func createWorkflow_WithSection(t *testing.T) []byte {
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

// createWorkflowWithMultipleNodes returns a workflow with start, two sections, condition, and end
func createWorkflow_MultipleNodes(t *testing.T) []byte {
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

// emptyQuestionStore returns a mock question store with no questions (for tests that only need workflow shape).
func emptyQuestionStore() *mockQuestionStore {
	return &mockQuestionStore{questions: make(map[uuid.UUID]question.Answerable)}
}

// createWorkflow_ValidWithEmptyStore returns a minimal valid workflow (start -> end) and an empty question store.
func createWorkflow_ValidWithEmptyStore(t *testing.T) ([]byte, workflow.QuestionStore) {
	t.Helper()
	return createWorkflow_SimpleValid(t), emptyQuestionStore()
}

// createWorkflow_MissingStartNode returns a workflow with only an end node (invalid) and an empty question store.
func createWorkflow_MissingStartNode(t *testing.T) ([]byte, workflow.QuestionStore) {
	t.Helper()
	endID := uuid.New()
	nodes := []map[string]interface{}{
		{"id": endID.String(), "type": "end", "label": "End"},
	}
	return createWorkflowJSON(t, nodes), emptyQuestionStore()
}

// createWorkflow_DuplicateNodeIDs returns a workflow where two nodes share the same ID (invalid) and an empty question store.
func createWorkflow_DuplicateNodeIDs(t *testing.T) ([]byte, workflow.QuestionStore) {
	t.Helper()
	startID := uuid.New()
	endID := uuid.New()
	nodes := []map[string]interface{}{
		{"id": startID.String(), "type": "start", "label": "Start", "next": endID.String()},
		{"id": startID.String(), "type": "end", "label": "End"},
	}
	return createWorkflowJSON(t, nodes), emptyQuestionStore()
}

// createWorkflow_UnreachableNode returns a workflow with an orphan section (unreachable from start) and an empty question store.
func createWorkflow_UnreachableNode(t *testing.T) ([]byte, workflow.QuestionStore) {
	t.Helper()
	startID := uuid.New()
	endID := uuid.New()
	orphanID := uuid.New()
	nodes := []map[string]interface{}{
		{"id": startID.String(), "type": "start", "label": "Start", "next": endID.String()},
		{"id": endID.String(), "type": "end", "label": "End"},
		{"id": orphanID.String(), "type": "section", "label": "Orphan"},
	}
	return createWorkflowJSON(t, nodes), emptyQuestionStore()
}

// createWorkflow_InvalidNextRefWithStore returns a workflow with an invalid next reference and an empty question store.
func createWorkflow_InvalidNextRefWithStore(t *testing.T) ([]byte, workflow.QuestionStore) {
	t.Helper()
	return createWorkflow_InvalidNextRef(t), emptyQuestionStore()
}

// createWorkflow_ConditionRuleWithEmptyStore returns a workflow with a condition rule (given questionID as key) and an empty question store.
func createWorkflow_ConditionRuleWithEmptyStore(t *testing.T, questionID string) ([]byte, workflow.QuestionStore) {
	t.Helper()
	return createWorkflow_ConditionRule(t, questionID), emptyQuestionStore()
}

// createWorkflow_ConditionRefsSectionAfter returns a workflow where condition references a section
// that comes after it in traversal (start -> condition -> section -> end). Invalid for draft validation.
func createWorkflow_ConditionRefsSectionAfter(t *testing.T, formID uuid.UUID) ([]byte, workflow.QuestionStore) {
	t.Helper()
	startID := uuid.New()
	conditionID := uuid.New()
	sectionID := uuid.New()
	endID := uuid.New()
	questionID := uuid.New()

	nodes := []map[string]interface{}{
		{"id": startID.String(), "type": "start", "label": "Start", "next": conditionID.String()},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  sectionID.String(),
			"nextFalse": endID.String(),
			"conditionRule": map[string]interface{}{
				"source":  "choice",
				"nodeId":  sectionID.String(),
				"key":     questionID.String(),
				"pattern": "yes",
			},
		},
		{"id": sectionID.String(), "type": "section", "label": "Section", "next": endID.String()},
		{"id": endID.String(), "type": "end", "label": "End"},
	}
	store := &mockQuestionStore{
		questions: map[uuid.UUID]question.Answerable{
			questionID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
		},
	}
	return createWorkflowJSON(t, nodes), store
}

// createWorkflow_ConditionRefsSectionBefore returns a workflow where condition references a section
// that comes before it (start -> section -> condition -> end). Valid for draft validation.
func createWorkflow_ConditionRefsSectionBefore(t *testing.T, formID uuid.UUID) ([]byte, workflow.QuestionStore) {
	t.Helper()
	startID := uuid.New()
	sectionID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	questionID := uuid.New()

	nodes := []map[string]interface{}{
		{"id": startID.String(), "type": "start", "label": "Start", "next": sectionID.String()},
		{"id": sectionID.String(), "type": "section", "label": "Section", "next": conditionID.String()},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  endID.String(),
			"nextFalse": endID.String(),
			"conditionRule": map[string]interface{}{
				"source":  "choice",
				"nodeId":  sectionID.String(),
				"key":     questionID.String(),
				"pattern": "yes",
			},
		},
		{"id": endID.String(), "type": "end", "label": "End"},
	}
	store := &mockQuestionStore{
		questions: map[uuid.UUID]question.Answerable{
			questionID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
		},
	}
	return createWorkflowJSON(t, nodes), store
}

// createWorkflow_ConditionRefsSelf returns a workflow where condition's conditionRule.nodeId
// references the condition node itself. Invalid for draft validation.
func createWorkflow_ConditionRefsSelf(t *testing.T, formID uuid.UUID) ([]byte, workflow.QuestionStore) {
	t.Helper()
	startID := uuid.New()
	sectionID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	questionID := uuid.New()

	nodes := []map[string]interface{}{
		{"id": startID.String(), "type": "start", "label": "Start", "next": sectionID.String()},
		{"id": sectionID.String(), "type": "section", "label": "Section", "next": conditionID.String()},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  endID.String(),
			"nextFalse": endID.String(),
			"conditionRule": map[string]interface{}{
				"source":  "choice",
				"nodeId":  conditionID.String(),
				"key":     questionID.String(),
				"pattern": "yes",
			},
		},
		{"id": endID.String(), "type": "end", "label": "End"},
	}
	store := &mockQuestionStore{
		questions: map[uuid.UUID]question.Answerable{
			questionID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
		},
	}
	return createWorkflowJSON(t, nodes), store
}

// createWorkflow_ConditionNoRule returns a workflow with a condition node without conditionRule
// (start -> condition -> end). Valid in draft mode.
func createWorkflow_ConditionNoRule(t *testing.T) ([]byte, workflow.QuestionStore) {
	t.Helper()
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()

	nodes := []map[string]interface{}{
		{"id": startID.String(), "type": "start", "label": "Start", "next": conditionID.String()},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  endID.String(),
			"nextFalse": endID.String(),
		},
		{"id": endID.String(), "type": "end", "label": "End"},
	}
	return createWorkflowJSON(t, nodes), nil
}

// createWorkflow_MultipleConditionsInvalidSectionRefs returns a workflow with two conditions
// that both reference sections appearing after them in traversal. Invalid for draft validation.
func createWorkflow_MultipleConditionsInvalidSectionRefs(t *testing.T, formID uuid.UUID) ([]byte, workflow.QuestionStore) {
	t.Helper()
	startID := uuid.New()
	condition1ID := uuid.New()
	condition2ID := uuid.New()
	section1ID := uuid.New()
	section2ID := uuid.New()
	endID := uuid.New()
	questionID := uuid.New()

	nodes := []map[string]interface{}{
		{"id": startID.String(), "type": "start", "label": "Start", "next": condition1ID.String()},
		{
			"id":        condition1ID.String(),
			"type":      "condition",
			"label":     "Condition 1",
			"nextTrue":  section1ID.String(),
			"nextFalse": condition2ID.String(),
			"conditionRule": map[string]interface{}{
				"source":  "choice",
				"nodeId":  section1ID.String(),
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
				"nodeId":  section2ID.String(),
				"key":     questionID.String(),
				"pattern": "no",
			},
		},
		{"id": section1ID.String(), "type": "section", "label": "Section 1", "next": endID.String()},
		{"id": section2ID.String(), "type": "section", "label": "Section 2", "next": endID.String()},
		{"id": endID.String(), "type": "end", "label": "End"},
	}
	store := &mockQuestionStore{
		questions: map[uuid.UUID]question.Answerable{
			questionID: createMockAnswerable(t, formID, question.QuestionTypeSingleChoice),
		},
	}
	return createWorkflowJSON(t, nodes), store
}

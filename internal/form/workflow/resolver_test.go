package workflow

import (
	"NYCU-SDC/core-system-backend/internal/form/answer"
	"context"
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

type setupParams struct {
	formID        uuid.UUID
	workflowJSON  []byte
	answers       []answer.Answer
	answerableMap map[string]question.Answerable
	sections      []uuid.UUID
	expected      []uuid.UUID
	expectedErr   error
}

type testCase struct {
	name     string
	setup    func(t *testing.T) setupParams
	validate func(t *testing.T, setup setupParams, result []uuid.UUID, err error)
}

func TestService_ResolveSections(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	tracer := noop.NewTracerProvider().Tracer("test")

	testCases := []testCase{
		{
			name:  "simple",
			setup: buildSimpleSetup,
			validate: func(t *testing.T, setup setupParams, result []uuid.UUID, err error) {
				require.NoError(t, err)
				require.Equal(t, setup.expected, result)
			},
		},
		{
			name:  "choice condition true -> nextTrue",
			setup: buildChoiceConditionTrueNextTrueSetup,
			validate: func(t *testing.T, setup setupParams, result []uuid.UUID, err error) {
				require.NoError(t, err)
				require.Equal(t, setup.expected, result)
			},
		},
		{
			name:  "multiple choice condition match -> nextTrue",
			setup: buildMultipleChoiceConditionMatchNextTrueSetup,
			validate: func(t *testing.T, setup setupParams, result []uuid.UUID, err error) {
				require.NoError(t, err)
				require.Equal(t, setup.expected, result)
			},
		},
		{
			name:  "detailed multi choice mismatch -> nextFalse",
			setup: buildDetailedMultiChoiceMismatchNextFalseSetup,
			validate: func(t *testing.T, setup setupParams, result []uuid.UUID, err error) {
				require.NoError(t, err)
				require.Equal(t, setup.expected, result)
			},
		},
		{
			name:  "empty workflow error",
			setup: buildEmptyWorkflowErrorSetup,
			validate: func(t *testing.T, setup setupParams, result []uuid.UUID, err error) {
				// Workflow row exists but JSON is `[]`: no start node, not ErrWorkflowNotFound
				// (ErrWorkflowNotFound is only when queries.Get returns pgx.ErrNoRows).
				require.Error(t, err)
				require.Contains(t, err.Error(), "start node not found")
				require.Empty(t, result)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			setup := tc.setup(t)

			mockQuerier := new(mockQuerier)
			service := createTestService(t, logger, tracer, mockQuerier, new(mockValidator), nil)
			mockQuerier.On("Get", mock.Anything, setup.formID).Return(WorkflowVersion{
				ID:       setup.formID,
				FormID:   setup.formID,
				Workflow: setup.workflowJSON,
			}, nil).Once()

			result, err := service.ResolveSections(ctx, setup.formID, setup.answers, setup.answerableMap)

			if tc.validate != nil {
				tc.validate(t, setup, result, err)
			}
		})
	}
}

func buildSimpleSetup(t *testing.T) setupParams {
	t.Helper()

	formID := uuid.New()
	startID := uuid.New()
	sectionAID := uuid.New()
	sectionBID := uuid.New()
	endID := uuid.New()

	workflowJSON := createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  sectionAID.String(),
		},
		{
			"id":    sectionAID.String(),
			"type":  "section",
			"label": "Section A",
			"next":  sectionBID.String(),
		},
		{
			"id":    sectionBID.String(),
			"type":  "section",
			"label": "Section B",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})

	var answers []answer.Answer
	answerableMap := map[string]question.Answerable{}
	return setupParams{
		formID:        formID,
		workflowJSON:  workflowJSON,
		answers:       answers,
		answerableMap: answerableMap,
		sections:      []uuid.UUID{sectionAID, sectionBID},
		expected:      []uuid.UUID{sectionAID, sectionBID},
	}
}

func buildChoiceConditionTrueNextTrueSetup(t *testing.T) setupParams {
	t.Helper()

	formID := uuid.New()
	startID := uuid.New()
	sectionAID := uuid.New()
	sectionBID := uuid.New()
	sectionCID := uuid.New()
	endID := uuid.New()
	conditionID := uuid.New()
	questionID := uuid.New()
	choiceID := uuid.New()
	responseID := uuid.New()

	workflowJSON := createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  sectionAID.String(),
		},
		{
			"id":    sectionAID.String(),
			"type":  "section",
			"label": "Section A",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  sectionBID.String(),
			"nextFalse": sectionCID.String(),
			"conditionRule": map[string]interface{}{
				"source":   "choice",
				"question": questionID.String(),
				"pattern":  "^" + choiceID.String() + "$",
			},
		},
		{
			"id":    sectionBID.String(),
			"type":  "section",
			"label": "Section B",
			"next":  endID.String(),
		},
		{
			"id":    sectionCID.String(),
			"type":  "section",
			"label": "Section C",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})

	answers := []answer.Answer{
		buildAnswer(t, questionID, responseID, shared.SingleChoiceAnswer{
			ChoiceID: choiceID,
			Snapshot: shared.ChoiceSnapshot{Name: "Option A", Description: ""},
		}),
	}

	answerableMap := map[string]question.Answerable{
		questionID.String(): createSingleChoiceAnswerable(t, questionID, sectionAID, formID, choiceID),
	}

	setup := setupParams{
		formID:        formID,
		workflowJSON:  workflowJSON,
		answers:       answers,
		answerableMap: answerableMap,
		sections:      []uuid.UUID{sectionAID, sectionBID, sectionCID},
		expected:      []uuid.UUID{sectionAID, sectionBID},
		expectedErr:   nil,
	}
	return setup
}

func buildMultipleChoiceConditionMatchNextTrueSetup(t *testing.T) setupParams {
	t.Helper()

	formID := uuid.New()
	startID := uuid.New()
	sectionAID := uuid.New()
	sectionBID := uuid.New()
	sectionCID := uuid.New()
	endID := uuid.New()
	conditionID := uuid.New()
	questionID := uuid.New()
	targetChoiceID := uuid.New()
	otherChoiceID := uuid.New()
	responseID := uuid.New()

	workflowJSON := createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  sectionAID.String(),
		},
		{
			"id":    sectionAID.String(),
			"type":  "section",
			"label": "Section A",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  sectionBID.String(),
			"nextFalse": sectionCID.String(),
			"conditionRule": map[string]interface{}{
				"source":   "choice",
				"question": questionID.String(),
				"pattern":  "^" + targetChoiceID.String() + "$",
			},
		},
		{
			"id":    sectionBID.String(),
			"type":  "section",
			"label": "Section B",
			"next":  endID.String(),
		},
		{
			"id":    sectionCID.String(),
			"type":  "section",
			"label": "Section C",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})

	answers := []answer.Answer{
		buildAnswer(t, questionID, responseID, shared.MultipleChoiceAnswer{
			Choices: []struct {
				ChoiceID uuid.UUID             `json:"choiceId"`
				Snapshot shared.ChoiceSnapshot `json:"snapshot"`
			}{
				{
					ChoiceID: otherChoiceID,
					Snapshot: shared.ChoiceSnapshot{Name: "Option A", Description: ""},
				},
				{
					ChoiceID: targetChoiceID,
					Snapshot: shared.ChoiceSnapshot{Name: "Option B", Description: ""},
				},
			},
		}),
	}

	answerableMap := map[string]question.Answerable{
		questionID.String(): createMultipleChoiceAnswerable(t, questionID, sectionAID, formID, targetChoiceID, otherChoiceID),
	}

	setup := setupParams{
		formID:        formID,
		workflowJSON:  workflowJSON,
		answers:       answers,
		answerableMap: answerableMap,
		sections:      []uuid.UUID{sectionAID, sectionBID, sectionCID},
		expected:      []uuid.UUID{sectionAID, sectionBID},
		expectedErr:   nil,
	}
	return setup
}

func buildDetailedMultiChoiceMismatchNextFalseSetup(t *testing.T) setupParams {
	t.Helper()

	formID := uuid.New()
	startID := uuid.New()
	sectionAID := uuid.New()
	sectionBID := uuid.New()
	sectionCID := uuid.New()
	endID := uuid.New()
	conditionID := uuid.New()
	questionID := uuid.New()
	choiceBID := uuid.New()
	choiceCID := uuid.New()
	responseID := uuid.New()

	workflowJSON := createWorkflowJSON(t, []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  sectionAID.String(),
		},
		{
			"id":    sectionAID.String(),
			"type":  "section",
			"label": "Section A",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  sectionBID.String(),
			"nextFalse": sectionCID.String(),
			"conditionRule": map[string]interface{}{
				"source":   "choice",
				"question": questionID.String(),
				"pattern":  choiceBID.String(),
			},
		},
		{
			"id":    sectionBID.String(),
			"type":  "section",
			"label": "Section B",
			"next":  endID.String(),
		},
		{
			"id":    sectionCID.String(),
			"type":  "section",
			"label": "Section C",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})

	answers := []answer.Answer{
		buildAnswer(t, questionID, responseID, shared.DetailedMultipleChoiceAnswer{
			Choices: []struct {
				ChoiceID uuid.UUID             `json:"choiceId"`
				Snapshot shared.ChoiceSnapshot `json:"snapshot"`
			}{
				{
					ChoiceID: choiceCID,
					Snapshot: shared.ChoiceSnapshot{
						Name:        "Option C",
						Description: "",
						OtherText:   "",
					},
				},
			},
		}),
	}

	answerable := createMockAnswerableWithChoiceIDs(t, formID, question.QuestionTypeDetailedMultipleChoice, []uuid.UUID{choiceBID, choiceCID})
	answerableMap := map[string]question.Answerable{
		questionID.String(): answerable,
	}

	setup := setupParams{
		formID:        formID,
		workflowJSON:  workflowJSON,
		answers:       answers,
		answerableMap: answerableMap,
		sections:      []uuid.UUID{sectionAID, uuid.Nil, sectionCID},
		expected:      []uuid.UUID{sectionAID, sectionCID},
		expectedErr:   nil,
	}
	return setup
}

func buildEmptyWorkflowErrorSetup(t *testing.T) setupParams {
	t.Helper()

	formID := uuid.New()
	workflowJSON := []byte("[]")
	var answers []answer.Answer
	answerableMap := map[string]question.Answerable{}

	return setupParams{
		formID:        formID,
		workflowJSON:  workflowJSON,
		answers:       answers,
		answerableMap: answerableMap,
	}
}

func buildAnswer(t *testing.T, questionID, responseID uuid.UUID, value any) answer.Answer {
	t.Helper()

	valueBytes, err := json.Marshal(value)
	require.NoError(t, err)

	return answer.Answer{
		ID:         responseID,
		ResponseID: responseID,
		QuestionID: questionID,
		Value:      valueBytes,
		CreatedAt:  pgtype.Timestamptz{},
		UpdatedAt:  pgtype.Timestamptz{},
	}
}

// Helper function to create a SingleChoice answerable for testing
func createSingleChoiceAnswerable(t *testing.T, questionID, sectionID, formID, choiceID uuid.UUID) question.Answerable {
	choiceMetadata, err := json.Marshal(map[string]interface{}{
		"choice": []map[string]interface{}{
			{
				"id":          choiceID.String(),
				"name":        "Option A",
				"description": "",
			},
		},
	})
	require.NoError(t, err)

	q := question.Question{
		ID:              questionID,
		SectionID:       sectionID,
		Required:        false,
		Type:            "single_choice",
		Title:           pgtype.Text{String: "Test Question", Valid: true},
		DescriptionJson: []byte(`{"type":"doc","content":[]}`),
		DescriptionHtml: "",
		Metadata:        choiceMetadata,
		Order:           1,
	}
	answerable, err := question.NewSingleChoice(q, formID)
	require.NoError(t, err)
	return answerable
}

// Helper function to create a MultipleChoice answerable for testing
func createMultipleChoiceAnswerable(t *testing.T, questionID, sectionID, formID uuid.UUID, choiceIDs ...uuid.UUID) question.Answerable {
	choices := make([]map[string]interface{}, len(choiceIDs))
	for i, choiceID := range choiceIDs {
		choices[i] = map[string]interface{}{
			"id":          choiceID.String(),
			"name":        "Option",
			"description": "",
		}
	}

	choiceMetadata, err := json.Marshal(map[string]interface{}{
		"choice": choices,
	})
	require.NoError(t, err)

	q := question.Question{
		ID:              questionID,
		SectionID:       sectionID,
		Required:        false,
		Type:            "multiple_choice",
		Title:           pgtype.Text{String: "Test Question", Valid: true},
		DescriptionJson: []byte(`{"type":"doc","content":[]}`),
		DescriptionHtml: "",
		Metadata:        choiceMetadata,
		Order:           1,
	}
	answerable, err := question.NewMultiChoice(q, formID)
	require.NoError(t, err)
	return answerable
}

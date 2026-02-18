package workflow_test

import (
	"NYCU-SDC/core-system-backend/internal/form/answer"
	"context"
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"NYCU-SDC/core-system-backend/internal/form/workflow"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

func TestService_ResolveSections_Simple(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()
	tracer := noop.NewTracerProvider().Tracer("test")
	formID := uuid.New()

	// Create a simple workflow: Start -> Section A -> Section B -> End
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

	mockQuerier := new(mockQuerier)
	service := workflow.NewServiceForTesting(logger, tracer, mockQuerier, nil, nil)

	mockQuerier.On("Get", mock.Anything, formID).Return(workflow.GetRow{
		ID:       uuid.New(),
		FormID:   formID,
		Workflow: workflowJSON,
	}, nil).Once()

	// No answers provided
	var answers []answer.Answer
	answerableMap := map[string]question.Answerable{} // Empty since no questions

	result, err := service.ResolveSections(ctx, formID, answers, answerableMap)

	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Equal(t, sectionAID, result[0])
	require.Equal(t, sectionBID, result[1])

	mockQuerier.AssertExpectations(t)
}

func TestService_ResolveSections_WithCondition_ChoiceTrue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()
	tracer := noop.NewTracerProvider().Tracer("test")
	formID := uuid.New()

	// Create workflow: Start -> Section A -> Condition -> (true: Section B) / (false: Section C) -> End
	startID := uuid.New()
	sectionAID := uuid.New()
	conditionID := uuid.New()
	sectionBID := uuid.New()
	sectionCID := uuid.New()
	endID := uuid.New()
	questionID := uuid.New()
	choiceID := uuid.New()

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
				"source":  "choice",
				"nodeId":  sectionAID.String(),
				"key":     questionID.String(),
				"pattern": "^" + choiceID.String() + "$", // Exact match for choice ID
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

	mockQuerier := new(mockQuerier)
	service := workflow.NewServiceForTesting(logger, tracer, mockQuerier, nil, nil)

	mockQuerier.On("Get", mock.Anything, formID).Return(workflow.GetRow{
		ID:       uuid.New(),
		FormID:   formID,
		Workflow: workflowJSON,
	}, nil).Once()

	// Create answer with the matching choice ID
	answerValue, err := json.Marshal(shared.SingleChoiceAnswer{
		ChoiceID: choiceID,
		Snapshot: shared.ChoiceSnapshot{Name: "Option A", Description: ""},
	})
	require.NoError(t, err)

	answers := []answer.Answer{
		{
			ID:         uuid.New(),
			ResponseID: uuid.New(),
			QuestionID: questionID,
			Value:      answerValue,
			CreatedAt:  pgtype.Timestamptz{},
			UpdatedAt:  pgtype.Timestamptz{},
		},
	}

	// Create answerable for the question (SingleChoice type)
	answerable := createSingleChoiceAnswerable(t, questionID, sectionAID, formID, choiceID)

	answerableMap := map[string]question.Answerable{
		questionID.String(): answerable,
	}

	result, err := service.ResolveSections(ctx, formID, answers, answerableMap)

	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Equal(t, sectionAID, result[0])
	require.Equal(t, sectionBID, result[1]) // Should follow nextTrue (8 matches pattern)

	mockQuerier.AssertExpectations(t)
}

func TestService_ResolveSections_MultipleChoice_AnyMatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()
	tracer := noop.NewTracerProvider().Tracer("test")
	formID := uuid.New()

	// Create workflow with condition checking multiple choice
	startID := uuid.New()
	sectionAID := uuid.New()
	conditionID := uuid.New()
	sectionBID := uuid.New()
	sectionCID := uuid.New()
	endID := uuid.New()
	questionID := uuid.New()
	targetChoiceID := uuid.New()
	otherChoiceID := uuid.New()

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
				"source":  "choice",
				"nodeId":  sectionAID.String(),
				"key":     questionID.String(),
				"pattern": "^" + targetChoiceID.String() + "$",
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

	mockQuerier := new(mockQuerier)
	service := workflow.NewServiceForTesting(logger, tracer, mockQuerier, nil, nil)

	mockQuerier.On("Get", mock.Anything, formID).Return(workflow.GetRow{
		ID:       uuid.New(),
		FormID:   formID,
		Workflow: workflowJSON,
	}, nil).Once()

	// Create multiple choice answer with target choice ID among selected choices
	answerValue, err := json.Marshal(shared.MultipleChoiceAnswer{
		Choices: []struct {
			ChoiceID uuid.UUID             `json:"choiceId"`
			Snapshot shared.ChoiceSnapshot `json:"snapshot"`
		}{
			{
				ChoiceID: otherChoiceID,
				Snapshot: shared.ChoiceSnapshot{Name: "Option A", Description: ""},
			},
			{
				ChoiceID: targetChoiceID, // This one matches!
				Snapshot: shared.ChoiceSnapshot{Name: "Option B", Description: ""},
			},
		},
	})
	require.NoError(t, err)

	answers := []answer.Answer{
		{
			ID:         uuid.New(),
			ResponseID: uuid.New(),
			QuestionID: questionID,
			Value:      answerValue,
			CreatedAt:  pgtype.Timestamptz{},
			UpdatedAt:  pgtype.Timestamptz{},
		},
	}

	// Create answerable for the question (MultipleChoice type)
	answerable := createMultipleChoiceAnswerable(t, questionID, sectionAID, formID, targetChoiceID, otherChoiceID)

	answerableMap := map[string]question.Answerable{
		questionID.String(): answerable,
	}

	result, err := service.ResolveSections(ctx, formID, answers, answerableMap)

	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Equal(t, sectionAID, result[0])
	require.Equal(t, sectionBID, result[1]) // Should follow nextTrue (one choice matches)

	mockQuerier.AssertExpectations(t)
}

func TestService_ResolveSections_EmptyWorkflow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()
	tracer := noop.NewTracerProvider().Tracer("test")
	formID := uuid.New()

	// Empty workflow
	workflowJSON := []byte("[]")

	mockQuerier := new(mockQuerier)
	service := workflow.NewServiceForTesting(logger, tracer, mockQuerier, nil, nil)

	mockQuerier.On("Get", mock.Anything, formID).Return(workflow.GetRow{
		ID:       uuid.New(),
		FormID:   formID,
		Workflow: workflowJSON,
	}, nil).Once()

	var answers []answer.Answer

	// Empty answerableMap for empty workflow test
	answerableMap := map[string]question.Answerable{}

	result, err := service.ResolveSections(ctx, formID, answers, answerableMap)

	require.Error(t, err)
	require.Contains(t, err.Error(), "start node not found")
	require.Nil(t, result)

	mockQuerier.AssertExpectations(t)
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
		ID:          questionID,
		SectionID:   sectionID,
		Required:    false,
		Type:        "single_choice",
		Title:       pgtype.Text{String: "Test Question", Valid: true},
		Description: pgtype.Text{String: "", Valid: false},
		Metadata:    choiceMetadata,
		Order:       1,
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
		ID:          questionID,
		SectionID:   sectionID,
		Required:    false,
		Type:        "multiple_choice",
		Title:       pgtype.Text{String: "Test Question", Valid: true},
		Description: pgtype.Text{String: "", Valid: false},
		Metadata:    choiceMetadata,
		Order:       1,
	}
	answerable, err := question.NewMultiChoice(q, formID)
	require.NoError(t, err)
	return answerable
}

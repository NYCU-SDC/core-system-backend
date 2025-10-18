package question_test

import (
	"context"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/question/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestQuestion(t question.QuestionType) question.Question {
	q := question.Question{Type: t}

	switch t {
	case question.QuestionTypeSingleChoice:
		metadata, _ := question.GenerateMetadata("single_choice", []question.ChoiceOption{{Name: "A"}, {Name: "B"}})
		q.Metadata = metadata
	case question.QuestionTypeMultipleChoice:
		metadata, _ := question.GenerateMetadata("multiple_choice", []question.ChoiceOption{{Name: "A"}, {Name: "B"}})
		q.Metadata = metadata
	default:
		q.Metadata = []byte(`{}`)
	}
	return q
}

func TestService_Create_KnownAndUnknown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		createReturn question.Question
		expectedErr  bool
	}{
		{
			name:         "Known type (ShortText) -> success",
			createReturn: newTestQuestion(question.QuestionTypeShortText),
			expectedErr:  false,
		},
		{
			name:         "Known type (LongText) -> success",
			createReturn: newTestQuestion(question.QuestionTypeLongText),
			expectedErr:  false,
		},
		{
			name:         "Known type (SingleChoice) -> success",
			createReturn: newTestQuestion(question.QuestionTypeSingleChoice),
			expectedErr:  false,
		},
		{
			name:         "Known type (MultipleChoice) -> success",
			createReturn: newTestQuestion(question.QuestionTypeMultipleChoice),
			expectedErr:  false,
		},
		{
			name:         "Known type (Date) -> success",
			createReturn: newTestQuestion(question.QuestionTypeDate),
			expectedErr:  false,
		},
		{
			name:         "Unknown type (Unknown) -> error",
			createReturn: newTestQuestion(question.QuestionType("___UNKNOWN___")),
			expectedErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockQuerier := mocks.NewMockQuerier(t)
			mockQuerier.EXPECT().
				Create(mock.Anything, question.CreateParams{}).
				Return(tc.createReturn, nil).Once()
			logger := zap.NewNop()
			service := question.NewService(logger, mockQuerier)

			got, err := service.Create(ctx, question.CreateParams{})
			if tc.expectedErr {
				require.Error(t, err, "expected error but got nil")
				require.Nil(t, got)
				return
			}

			require.NoError(t, err, "unexpected error occurred")
			require.NotNil(t, got, "should return an Answerable")
			require.Equal(t, tc.createReturn.Type, got.Question().Type)
		})
	}
}

func TestService_ListByFormID_AllKnown_And_ContainsUnknown(t *testing.T) {
	t.Parallel()

	formID := uuid.New()

	allKnown := []question.Question{
		newTestQuestion(question.QuestionTypeShortText),
		newTestQuestion(question.QuestionTypeSingleChoice),
	}

	withUnknown := []question.Question{
		newTestQuestion(question.QuestionTypeLongText),
		newTestQuestion(question.QuestionType("___UNKNOWN___")),
	}

	tests := []struct {
		name       string
		listReturn []question.Question
		wantCount  int
		wantErr    bool
	}{
		{
			name:       "All known types -> return []Answerable",
			listReturn: allKnown,
			wantCount:  len(allKnown),
			wantErr:    false,
		},
		{
			name:       "Contains an unknown type -> fail",
			listReturn: withUnknown,
			wantCount:  0,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockQuerier := mocks.NewMockQuerier(t)
			mockQuerier.EXPECT().
				ListByFormID(mock.Anything, formID).
				Return(tc.listReturn, nil).Once()

			logger := zap.NewNop()
			service := question.NewService(logger, mockQuerier)

			got, err := service.ListByFormID(ctx, formID)

			if tc.wantErr {
				require.Error(t, err, "expected error but got nil")
				require.Nil(t, got)
				return
			}

			require.NoError(t, err, "unexpected error")
			require.Len(t, got, tc.wantCount)

			for i, a := range got {
				require.NotNil(t, a, "answerable[%d] should not be nil", i)
				require.Equal(t, tc.listReturn[i].Type, a.Question().Type)
			}
		})
	}
}

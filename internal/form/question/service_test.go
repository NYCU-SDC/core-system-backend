package question_test

import (
	"context"
	"errors"
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

func TestService_Create_DBError(t *testing.T) {
	ctx := context.Background()
	mockQuerier := mocks.NewMockQuerier(t)
	mockQuerier.EXPECT().
		Create(mock.Anything, question.CreateParams{}).
		Return(question.Question{}, errors.New("db down")).Once()

	service := question.NewService(zap.NewNop(), mockQuerier)

	got, err := service.Create(ctx, question.CreateParams{})
	require.Error(t, err)
	require.Nil(t, got)
}

func TestService_Create_InvalidMetadata(t *testing.T) {
	ctx := context.Background()
	q := question.Question{ // Single Choice but given empty metadata
		Type:     question.QuestionTypeSingleChoice,
		Metadata: []byte(`{}`),
	}

	mockQuerier := mocks.NewMockQuerier(t)
	mockQuerier.EXPECT().
		Create(mock.Anything, question.CreateParams{}).
		Return(q, nil).Once()

	service := question.NewService(zap.NewNop(), mockQuerier)

	got, err := service.Create(ctx, question.CreateParams{})
	require.Error(t, err)
	require.Equal(t, got, question.SingleChoice{})
}

func TestService_Create_ChineseInputs_Validate(t *testing.T) {
	type params struct {
		createReturn question.Question
		valid        []string
		invalid      []string
	}

	tests := []struct {
		name        string
		params      params
		setup       func(t *testing.T, params *params) context.Context
		validate    func(t *testing.T, params *params, answerable question.Answerable)
		expectedErr bool
	}{
		{
			name: "SingleChoice with Chinese options",
			params: params{
				createReturn: func() question.Question {
					choices := []question.ChoiceOption{{Name: "是"}, {Name: "否"}}
					metadata, _ := question.GenerateMetadata("single_choice", choices)
					return question.Question{Type: question.QuestionTypeSingleChoice, Metadata: metadata}
				}(),
			},
			setup: func(t *testing.T, params *params) context.Context {
				choices, err := question.ExtractChoices(params.createReturn.Metadata)
				require.NoError(t, err)
				var idYes, idNo string
				for _, choice := range choices {
					if choice.Name == "是" {
						idYes = choice.ID.String()
					}
					if choice.Name == "否" {
						idNo = choice.ID.String()
					}
				}
				require.NotEmpty(t, idYes)
				require.NotEmpty(t, idNo)
				params.valid = []string{idYes, idNo, ""}
				params.invalid = []string{"not-a-valid-id"}
				return context.Background()
			},
			validate: func(t *testing.T, params *params, answerable question.Answerable) {
				for _, valid := range params.valid {
					require.NoErrorf(t, answerable.Validate(valid), "valid input %q should pass", valid)
				}
				for _, invalid := range params.invalid {
					require.Errorf(t, answerable.Validate(invalid), "invalid input %q should fail", invalid)
				}
			},
		},
		{
			name: "MultipleChoice with Chinese options",
			params: params{
				createReturn: func() question.Question {
					metadata, _ := question.GenerateMetadata("multiple_choice",
						[]question.ChoiceOption{{Name: "選項一"}, {Name: "選項二"}, {Name: "選項三"}})
					return question.Question{Type: question.QuestionTypeMultipleChoice, Metadata: metadata}
				}(),
			},
			setup: func(t *testing.T, params *params) context.Context {
				choices, err := question.ExtractChoices(params.createReturn.Metadata)
				require.NoError(t, err)

				nameToID := map[string]string{}
				for _, choice := range choices {
					nameToID[choice.Name] = choice.ID.String()
				}
				require.Contains(t, nameToID, "選項一")
				require.Contains(t, nameToID, "選項二")
				require.Contains(t, nameToID, "選項三")
				id1 := nameToID["選項一"]
				id2 := nameToID["選項二"]
				id3 := nameToID["選項三"]

				params.valid = []string{id1, id1 + ";" + id2, id3 + ";" + id2}
				params.invalid = []string{id1 + ";not-a-valid-id"}
				return context.Background()
			},
			validate: func(t *testing.T, params *params, answerable question.Answerable) {
				for _, valid := range params.valid {
					require.NoErrorf(t, answerable.Validate(valid), "valid input %q should pass", valid)
				}
				for _, invalid := range params.invalid {
					require.Errorf(t, answerable.Validate(invalid), "invalid input %q should fail", invalid)
				}
			},
		},
		{
			name: "ShortText accepts Chinese",
			params: params{
				createReturn: question.Question{
					Type:     question.QuestionTypeShortText,
					Metadata: []byte(`{}`),
				},
				valid:   []string{"中文輸入短字串", "中文短字串test"},
				invalid: nil,
			},
			setup: func(t *testing.T, params *params) context.Context {
				return context.Background()
			},
			validate: func(t *testing.T, params *params, answerable question.Answerable) {
				for _, valid := range params.valid {
					require.NoErrorf(t, answerable.Validate(valid), "valid input %q should pass", valid)
				}
			},
		},
		{
			name: "LongText accepts Chinese",
			params: params{
				createReturn: question.Question{
					Type:     question.QuestionTypeLongText,
					Metadata: []byte(`{}`),
				},
				valid:   []string{"這是要測試 LongText 是不是可以用中文，但他是 LongText 所以會長一點點，長字串長字串長字串長字串", "多行中文\n也要可以通過"},
				invalid: nil,
			},
			setup: func(t *testing.T, params *params) context.Context {
				return context.Background()
			},
			validate: func(t *testing.T, params *params, answerable question.Answerable) {
				for _, valid := range params.valid {
					require.NoErrorf(t, answerable.Validate(valid), "valid input %q should pass", valid)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			params := tc.params
			if tc.setup != nil {
				ctx = tc.setup(t, &params)
			}

			mockQuerier := mocks.NewMockQuerier(t)
			mockQuerier.EXPECT().
				Create(mock.Anything, question.CreateParams{}).
				Return(params.createReturn, nil).Once()

			svc := question.NewService(zap.NewNop(), mockQuerier)

			ans, err := svc.Create(ctx, question.CreateParams{})

			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)
			if tc.expectedErr {
				return
			}

			require.NoError(t, err)
			require.NotNil(t, ans)

			if tc.validate != nil {
				tc.validate(t, &params, ans)
			}
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
		name          string
		listReturn    []question.Question
		wantCount     int
		expectedError bool
	}{
		{
			name:          "All known types -> return []Answerable",
			listReturn:    allKnown,
			wantCount:     len(allKnown),
			expectedError: false,
		},
		{
			name:          "Contains an unknown type -> fail",
			listReturn:    withUnknown,
			wantCount:     0,
			expectedError: true,
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

			if tc.expectedError {
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

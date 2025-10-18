package submit_test

import (
	"NYCU-SDC/core-system-backend/internal/form/submit"
	"context"
	"errors"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"

	"NYCU-SDC/core-system-backend/internal/form/submit/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeAnswerable struct {
	q           question.Question
	validateErr error
}

func (f fakeAnswerable) Question() question.Question {
	return f.q
}
func (f fakeAnswerable) Validate(value string) error {
	return f.validateErr
}

func ans(qID uuid.UUID, val string) shared.AnswerParam {
	return shared.AnswerParam{QuestionID: qID.String(), Value: val}
}

func makeQ(questionID uuid.UUID, t question.QuestionType, validateErr error) question.Answerable {
	return fakeAnswerable{
		q: question.Question{
			ID:   questionID,
			Type: t,
		},
		validateErr: validateErr,
	}
}

func TestSubmitService_Submit(t *testing.T) {
	type Params struct {
		formID  uuid.UUID
		userID  uuid.UUID
		answers []shared.AnswerParam
		qs      []question.Answerable

		qStore  *mocks.MockQuestionStore
		rStore  *mocks.MockFormResponseStore
		service *submit.Service
	}

	type testCase struct {
		name        string
		params      Params
		expectedErr bool
		setup       func(t *testing.T, p *Params) context.Context
		validate    func(t *testing.T, p Params, got response.FormResponse, errs []error)
	}

	formID := uuid.New()
	userID := uuid.New()
	q1, q2 := uuid.New(), uuid.New()
	unknown := uuid.New()

	testCases := []testCase{
		{
			name: "CreateOrUpdate called with mapped types if all valid",
			params: Params{
				formID:  formID,
				userID:  userID,
				qs:      []question.Answerable{makeQ(q1, question.QuestionTypeShortText, nil), makeQ(q2, question.QuestionTypeLongText, nil)},
				answers: []shared.AnswerParam{ans(q1, "hello"), ans(q2, "world")},
			},
			setup: func(t *testing.T, p *Params) context.Context {
				qm := mocks.NewMockQuestionStore(t)
				rm := mocks.NewMockFormResponseStore(t)

				qm.EXPECT().
					ListByFormID(mock.Anything, p.formID).
					Return(p.qs, nil).Once()

				respID := uuid.New()
				rm.EXPECT().
					CreateOrUpdate(
						mock.Anything,
						p.formID,
						p.userID,
						p.answers,
						[]response.QuestionType{
							response.QuestionType(question.QuestionTypeShortText),
							response.QuestionType(question.QuestionTypeLongText),
						},
					).
					Return(response.FormResponse{ID: respID, FormID: p.formID, SubmittedBy: p.userID}, nil).
					Once()

				p.qStore = qm
				p.rStore = rm
				p.service = submit.NewService(zap.NewNop(), qm, rm)
				return context.Background()
			},
			validate: func(t *testing.T, p Params, got response.FormResponse, errs []error) {
				require.Nil(t, errs)
				require.Equal(t, p.formID, got.FormID)
				require.Equal(t, p.userID, got.SubmittedBy)
				require.NotEqual(t, uuid.Nil, got.ID)
			},
		},
		{
			name: "Not call CreateOrUpdate if there are some invalid answers",
			params: Params{
				formID:  formID,
				userID:  userID,
				qs:      []question.Answerable{makeQ(q1, question.QuestionTypeShortText, nil), makeQ(q2, question.QuestionTypeLongText, errors.New("invalid"))},
				answers: []shared.AnswerParam{ans(q1, "ok"), ans(q2, "bad")},
			},
			expectedErr: true,
			setup: func(t *testing.T, p *Params) context.Context {
				qm := mocks.NewMockQuestionStore(t)
				rm := mocks.NewMockFormResponseStore(t)

				qm.EXPECT().
					ListByFormID(mock.Anything, p.formID).
					Return(p.qs, nil).Once()

				p.qStore = qm
				p.rStore = rm
				p.service = submit.NewService(zap.NewNop(), qm, rm)
				return context.Background()
			},
			validate: func(t *testing.T, p Params, got response.FormResponse, errs []error) {
				require.NotNil(t, errs)
				require.Len(t, errs, 1)
				require.Equal(t, uuid.Nil, got.ID)
				p.rStore.AssertNotCalled(t, "CreateOrUpdate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			},
		},
		{
			name: "Not call CreateOrUpdate if answer refers to unknown question ",
			params: Params{
				formID:  formID,
				userID:  userID,
				qs:      []question.Answerable{makeQ(q1, question.QuestionTypeShortText, nil)},
				answers: []shared.AnswerParam{ans(q1, "ok"), ans(unknown, "???")},
			},
			expectedErr: true,
			setup: func(t *testing.T, p *Params) context.Context {
				qm := mocks.NewMockQuestionStore(t)
				rm := mocks.NewMockFormResponseStore(t)

				qm.EXPECT().
					ListByFormID(mock.Anything, p.formID).
					Return(p.qs, nil).Once()

				p.qStore = qm
				p.rStore = rm
				p.service = submit.NewService(zap.NewNop(), qm, rm)
				return context.Background()
			},
			validate: func(t *testing.T, p Params, got response.FormResponse, errs []error) {
				require.NotNil(t, errs)
				require.Len(t, errs, 1)
				require.Equal(t, uuid.Nil, got.ID)
				p.rStore.AssertNotCalled(t, "CreateOrUpdate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			},
		},
		{
			name: "Return error and do not call CreateOrUpdate if ListByFormID fails",
			params: Params{
				formID:  formID,
				userID:  userID,
				answers: []shared.AnswerParam{ans(q1, "x")},
			},
			expectedErr: true,
			setup: func(t *testing.T, p *Params) context.Context {
				qm := mocks.NewMockQuestionStore(t)
				rm := mocks.NewMockFormResponseStore(t)

				qm.EXPECT().
					ListByFormID(mock.Anything, p.formID).
					Return(nil, errors.New("db down")).Once()

				p.qStore = qm
				p.rStore = rm
				p.service = submit.NewService(zap.NewNop(), qm, rm)
				return context.Background()
			},
			validate: func(t *testing.T, p Params, got response.FormResponse, errs []error) {
				require.NotNil(t, errs)
				require.Equal(t, uuid.Nil, got.ID)
				p.rStore.AssertNotCalled(t, "CreateOrUpdate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			},
		},
		{
			name: "Return error if validation passes but CreateOrUpdate fails",
			params: Params{
				formID:  formID,
				userID:  userID,
				qs:      []question.Answerable{makeQ(q1, question.QuestionTypeShortText, nil)},
				answers: []shared.AnswerParam{ans(q1, "ok")},
			},
			expectedErr: true,
			setup: func(t *testing.T, p *Params) context.Context {
				qm := mocks.NewMockQuestionStore(t)
				rm := mocks.NewMockFormResponseStore(t)

				qm.EXPECT().
					ListByFormID(mock.Anything, p.formID).
					Return(p.qs, nil).Once()

				rm.EXPECT().
					CreateOrUpdate(
						mock.Anything,
						p.formID,
						p.userID,
						p.answers,
						[]response.QuestionType{response.QuestionType(question.QuestionTypeShortText)},
					).
					Return(response.FormResponse{}, errors.New("insert fail")).
					Once()

				p.qStore = qm
				p.rStore = rm
				p.service = submit.NewService(zap.NewNop(), qm, rm)
				return context.Background()
			},
			validate: func(t *testing.T, p Params, got response.FormResponse, errs []error) {
				require.NotNil(t, errs)
				require.Equal(t, uuid.Nil, got.ID)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			params := tc.params
			if tc.setup != nil {
				ctx = tc.setup(t, &params)
			}

			got, errs := params.service.Submit(ctx, params.formID, params.userID, params.answers)
			require.Equal(t, tc.expectedErr, len(errs) > 0, "expectedErr=%v, errs=%v", tc.expectedErr, errs)

			if tc.validate != nil {
				tc.validate(t, params, got, errs)
			}
		})
	}
}

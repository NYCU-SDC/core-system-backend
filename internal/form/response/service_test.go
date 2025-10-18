package response_test

import (
	"context"
	"errors"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/response/mocks"
	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newSvcWithMock(q response.Querier) *response.Service {
	return response.NewService(zap.NewNop(), q)
}

func newTestAnswerParam(qID uuid.UUID, val string) shared.AnswerParam {
	return shared.AnswerParam{
		QuestionID: qID.String(),
		Value:      val,
	}
}

func TestService_CreateOrUpdate(t *testing.T) {
	type Params struct {
		formID uuid.UUID
		userID uuid.UUID
		ans    []shared.AnswerParam
		types  []response.QuestionType

		service *response.Service
		mock    *mocks.MockQuerier
	}

	type testCase struct {
		name        string
		params      Params
		expectedErr bool
		setup       func(t *testing.T, p *Params) context.Context
		validate    func(t *testing.T, p Params, got response.FormResponse)
	}

	testCases := []testCase{
		{
			name: "Return error if answers length != question types length",
			params: Params{
				formID: uuid.New(),
				userID: uuid.New(),
				ans:    []shared.AnswerParam{newTestAnswerParam(uuid.New(), "A")}, // len=1
				types:  []response.QuestionType{},                                 // len=0
			},
			expectedErr: true,
			setup: func(t *testing.T, p *Params) context.Context {
				q := mocks.NewMockQuerier(t)
				p.mock = q
				p.service = newSvcWithMock(q)
				return context.Background()
			},
		},
		{
			name: "Create response and answers if not exists",
			params: Params{
				formID: uuid.New(),
				userID: uuid.New(),
			},
			setup: func(t *testing.T, p *Params) context.Context {
				respID := uuid.New()
				q1, q2 := uuid.New(), uuid.New()

				p.ans = []shared.AnswerParam{newTestAnswerParam(q1, "v1"), newTestAnswerParam(q2, "v2")}
				p.types = []response.QuestionType{response.QuestionTypeShortText, response.QuestionTypeLongText}

				q := mocks.NewMockQuerier(t)

				q.EXPECT().
					Exists(mock.Anything, response.ExistsParams{FormID: p.formID, SubmittedBy: p.userID}).
					Return(false, nil).Once()

				q.EXPECT().
					Create(mock.Anything, mock.MatchedBy(func(cp response.CreateParams) bool {
						return cp.FormID == p.formID && cp.SubmittedBy == p.userID
					})).
					Return(response.FormResponse{ID: respID, FormID: p.formID, SubmittedBy: p.userID}, nil).Once()

				q.EXPECT().
					CreateAnswer(mock.Anything, mock.MatchedBy(func(a response.CreateAnswerParams) bool {
						return a.ResponseID == respID && a.QuestionID == q1 && a.Value == "v1"
					})).
					Return(response.Answer{ID: uuid.New(), ResponseID: respID, QuestionID: q1}, nil).Once()

				q.EXPECT().
					CreateAnswer(mock.Anything, mock.MatchedBy(func(a response.CreateAnswerParams) bool {
						return a.ResponseID == respID && a.QuestionID == q2 && a.Value == "v2"
					})).
					Return(response.Answer{ID: uuid.New(), ResponseID: respID, QuestionID: q2}, nil).Once()

				p.mock = q
				p.service = newSvcWithMock(q)
				return context.Background()
			},
			validate: func(t *testing.T, p Params, got response.FormResponse) {
				require.Equal(t, p.formID, got.FormID)
				require.Equal(t, p.userID, got.SubmittedBy)
				require.NotEqual(t, uuid.Nil, got.ID)
			},
		},
		{
			name: "Update if it exists",
			params: Params{
				formID: uuid.New(),
				userID: uuid.New(),
				ans:    []shared.AnswerParam{},
				types:  []response.QuestionType{},
			},
			setup: func(t *testing.T, p *Params) context.Context {
				respID := uuid.New()
				q := mocks.NewMockQuerier(t)

				q.EXPECT().
					Exists(mock.Anything, response.ExistsParams{FormID: p.formID, SubmittedBy: p.userID}).
					Return(true, nil).Once()

				q.EXPECT().
					GetByFormIDAndSubmittedBy(mock.Anything, response.GetByFormIDAndSubmittedByParams{
						FormID: p.formID, SubmittedBy: p.userID,
					}).
					Return(response.FormResponse{ID: respID, FormID: p.formID, SubmittedBy: p.userID}, nil).Once()

				q.EXPECT().Update(mock.Anything, respID).Return(nil).Once()

				p.mock = q
				p.service = newSvcWithMock(q)
				return context.Background()
			},
			validate: func(t *testing.T, p Params, got response.FormResponse) {
				require.Equal(t, p.formID, got.FormID)
				require.Equal(t, p.userID, got.SubmittedBy)
			},
		},
		{
			name: "Return error if Exists query fails",
			params: Params{
				formID: uuid.New(),
				userID: uuid.New(),
			},
			expectedErr: true,
			setup: func(t *testing.T, p *Params) context.Context {
				q := mocks.NewMockQuerier(t)
				q.EXPECT().
					Exists(mock.Anything, response.ExistsParams{FormID: p.formID, SubmittedBy: p.userID}).
					Return(false, errors.New("db down")).Once()
				p.mock = q
				p.service = newSvcWithMock(q)
				return context.Background()
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			params := tc.params
			if tc.setup != nil {
				ctx = tc.setup(t, &params)
			}

			result, err := params.service.CreateOrUpdate(ctx, params.formID, params.userID, params.ans, params.types)
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.expectedErr {
				return
			}
			if tc.validate != nil {
				tc.validate(t, params, result)
			}
		})
	}
}

func TestService_Update(t *testing.T) {
	type Params struct {
		formID uuid.UUID
		userID uuid.UUID
		ans    []shared.AnswerParam
		types  []response.QuestionType

		respID uuid.UUID
		qNew   uuid.UUID
		qSame  uuid.UUID
		qDiff  uuid.UUID

		service *response.Service
		mock    *mocks.MockQuerier
	}

	type testCase struct {
		name        string
		params      Params
		expectedErr bool
		setup       func(t *testing.T, p *Params) context.Context
		validate    func(t *testing.T, p Params, got response.FormResponse)
	}

	testCases := []testCase{
		{
			name: "Create answer if not exist",
			params: Params{
				formID: uuid.New(),
				userID: uuid.New(),
				qNew:   uuid.New(),
			},
			setup: func(t *testing.T, p *Params) context.Context {
				p.respID = uuid.New()
				p.ans = []shared.AnswerParam{newTestAnswerParam(p.qNew, "aaa")}
				p.types = []response.QuestionType{response.QuestionTypeShortText}

				q := mocks.NewMockQuerier(t)

				q.EXPECT().
					GetByFormIDAndSubmittedBy(mock.Anything, response.GetByFormIDAndSubmittedByParams{
						FormID: p.formID, SubmittedBy: p.userID,
					}).
					Return(response.FormResponse{ID: p.respID, FormID: p.formID, SubmittedBy: p.userID}, nil).Once()

				q.EXPECT().
					AnswerExists(mock.Anything, response.AnswerExistsParams{ResponseID: p.respID, QuestionID: p.qNew}).
					Return(false, nil).Once()

				q.EXPECT().
					CreateAnswer(mock.Anything, mock.MatchedBy(func(a response.CreateAnswerParams) bool {
						return a.ResponseID == p.respID && a.QuestionID == p.qNew && a.Value == "aaa"
					})).
					Return(response.Answer{ID: uuid.New(), ResponseID: p.respID, QuestionID: p.qNew}, nil).Once()

				q.EXPECT().Update(mock.Anything, p.respID).Return(nil).Once()

				p.mock = q
				p.service = newSvcWithMock(q)
				return context.Background()
			},
			validate: func(t *testing.T, p Params, got response.FormResponse) {
				require.Equal(t, p.formID, got.FormID)
				require.Equal(t, p.userID, got.SubmittedBy)
			},
		},
		{
			name: "Do nothing when answer exists and content is the same",
			params: Params{
				formID: uuid.New(),
				userID: uuid.New(),
				qSame:  uuid.New(),
			},
			setup: func(t *testing.T, p *Params) context.Context {
				p.respID = uuid.New()
				p.ans = []shared.AnswerParam{newTestAnswerParam(p.qSame, "bbb")}
				p.types = []response.QuestionType{response.QuestionTypeShortText}

				q := mocks.NewMockQuerier(t)

				q.EXPECT().
					GetByFormIDAndSubmittedBy(mock.Anything, response.GetByFormIDAndSubmittedByParams{
						FormID: p.formID, SubmittedBy: p.userID,
					}).
					Return(response.FormResponse{ID: p.respID, FormID: p.formID, SubmittedBy: p.userID}, nil).Once()

				q.EXPECT().
					AnswerExists(mock.Anything, response.AnswerExistsParams{ResponseID: p.respID, QuestionID: p.qSame}).
					Return(true, nil).Once()

				q.EXPECT().
					CheckAnswerContent(mock.Anything, response.CheckAnswerContentParams{
						ResponseID: p.respID, QuestionID: p.qSame, Value: "bbb",
					}).
					Return(true, nil).Once()

				q.EXPECT().Update(mock.Anything, p.respID).Return(nil).Once()

				p.mock = q
				p.service = newSvcWithMock(q)
				return context.Background()
			},
			validate: func(t *testing.T, p Params, got response.FormResponse) {
				require.Equal(t, p.formID, got.FormID)
				require.Equal(t, p.userID, got.SubmittedBy)
				p.mock.AssertNotCalled(t, "UpdateAnswer", mock.Anything, mock.Anything)
			},
		},
		{
			name: "Update answer when it exists and content is different",
			params: Params{
				formID: uuid.New(),
				userID: uuid.New(),
				qDiff:  uuid.New(),
			},
			setup: func(t *testing.T, p *Params) context.Context {
				p.respID = uuid.New()
				p.ans = []shared.AnswerParam{newTestAnswerParam(p.qDiff, "ccc")}
				p.types = []response.QuestionType{response.QuestionTypeShortText}
				ansID := uuid.New()

				q := mocks.NewMockQuerier(t)

				q.EXPECT().
					GetByFormIDAndSubmittedBy(mock.Anything, response.GetByFormIDAndSubmittedByParams{
						FormID: p.formID, SubmittedBy: p.userID,
					}).
					Return(response.FormResponse{ID: p.respID, FormID: p.formID, SubmittedBy: p.userID}, nil).Once()

				q.EXPECT().
					AnswerExists(mock.Anything, response.AnswerExistsParams{ResponseID: p.respID, QuestionID: p.qDiff}).
					Return(true, nil).Once()

				q.EXPECT().
					CheckAnswerContent(mock.Anything, response.CheckAnswerContentParams{
						ResponseID: p.respID, QuestionID: p.qDiff, Value: "ccc",
					}).
					Return(false, nil).Once()

				q.EXPECT().
					GetAnswerID(mock.Anything, response.GetAnswerIDParams{ResponseID: p.respID, QuestionID: p.qDiff}).
					Return(ansID, nil).Once()

				q.EXPECT().
					UpdateAnswer(mock.Anything, response.UpdateAnswerParams{ID: ansID, Value: "ccc"}).
					Return(response.Answer{ID: ansID, Value: "ccc"}, nil).Once()

				q.EXPECT().Update(mock.Anything, p.respID).Return(nil).Once()

				p.mock = q
				p.service = newSvcWithMock(q)
				return context.Background()
			},
			validate: func(t *testing.T, p Params, got response.FormResponse) {
				require.Equal(t, p.formID, got.FormID)
				require.Equal(t, p.userID, got.SubmittedBy)
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

			result, err := params.service.Update(ctx, params.formID, params.userID, params.ans, params.types)
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.expectedErr {
				return
			}
			if tc.validate != nil {
				tc.validate(t, params, result)
			}
		})
	}
}

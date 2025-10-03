package response

import (
	"context"
	"fmt"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

type fakeQuerier struct {
	existsReturn bool
	existsErr    error

	createResp FormResponse
	createErr  error
	updateErr  error

	getByFormIDAndSubmittedByResp FormResponse
	getByFormIDAndSubmittedByErr  error

	answerExists    map[uuid.UUID]bool
	answerExistsErr error

	sameAnswer    map[uuid.UUID]bool
	sameAnswerErr error

	answerIDByQuestion map[uuid.UUID]uuid.UUID
	answerIDErr        error

	createAnswerErr error
	updateAnswerErr error

	createAnswerCalls []CreateAnswerParams
	updateAnswerCalls []UpdateAnswerParams
	updateResponseIDs []uuid.UUID
}

func (f *fakeQuerier) Create(ctx context.Context, arg CreateParams) (FormResponse, error) {
	return f.createResp, f.createErr
}
func (f *fakeQuerier) Get(ctx context.Context, arg GetParams) (FormResponse, error) {
	panic("not used in these tests")
}
func (f *fakeQuerier) GetByFormIDAndSubmittedBy(ctx context.Context, arg GetByFormIDAndSubmittedByParams) (FormResponse, error) {
	return f.getByFormIDAndSubmittedByResp, f.getByFormIDAndSubmittedByErr
}
func (f *fakeQuerier) Exists(ctx context.Context, arg ExistsParams) (bool, error) {
	return f.existsReturn, f.existsErr
}
func (f *fakeQuerier) ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error) {
	panic("not used in these tests")
}
func (f *fakeQuerier) Update(ctx context.Context, id uuid.UUID) error {
	f.updateResponseIDs = append(f.updateResponseIDs, id)
	return f.updateErr
}
func (f *fakeQuerier) Delete(ctx context.Context, id uuid.UUID) error {
	panic("not used in these tests")
}
func (f *fakeQuerier) CreateAnswer(ctx context.Context, arg CreateAnswerParams) (Answer, error) {
	f.createAnswerCalls = append(f.createAnswerCalls, arg)
	return Answer{ID: uuid.New(), ResponseID: arg.ResponseID, QuestionID: arg.QuestionID, Type: arg.Type, Value: arg.Value}, f.createAnswerErr
}
func (f *fakeQuerier) GetAnswersByQuestionID(ctx context.Context, arg GetAnswersByQuestionIDParams) ([]GetAnswersByQuestionIDRow, error) {
	panic("not used in these tests")
}
func (f *fakeQuerier) GetAnswersByResponseID(ctx context.Context, responseID uuid.UUID) ([]Answer, error) {
	panic("not used in these tests")
}
func (f *fakeQuerier) UpdateAnswer(ctx context.Context, arg UpdateAnswerParams) (Answer, error) {
	f.updateAnswerCalls = append(f.updateAnswerCalls, arg)
	return Answer{ID: arg.ID, Value: arg.Value}, f.updateAnswerErr
}
func (f *fakeQuerier) AnswerExists(ctx context.Context, arg AnswerExistsParams) (bool, error) {
	if f.answerExistsErr != nil {
		return false, f.answerExistsErr
	}
	return f.answerExists[arg.QuestionID], nil
}
func (f *fakeQuerier) CheckAnswerContent(ctx context.Context, arg CheckAnswerContentParams) (bool, error) {
	if f.sameAnswerErr != nil {
		return false, f.sameAnswerErr
	}
	return f.sameAnswer[arg.QuestionID], nil
}
func (f *fakeQuerier) GetAnswerID(ctx context.Context, arg GetAnswerIDParams) (uuid.UUID, error) {
	if f.answerIDErr != nil {
		return uuid.Nil, f.answerIDErr
	}
	id, ok := f.answerIDByQuestion[arg.QuestionID]
	if !ok {
		return uuid.Nil, fmt.Errorf("fakeQuerier.GetAnswerID: no mapping for questionID %s", arg.QuestionID)
	}
	return id, nil
}

func newSvcWithFake(q *fakeQuerier) *Service {
	return &Service{
		logger:  zap.NewNop(),
		queries: q,
		tracer:  otel.Tracer("test"),
	}
}

func ap(qID uuid.UUID, val string) shared.AnswerParam {
	return shared.AnswerParam{
		QuestionID: qID.String(),
		Value:      val,
	}
}

func TestService_CreateOrUpdate_LengthMismatch(t *testing.T) {
	svc := newSvcWithFake(&fakeQuerier{})
	formID := uuid.New()
	userID := uuid.New()

	answers := []shared.AnswerParam{ap(uuid.New(), "A")}
	qtypes := []QuestionType{}

	_, err := svc.CreateOrUpdate(context.Background(), formID, userID, answers, qtypes)
	require.Error(t, err)
}

func TestService_CreateOrUpdate_CreatePath(t *testing.T) {
	respID := uuid.New()
	formID := uuid.New()
	userID := uuid.New()

	q1 := uuid.New()
	q2 := uuid.New()

	f := fakeQuerier{
		existsReturn: false,
		createResp:   FormResponse{ID: respID, FormID: formID, SubmittedBy: userID},
	}
	svc := newSvcWithFake(&f)

	answers := []shared.AnswerParam{ap(q1, "v1"), ap(q2, "v2")}
	qtypes := []QuestionType{QuestionTypeShortText, QuestionTypeLongText}

	got, err := svc.CreateOrUpdate(context.Background(), formID, userID, answers, qtypes)
	require.NoError(t, err)
	require.Equal(t, respID, got.ID)

	require.Len(t, f.createAnswerCalls, 2)
	require.Equal(t, q1, f.createAnswerCalls[0].QuestionID)
	require.Equal(t, q2, f.createAnswerCalls[1].QuestionID)
	require.Equal(t, respID, f.createAnswerCalls[0].ResponseID)
	require.Equal(t, respID, f.createAnswerCalls[1].ResponseID)
}

func TestService_CreateOrUpdate_UpdatePath(t *testing.T) {
	formID := uuid.New()
	userID := uuid.New()
	respID := uuid.New()

	qNew := uuid.New()
	qSame := uuid.New()
	qDiff := uuid.New()

	answerIDDiff := uuid.New()

	f := fakeQuerier{
		existsReturn: true,
		getByFormIDAndSubmittedByResp: FormResponse{
			ID: respID, FormID: formID, SubmittedBy: userID,
		},

		answerExists: map[uuid.UUID]bool{
			qNew:  false,
			qSame: true,
			qDiff: true,
		},
		sameAnswer: map[uuid.UUID]bool{
			qSame: true,
			qDiff: false,
		},
		answerIDByQuestion: map[uuid.UUID]uuid.UUID{
			qDiff: answerIDDiff,
		},
	}
	svc := newSvcWithFake(&f)

	answers := []shared.AnswerParam{
		ap(qNew, "aaa"),
		ap(qSame, "bbb"),
		ap(qDiff, "ccc"),
	}
	qtypes := []QuestionType{
		QuestionTypeShortText,
		QuestionTypeShortText,
		QuestionTypeShortText,
	}

	got, err := svc.CreateOrUpdate(context.Background(), formID, userID, answers, qtypes)
	require.NoError(t, err)
	require.Equal(t, respID, got.ID)

	require.Equal(t, 1, countCreateForQuestion(f.createAnswerCalls, qNew))

	require.Equal(t, 0, countUpdateForQuestion(f.updateAnswerCalls, qSame, f.answerIDByQuestion))

	require.Equal(t, 1, countUpdateForQuestion(f.updateAnswerCalls, qDiff, f.answerIDByQuestion))
	require.Equal(t, answerIDDiff, f.updateAnswerCalls[0].ID)

	require.Len(t, f.updateResponseIDs, 1)
	require.Equal(t, respID, f.updateResponseIDs[0])
}

func countCreateForQuestion(calls []CreateAnswerParams, qid uuid.UUID) int {
	n := 0
	for _, c := range calls {
		if c.QuestionID == qid {
			n++
		}
	}
	return n
}

func countUpdateForQuestion(
	calls []UpdateAnswerParams,
	qid uuid.UUID,
	mapping map[uuid.UUID]uuid.UUID,
) int {
	wantID, ok := mapping[qid]
	if !ok {
		return 0
	}
	n := 0
	for _, c := range calls {
		if c.ID == wantID {
			n++
		}
	}
	return n
}

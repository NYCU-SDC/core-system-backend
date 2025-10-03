package submit

import (
	"context"
	"errors"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
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

type fakeQuestionStore struct {
	list []question.Answerable
	err  error
}

func (f *fakeQuestionStore) ListByFormID(ctx context.Context, formID uuid.UUID) ([]question.Answerable, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

type fakeResponseStore struct {
	called bool

	gotFormID  uuid.UUID
	gotUserID  uuid.UUID
	gotAnswers []shared.AnswerParam
	gotQTypes  []response.QuestionType
	returnResp response.FormResponse
	returnErr  error
}

func (f *fakeResponseStore) CreateOrUpdate(
	ctx context.Context,
	formID uuid.UUID,
	userID uuid.UUID,
	answers []shared.AnswerParam,
	qtypes []response.QuestionType,
) (response.FormResponse, error) {
	f.called = true
	f.gotFormID = formID
	f.gotUserID = userID
	f.gotAnswers = append([]shared.AnswerParam(nil), answers...)
	f.gotQTypes = append([]response.QuestionType(nil), qtypes...)
	return f.returnResp, f.returnErr
}

func ans(qID uuid.UUID, val string) shared.AnswerParam {
	return shared.AnswerParam{QuestionID: qID.String(), Value: val}
}

func q(questionID uuid.UUID, t question.QuestionType, validateErr error) question.Answerable {
	return fakeAnswerable{
		q: question.Question{
			ID:   questionID,
			Type: t,
		},
		validateErr: validateErr,
	}
}

func TestSubmit_AllValid_CallsCreateOrUpdate(t *testing.T) {
	formID := uuid.New()
	userID := uuid.New()

	qid1 := uuid.New()
	qid2 := uuid.New()

	qs := &fakeQuestionStore{
		list: []question.Answerable{
			q(qid1, question.QuestionTypeShortText, nil),
			q(qid2, question.QuestionTypeLongText, nil),
		},
	}
	rs := &fakeResponseStore{
		returnResp: response.FormResponse{ID: uuid.New(), FormID: formID, SubmittedBy: userID},
	}

	svc := NewService(zap.NewNop(), qs, rs)

	answers := []shared.AnswerParam{
		ans(qid1, "hello"),
		ans(qid2, "world"),
	}

	resp, errs := svc.Submit(context.Background(), formID, userID, answers)
	require.Nil(t, errs, "should not return validation errors")
	require.True(t, rs.called, "CreateOrUpdate should be called")
	require.Equal(t, formID, rs.gotFormID)
	require.Equal(t, userID, rs.gotUserID)
	require.Equal(t, answers, rs.gotAnswers)

	require.Equal(t,
		[]response.QuestionType{
			response.QuestionType(question.QuestionTypeShortText),
			response.QuestionType(question.QuestionTypeLongText),
		},
		rs.gotQTypes,
	)
	require.Equal(t, rs.returnResp.ID, resp.ID)
}

func TestSubmit_SomeInvalid_DoNotCallCreateOrUpdate(t *testing.T) {
	formID := uuid.New()
	userID := uuid.New()

	qid1 := uuid.New()
	qid2 := uuid.New()

	validationErr := errors.New("invalid")

	qs := &fakeQuestionStore{
		list: []question.Answerable{
			q(qid1, question.QuestionTypeShortText, nil),
			q(qid2, question.QuestionTypeLongText, validationErr), // question 2 validate error
		},
	}
	rs := &fakeResponseStore{}

	svc := NewService(zap.NewNop(), qs, rs)

	answers := []shared.AnswerParam{
		ans(qid1, "ok"),
		ans(qid2, "bad"),
	}

	resp, errs := svc.Submit(context.Background(), formID, userID, answers)
	require.NotNil(t, errs)
	require.Len(t, errs, 1)
	require.Zero(t, resp.ID)
	require.False(t, rs.called, "CreateOrUpdate should NOT be called when validation errors exist")
}

func TestSubmit_AnswerRefersUnknownQuestion_DoNotCallCreateOrUpdate(t *testing.T) {
	formID := uuid.New()
	userID := uuid.New()

	qid1 := uuid.New()
	unknownQ := uuid.New()

	qs := &fakeQuestionStore{
		list: []question.Answerable{
			q(qid1, question.QuestionTypeShortText, nil),
		},
	}
	rs := &fakeResponseStore{}

	svc := NewService(zap.NewNop(), qs, rs)

	answers := []shared.AnswerParam{
		ans(qid1, "ok"),
		ans(unknownQ, "should fail"),
	}

	resp, errs := svc.Submit(context.Background(), formID, userID, answers)
	require.NotNil(t, errs)
	require.Len(t, errs, 1)
	require.Zero(t, resp.ID)
	require.False(t, rs.called, "CreateOrUpdate should NOT be called when some QuestionID not found")
}

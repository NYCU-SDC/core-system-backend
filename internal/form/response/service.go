package response

import (
	"context"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Create(ctx context.Context, arg CreateParams) (Response, error)
	Get(ctx context.Context, arg GetParams) (Response, error)
	GetByFormIDAndSubmittedBy(ctx context.Context, arg GetByFormIDAndSubmittedByParams) (Response, error)
	Exists(ctx context.Context, arg ExistsParams) (bool, error)
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]Response, error)
	Update(ctx context.Context, id uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
	CreateAnswer(ctx context.Context, arg CreateAnswerParams) (Answer, error)
	GetAnswersByQuestionID(ctx context.Context, arg GetAnswersByQuestionIDParams) ([]GetAnswersByQuestionIDRow, error)
	GetAnswersByResponseID(ctx context.Context, responseID uuid.UUID) ([]Answer, error)
	DeleteAnswersByResponseID(ctx context.Context, responseID uuid.UUID) error
	UpdateAnswer(ctx context.Context, arg UpdateAnswerParams) (Answer, error)
	AnswerExists(ctx context.Context, arg AnswerExistsParams) (bool, error)
	CheckAnswerContent(ctx context.Context, arg CheckAnswerContentParams) (bool, error)
	GetAnswerID(ctx context.Context, arg GetAnswerIDParams) (uuid.UUID, error)
	GetQuestionByID(ctx context.Context, arg uuid.UUID) (Question, error)
}

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer
}

func NewService(logger *zap.Logger, db DBTX) *Service {
	return &Service{
		logger:  logger,
		queries: New(db),
		tracer:  otel.Tracer("response/service"),
	}
}

func (s *Service) Submit(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []Answer) (Response, error) {
	traceCtx, span := s.tracer.Start(ctx, "Submit")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exist, err := s.queries.Exists(traceCtx, ExistsParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check if response exists")
		span.RecordError(err)
		return Response{}, err
	}

	if exist {
		response, err := Update(s, traceCtx, formID, userID, answers)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "update response")
			span.RecordError(err)
			return Response{}, err
		}
		return response, nil
	}

	response, err := Create(s, traceCtx, formID, userID, answers)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create response")
		span.RecordError(err)
		return Response{}, err
	}
	return response, nil
}

// Create creates a new response and answers for a given form and user
func Create(s *Service, ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []Answer) (Response, error) {
	traceCtx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	newResponse, err := s.queries.Create(traceCtx, CreateParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create response")
		span.RecordError(err)
		return Response{}, err
	}

	for _, answer := range answers {
		question, err := s.queries.GetQuestionByID(traceCtx, answer.QuestionID)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "get question type")
			span.RecordError(err)
			return Response{}, err
		}
		_, err = s.queries.CreateAnswer(traceCtx, CreateAnswerParams{
			ResponseID: newResponse.ID,
			QuestionID: answer.QuestionID,
			Type:       question.Type,
			Value:      answer.Value,
		})
		if err != nil {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", newResponse.ID.String(), logger, "create answer")
			span.RecordError(err)
			return Response{}, err
		}
	}

	return newResponse, nil
}

func Update(s *Service, ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []Answer) (Response, error) {
	traceCtx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	response, err := GetByFormIDAndSubmittedBy(s, traceCtx, formID, userID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get response by form id and submitted by")
		span.RecordError(err)
		return Response{}, err
	}

	for _, answer := range answers {
		// check if answer exists
		answerExists, err := s.queries.AnswerExists(traceCtx, AnswerExistsParams{
			ResponseID: response.ID,
			QuestionID: answer.QuestionID,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "check if answer exists")
			span.RecordError(err)
			return Response{}, err
		}

		// if answer does not exist, create it
		if !answerExists {
			question, err := s.queries.GetQuestionByID(traceCtx, answer.QuestionID)
			if err != nil {
				err = databaseutil.WrapDBError(err, logger, "get question type")
				span.RecordError(err)
				return Response{}, err
			}
			_, err = s.queries.CreateAnswer(traceCtx, CreateAnswerParams{
				ResponseID: response.ID,
				QuestionID: answer.QuestionID,
				Type:       question.Type,
				Value:      answer.Value,
			})
			if err != nil {
				err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", response.ID.String(), logger, "create answer")
				span.RecordError(err)
				return Response{}, err
			}
		}

		// if answer exists, check if it is the same as the new answer
		sameAnswer, err := s.queries.CheckAnswerContent(traceCtx, CheckAnswerContentParams{
			ResponseID: response.ID,
			QuestionID: answer.QuestionID,
			Value:      answer.Value,
		})
		if err != nil {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "id", answer.ID.String(), logger, "check answer content")
			span.RecordError(err)
			return Response{}, err
		}

		// if answer is different, update it
		if !sameAnswer {
			answerID, err := s.queries.GetAnswerID(traceCtx, GetAnswerIDParams{
				ResponseID: response.ID,
				QuestionID: answer.QuestionID,
			})
			if err != nil {
				err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", response.ID.String(), logger, "get answer id")
				span.RecordError(err)
				return Response{}, err
			}
			_, err = s.queries.UpdateAnswer(traceCtx, UpdateAnswerParams{
				ID:    answerID,
				Value: answer.Value,
			})
			if err != nil {
				err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "id", answer.ID.String(), logger, "update answer")
				span.RecordError(err)
				return Response{}, err
			}
		}
	}

	// update the value of updated_at of response
	err = s.queries.Update(traceCtx, response.ID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", response.ID.String(), logger, "update response")
		span.RecordError(err)
		return Response{}, err
	}
	return response, nil
}

func GetByFormIDAndSubmittedBy(s *Service, ctx context.Context, formID uuid.UUID, userID uuid.UUID) (Response, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetByFormIDAndSubmittedBy")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	response, err := s.queries.GetByFormIDAndSubmittedBy(traceCtx, GetByFormIDAndSubmittedByParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get response by form id and submitted by")
		span.RecordError(err)
		return Response{}, err
	}

	return response, nil
}

// Get retrieves a response and answers by id
func (s *Service) Get(ctx context.Context, formID uuid.UUID, id uuid.UUID) (Response, []Answer, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	response, err := s.queries.Get(traceCtx, GetParams{
		ID:     id,
		FormID: formID,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get response by id")
		span.RecordError(err)
		return Response{}, []Answer{}, err
	}

	answers, err := s.queries.GetAnswersByResponseID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", response.ID.String(), logger, "get answers by response id")
		span.RecordError(err)
		return Response{}, []Answer{}, err
	}

	return response, answers, nil
}

// ListByFormID retrieves all responses for a given form
func (s *Service) ListByFormID(ctx context.Context, formID uuid.UUID) ([]Response, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListByFormID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	responses, err := s.queries.ListByFormID(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "form_id", formID.String(), logger, "list responses by form id")
		span.RecordError(err)
		return []Response{}, err
	}

	return responses, nil
}

func GetAnswersByResponseID(s *Service, ctx context.Context, responseID uuid.UUID) ([]Answer, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	answers, err := s.queries.GetAnswersByResponseID(traceCtx, responseID)

	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", responseID.String(), logger, "get answers by response id")
		span.RecordError(err)
		return []Answer{}, err
	}

	return answers, nil
}

// Delete deletes a response by id
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	answers, err := s.queries.GetAnswersByResponseID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", id.String(), logger, "get answers by response id")
		span.RecordError(err)
		return err
	}

	for _, answer := range answers {
		err = s.queries.DeleteAnswersByResponseID(traceCtx, answer.ResponseID)
		if err != nil {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", answer.ResponseID.String(), logger, "delete answer")
			span.RecordError(err)
			return err
		}
	}

	err = s.queries.Delete(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "delete response")
		span.RecordError(err)
		return err
	}

	return nil
}

// GetAnswersByQuestionID retrieves all answers for a given question
func (s *Service) GetAnswersByQuestionID(ctx context.Context, questionID uuid.UUID, formID uuid.UUID) ([]GetAnswersByQuestionIDRow, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetAnswersByQuestionID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	rows, err := s.queries.GetAnswersByQuestionID(traceCtx, GetAnswersByQuestionIDParams{
		QuestionID: questionID,
		FormID:     formID,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "question_id", questionID.String(), logger, "get answers by question id")
		span.RecordError(err)
		return []GetAnswersByQuestionIDRow{}, err
	}

	return rows, nil
}

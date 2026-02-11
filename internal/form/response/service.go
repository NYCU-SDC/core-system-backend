package response

import (
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
	"fmt"

	"NYCU-SDC/core-system-backend/internal"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Create(ctx context.Context, arg CreateParams) (FormResponse, error)
	Get(ctx context.Context, arg GetParams) (FormResponse, error)
	GetByFormIDAndSubmittedBy(ctx context.Context, arg GetByFormIDAndSubmittedByParams) (FormResponse, error)
	Exists(ctx context.Context, arg ExistsParams) (bool, error)
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error)
	Update(ctx context.Context, id uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
	CreateAnswer(ctx context.Context, arg CreateAnswerParams) (Answer, error)
	GetAnswersByQuestionID(ctx context.Context, arg GetAnswersByQuestionIDParams) ([]GetAnswersByQuestionIDRow, error)
	GetAnswersByResponseID(ctx context.Context, responseID uuid.UUID) ([]Answer, error)
	UpdateAnswer(ctx context.Context, arg UpdateAnswerParams) (Answer, error)
	AnswerExists(ctx context.Context, arg AnswerExistsParams) (bool, error)
	CheckAnswerContent(ctx context.Context, arg CheckAnswerContentParams) (bool, error)
	GetAnswerID(ctx context.Context, arg GetAnswerIDParams) (uuid.UUID, error)
	ListBySubmittedBy(ctx context.Context, submittedBy uuid.UUID) ([]FormResponse, error)
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

func (s Service) CreateOrUpdate(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam, questionType []QuestionType) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "CreateOrUpdate")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	if len(answers) != len(questionType) {
		err := fmt.Errorf("number of answers (%d) does not match number of question types (%d)", len(answers), len(questionType))
		logger.Error("Failed to create response", zap.Error(err), zap.String("formID", formID.String()), zap.String("userID", userID.String()))
		span.RecordError(err)
		return FormResponse{}, err
	}

	exists, err := s.queries.Exists(traceCtx, ExistsParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check if response exists")
		span.RecordError(err)
		return FormResponse{}, err
	}

	if exists {
		return s.Update(traceCtx, formID, userID, answers, questionType)
	} else {
		return s.Create(traceCtx, formID, userID, answers, questionType)
	}
}

// CreateEmpty creates an empty response (draft) for a given form and user
// Returns an error if the user already has a response for the form
func (s *Service) CreateEmpty(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "CreateEmpty")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)
	// Check if user already has a response for this form
	exists, err := s.queries.Exists(traceCtx, ExistsParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check if response exists")
		span.RecordError(err)
		return FormResponse{}, err
	}
	if exists {
		err = fmt.Errorf("user already has a response for this form")
		logger.Error("Failed to create empty response", zap.Error(err), zap.String("formID", formID.String()), zap.String("userID", userID.String()))
		span.RecordError(err)
		return FormResponse{}, internal.ErrResponseAlreadyExists
	}
	// Create empty response
	newResponse, err := s.queries.Create(traceCtx, CreateParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create empty response")
		span.RecordError(err)
		return FormResponse{}, err
	}
	return newResponse, nil
}

// Create creates a new response and answers for a given form and user
func (s Service) Create(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam, questionType []QuestionType) (FormResponse, error) {
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
		return FormResponse{}, err
	}

	for i, answer := range answers {
		questionID, err := internal.ParseUUID(answer.QuestionID)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "parse question id")
			span.RecordError(err)
			return FormResponse{}, err
		}

		_, err = s.queries.CreateAnswer(traceCtx, CreateAnswerParams{
			ResponseID: newResponse.ID,
			QuestionID: questionID,
			Type:       questionType[i],
			Value:      answer.Value,
		})
		if err != nil {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", newResponse.ID.String(), logger, "create answer")
			span.RecordError(err)
			return FormResponse{}, err
		}
	}

	return newResponse, nil
}

func (s Service) Update(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam, questionType []QuestionType) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	currentResponse, err := s.queries.GetByFormIDAndSubmittedBy(traceCtx, GetByFormIDAndSubmittedByParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get response by form id and submitted by")
		span.RecordError(err)
		return FormResponse{}, err
	}

	for i, answer := range answers {
		// check if answer exists
		questionID, err := internal.ParseUUID(answer.QuestionID)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "parse question id")
			span.RecordError(err)
			return FormResponse{}, err
		}
		answerExists, err := s.queries.AnswerExists(traceCtx, AnswerExistsParams{
			ResponseID: currentResponse.ID,
			QuestionID: questionID,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "check if answer exists")
			span.RecordError(err)
			return FormResponse{}, err
		}

		// if answer does not exist, create it
		if !answerExists {
			_, err = s.queries.CreateAnswer(traceCtx, CreateAnswerParams{
				ResponseID: currentResponse.ID,
				QuestionID: questionID,
				Type:       questionType[i],
				Value:      answer.Value,
			})
			if err != nil {
				err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", currentResponse.ID.String(), logger, "create answer")
				span.RecordError(err)
				return FormResponse{}, err
			}
		}

		// if answer exists, check if it is the same as the new answer
		sameAnswer, err := s.queries.CheckAnswerContent(traceCtx, CheckAnswerContentParams{
			ResponseID: currentResponse.ID,
			QuestionID: questionID,
			Value:      answer.Value,
		})
		if err != nil {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", currentResponse.ID.String(), logger, "check answer content")
			span.RecordError(err)
			return FormResponse{}, err
		}

		// if answer is different, update it
		if !sameAnswer {
			answerID, err := s.queries.GetAnswerID(traceCtx, GetAnswerIDParams{
				ResponseID: currentResponse.ID,
				QuestionID: questionID,
			})
			if err != nil {
				err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", currentResponse.ID.String(), logger, "get answer id")
				span.RecordError(err)
				return FormResponse{}, err
			}
			_, err = s.queries.UpdateAnswer(traceCtx, UpdateAnswerParams{
				ID:    answerID,
				Value: answer.Value,
			})
			if err != nil {
				err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "id", answerID.String(), logger, "update answer")
				span.RecordError(err)
				return FormResponse{}, err
			}
		}
	}

	// update the value of updated_at of response
	err = s.queries.Update(traceCtx, currentResponse.ID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", currentResponse.ID.String(), logger, "update response")
		span.RecordError(err)
		return FormResponse{}, err
	}
	return currentResponse, nil
}

// Get retrieves a response and answers by id
func (s Service) Get(ctx context.Context, formID uuid.UUID, id uuid.UUID) (FormResponse, []Answer, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	currentResponse, err := s.queries.Get(traceCtx, GetParams{
		ID:     id,
		FormID: formID,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get response by id")
		span.RecordError(err)
		return FormResponse{}, []Answer{}, err
	}

	answers, err := s.queries.GetAnswersByResponseID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", currentResponse.ID.String(), logger, "get answers by response id")
		span.RecordError(err)
		return FormResponse{}, []Answer{}, err
	}

	return currentResponse, answers, nil
}

// ListByFormID retrieves all responses for a given form
func (s Service) ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListByFormID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	responses, err := s.queries.ListByFormID(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "form_id", formID.String(), logger, "list responses by form id")
		span.RecordError(err)
		return []FormResponse{}, err
	}

	return responses, nil
}

// Delete deletes a response by id
func (s Service) Delete(ctx context.Context, id uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	err := s.queries.Delete(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "delete response")
		span.RecordError(err)
		return err
	}

	return nil
}

// GetAnswersByQuestionID retrieves all answers for a given question
func (s Service) GetAnswersByQuestionID(ctx context.Context, questionID uuid.UUID, formID uuid.UUID) ([]GetAnswersByQuestionIDRow, error) {
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

func (s *Service) ListBySubmittedBy(ctx context.Context, userID uuid.UUID) ([]FormResponse, error) {
	ctx, span := s.tracer.Start(ctx, "ListBySubmittedBy")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	responses, err := s.queries.ListBySubmittedBy(ctx, userID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list responses by submitted by")
		span.RecordError(err)
		return nil, err
	}

	return responses, nil
}

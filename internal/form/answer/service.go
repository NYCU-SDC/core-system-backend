package answer

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
	"encoding/json"
	"fmt"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	ListByResponseID(ctx context.Context, responseID uuid.UUID) ([]Answer, error)
	GetByResponseIDAndQuestionID(ctx context.Context, arg GetByResponseIDAndQuestionIDParams) (Answer, error)
	BatchUpsert(ctx context.Context, arg BatchUpsertParams) ([]Answer, error)
}

type QuestionStore interface {
	ListSectionsWithAnswersByFormID(ctx context.Context, formID uuid.UUID) ([]question.SectionWithAnswerableList, error)
	GetAnswerableMapByFormID(ctx context.Context, formID uuid.UUID) (map[string]question.Answerable, error)
}

type Answerable interface {
	Question() question.Question

	Validate(rawValue json.RawMessage) error

	DisplayValue(rawValue json.RawMessage) (string, error)

	// DecodeRequest decodes the raw JSON value from the request into the appropriate Go type based on the question type.
	DecodeRequest(rawValue json.RawMessage) (any, error)

	// DecodeStorage decodes the raw JSON value from the database into the appropriate Go type based on the question type.
	DecodeStorage(rawValue json.RawMessage) (any, error)

	// EncodeRequest encodes the Go value into raw JSON for storage in the database or for sending in a response, based on the question type.
	EncodeRequest(answer any) (json.RawMessage, error)
}

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer

	questionStore QuestionStore
}

func NewService(logger *zap.Logger, db DBTX, questionStore QuestionStore) *Service {
	return &Service{
		logger:        logger,
		queries:       New(db),
		tracer:        otel.Tracer("answer/service"),
		questionStore: questionStore,
	}
}

func (s Service) List(ctx context.Context, formID, responseID uuid.UUID) ([]Answer, []question.Answerable, map[string]question.Answerable, error) {
	traceCtx, span := s.tracer.Start(ctx, "List")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	answers, err := s.queries.ListByResponseID(traceCtx, responseID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list answers")
		span.RecordError(err)
		return nil, nil, nil, fmt.Errorf("failed to list answers: %w", err)
	}

	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get answerable map for form ID %s: %w", formID, err)
	}

	transformedAnswers := make([]Answer, 0, len(answers))
	answerableList := make([]question.Answerable, 0, len(answers))
	for _, answer := range answers {
		transformedAnswer, answerable, err := s.transformAnswerForResponse(traceCtx, answer, answerableMap, formID)
		if err != nil {
			logger.Error("failed to transform answer", zap.String("questionID", answer.QuestionID.String()), zap.Error(err))
			span.RecordError(err)
			return nil, nil, nil, err
		}
		transformedAnswers = append(transformedAnswers, transformedAnswer)
		answerableList = append(answerableList, answerable)
	}

	logger.Info("successfully listed answers", zap.Int("count", len(transformedAnswers)), zap.String("responseID", responseID.String()))
	return transformedAnswers, answerableList, answerableMap, nil
}

func (s Service) Get(ctx context.Context, formID, responseID, questionID uuid.UUID) (Answer, Answerable, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	answer, err := s.queries.GetByResponseIDAndQuestionID(traceCtx, GetByResponseIDAndQuestionIDParams{
		ResponseID: responseID,
		QuestionID: questionID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get answer by response ID and question ID")
		span.RecordError(err)
		return Answer{}, nil, fmt.Errorf("failed to get answer for response ID %s and question ID %s: %w", responseID, questionID, err)
	}

	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		return Answer{}, nil, fmt.Errorf("failed to get answerable map for form ID %s: %w", formID, err)
	}

	transformedAnswer, answerable, err := s.transformAnswerForResponse(traceCtx, answer, answerableMap, formID)
	if err != nil {
		span.RecordError(err)
		return Answer{}, nil, err
	}

	logger.Info("successfully retrieved answer", zap.String("responseID", responseID.String()), zap.String("questionID", questionID.String()))

	return transformedAnswer, answerable, nil
}

// transformAnswerForResponse transforms an answer from storage format to response format
func (s Service) transformAnswerForResponse(ctx context.Context, answer Answer, answerableMap map[string]question.Answerable, formID uuid.UUID) (Answer, question.Answerable, error) {
	_, span := s.tracer.Start(ctx, "transformAnswerForResponse")
	defer span.End()

	questionID := answer.QuestionID

	answerable, found := answerableMap[questionID.String()]
	if !found {
		return Answer{}, nil, fmt.Errorf("question with ID %s not found in form %s", questionID, formID)
	}

	return answer, answerable, nil
}

// Upsert validates and upserts answers for a given form response. It returns the upserted answers and any validation errors that occurred during the process.
func (s Service) Upsert(ctx context.Context, formID, responseID uuid.UUID, answers []shared.AnswerParam) ([]Answer, []Answerable, []error) {
	answerQuestionPairs := make([]struct {
		AnswerParam shared.AnswerParam
		Answerable  Answerable
	}, 0, len(answers))

	traceCtx, span := s.tracer.Start(ctx, "Upsert")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		return nil, nil, []error{err}
	}

	validationErrors := make([]error, 0)
	answeredQuestionIDs := make(map[string]bool)

	for _, ans := range answers {
		answerable, found := answerableMap[ans.QuestionID]
		if !found {
			validationErrors = append(validationErrors, fmt.Errorf("question with ID %s not found in form %s", ans.QuestionID, formID))
			continue
		}

		answeredQuestionIDs[ans.QuestionID] = true

		// Validate answer value (convert string to json.RawMessage)
		err := answerable.Validate(ans.Value)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("validation error for question ID %s: %w", ans.QuestionID, err))
		}

		answerQuestionPairs = append(answerQuestionPairs, struct {
			AnswerParam shared.AnswerParam
			Answerable  Answerable
		}{
			AnswerParam: ans,
			Answerable:  answerable,
		})
	}

	if len(validationErrors) > 0 {
		logger.Error("validation errors occurred", zap.Error(fmt.Errorf("validation errors occurred")), zap.Any("errors", validationErrors))
		span.RecordError(fmt.Errorf("validation errors occurred"))
		validationErrors = append([]error{internal.ErrValidationFailed}, validationErrors...)
		return nil, nil, validationErrors
	}

	// Prepare batch upsert parameters
	responseIDs := make([]uuid.UUID, len(answers))
	questionIDs := make([]uuid.UUID, len(answers))
	values := make([][]byte, len(answers))

	for i, pair := range answerQuestionPairs {
		responseIDs[i] = responseID

		questionID, err := uuid.Parse(pair.AnswerParam.QuestionID)
		if err != nil {
			logger.Error("invalid question ID format", zap.String("questionID", pair.AnswerParam.QuestionID), zap.Error(err))
			span.RecordError(fmt.Errorf("invalid question ID format for question ID %s: %w", pair.AnswerParam.QuestionID, err))
			return nil, nil, []error{fmt.Errorf("invalid question ID format for question ID %s: %w", pair.AnswerParam.QuestionID, err)}
		}

		questionIDs[i] = questionID

		encodedValue, err := pair.Answerable.DecodeRequest(pair.AnswerParam.Value)
		if err != nil {
			logger.Error("failed to encode answer value for storage", zap.String("questionID", pair.AnswerParam.QuestionID), zap.Error(err))
			span.RecordError(fmt.Errorf("failed to encode answer value for question ID %s: %w", pair.AnswerParam.QuestionID, err))
			return nil, nil, []error{fmt.Errorf("failed to encode answer value for question ID %s: %w", pair.AnswerParam.QuestionID, err)}
		}

		jsonValue, err := json.Marshal(encodedValue)
		if err != nil {
			logger.Error("failed to marshal encoded answer value to JSON", zap.String("questionID", pair.AnswerParam.QuestionID), zap.Error(err))
			span.RecordError(fmt.Errorf("failed to marshal encoded answer value to JSON for question ID %s: %w", pair.AnswerParam.QuestionID, err))
			return nil, nil, []error{fmt.Errorf("failed to marshal encoded answer value to JSON for question ID %s: %w", pair.AnswerParam.QuestionID, err)}
		}

		values[i] = jsonValue
	}

	// Batch upsert answers into database
	upsertedAnswers, err := s.queries.BatchUpsert(traceCtx, BatchUpsertParams{
		ResponseIds: responseIDs,
		QuestionIds: questionIDs,
		Values:      values,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "batch upsert answers")
		span.RecordError(err)
		return nil, nil, []error{fmt.Errorf("failed to save answers: %w", err)}
	}

	transformedAnswers := make([]Answer, 0, len(answers))
	answerableList := make([]Answerable, 0, len(answers))
	for _, answer := range upsertedAnswers {
		transformedAnswer, answerable, err := s.transformAnswerForResponse(traceCtx, answer, answerableMap, formID)
		if err != nil {
			logger.Error("failed to transform answer", zap.String("questionID", answer.QuestionID.String()), zap.Error(err))
			span.RecordError(err)
			return nil, nil, []error{err}
		}
		transformedAnswers = append(transformedAnswers, transformedAnswer)
		answerableList = append(answerableList, answerable)
	}

	logger.Info("successfully upserted answers", zap.Int("count", len(upsertedAnswers)))
	return transformedAnswers, answerableList, nil
}

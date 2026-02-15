package answer

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
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
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]question.SectionWithQuestions, error)
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

func (s Service) List(ctx context.Context, responseID uuid.UUID) ([]Answer, error) {
	traceCtx, span := s.tracer.Start(ctx, "List")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	answers, err := s.queries.ListByResponseID(traceCtx, responseID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list answers")
		span.RecordError(err)
		return nil, fmt.Errorf("failed to list answers: %w", err)
	}

	logger.Info("successfully listed answers", zap.Int("count", len(answers)), zap.String("responseID", responseID.String()))
	return answers, nil
}

func (s Service) Get(ctx context.Context, responseID, questionID uuid.UUID) (Answer, error) {
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
		return Answer{}, fmt.Errorf("failed to list answers for Get: %w", err)
	}

	return answer, nil
}

// Upsert validates and upserts answers for a given form response. It returns the upserted answers and any validation errors that occurred during the process.
func (s Service) Upsert(ctx context.Context, formID, responseID uuid.UUID, answers []shared.AnswerParam) ([]Answer, []error) {
	traceCtx, span := s.tracer.Start(ctx, "Upsert")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	list, err := s.questionStore.ListByFormID(traceCtx, formID)
	if err != nil {
		return []Answer{}, []error{err}
	}

	var questionTypes []response.QuestionType
	validationErrors := make([]error, 0)
	answeredQuestionIDs := make(map[string]bool)

	for _, ans := range answers {
		var found bool
		for _, section := range list {
			for _, q := range section.Questions {
				if q.Question().ID.String() == ans.QuestionID {
					found = true
					answeredQuestionIDs[ans.QuestionID] = true

					// Validate answer value
					err := q.Validate(ans.Value)
					if err != nil {
						validationErrors = append(validationErrors, fmt.Errorf("validation error for question ID %s: %w", ans.QuestionID, err))
					}

					questionTypes = append(questionTypes, response.QuestionType(q.Question().Type))

					break
				}
			}
		}

		if !found {
			validationErrors = append(validationErrors, fmt.Errorf("question with ID %s not found in form %s", ans.QuestionID, formID))
		}
	}

	if len(validationErrors) > 0 {
		logger.Error("validation errors occurred", zap.Error(fmt.Errorf("validation errors occurred")), zap.Any("errors", validationErrors))
		span.RecordError(fmt.Errorf("validation errors occurred"))
		validationErrors = append([]error{internal.ErrValidationFailed}, validationErrors...)
		return []Answer{}, validationErrors
	}

	// Prepare batch upsert parameters
	responseIDs := make([]uuid.UUID, len(answers))
	questionIDs := make([]uuid.UUID, len(answers))
	values := make([][]byte, len(answers))

	for i, ans := range answers {
		questionID, err := uuid.Parse(ans.QuestionID)
		if err != nil {
			logger.Error("failed to parse question ID", zap.Error(err), zap.String("questionID", ans.QuestionID))
			span.RecordError(err)
			return []Answer{}, []error{fmt.Errorf("invalid question ID %s: %w", ans.QuestionID, err)}
		}

		responseIDs[i] = responseID
		questionIDs[i] = questionID
		values[i] = []byte(ans.Value)
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
		return []Answer{}, []error{fmt.Errorf("failed to save answers: %w", err)}
	}

	logger.Info("successfully upserted answers", zap.Int("count", len(upsertedAnswers)))
	return upsertedAnswers, nil
}

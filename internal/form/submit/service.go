package submit

import (
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
	"fmt"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type QuestionStore interface {
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]question.Answerable, error)
}

type FormResponseStore interface {
	CreateOrUpdate(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam, questionType []response.QuestionType) (response.FormResponse, error)
}

type Service struct {
	logger *zap.Logger
	tracer trace.Tracer

	questionStore QuestionStore
	responseStore FormResponseStore
}

func NewService(logger *zap.Logger, questionStore QuestionStore, formResponseStore FormResponseStore) *Service {
	return &Service{
		logger:        logger,
		tracer:        otel.Tracer("submit/service"),
		questionStore: questionStore,
		responseStore: formResponseStore,
	}
}

// Submit handles a user's submission for a specific form.
// It performs the following steps:
// 1. Retrieves all questions associated with the form.
// 2. Validates the submitted answers against the corresponding questions.
//   - If any validation fails or if an answer references a nonexistent question, it accumulates the errors.
//
// 3. If there are validation errors, returns them without saving.
// 4. If validation passes, creates or updates the response record using the answer values and question types.
//
// Returns the saved form response if successful, or a list of validation/database errors otherwise.
func (s *Service) Submit(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam) (response.FormResponse, []error) {
	traceCtx, span := s.tracer.Start(ctx, "Submit")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	questions, err := s.questionStore.ListByFormID(traceCtx, formID)
	if err != nil {
		return response.FormResponse{}, []error{err}
	}

	// Validate answers against questions
	questionTypes := make([]response.QuestionType, len(questions))
	validationErrors := make([]error, 0)
	for _, ans := range answers {
		var found bool
		for _, q := range questions {
			if q.Question().ID.String() == ans.QuestionID {
				found = true
				err := q.Validate(ans.Value)
				if err != nil {
					validationErrors = append(validationErrors, err)
				}

				questionTypes = append(questionTypes, response.QuestionType(q.Question().Type))

				break
			}
		}

		if !found {
			err := fmt.Errorf("question with ID %s not found in form %s", ans.QuestionID, formID)
			validationErrors = append(validationErrors, err)
		}
	}

	if len(validationErrors) > 0 {
		logger.Error("validation errors occurred", zap.Error(fmt.Errorf("validation errors occurred")), zap.Any("errors", validationErrors))
		span.RecordError(fmt.Errorf("validation errors occurred"))
		return response.FormResponse{}, validationErrors
	}

	result, err := s.responseStore.CreateOrUpdate(traceCtx, formID, userID, answers, questionTypes)
	if err != nil {
		logger.Error("failed to create or update form response", zap.Error(err), zap.String("formID", formID.String()), zap.String("userID", userID.String()))
		span.RecordError(err)
		return response.FormResponse{}, []error{err}
	}

	return result, nil
}

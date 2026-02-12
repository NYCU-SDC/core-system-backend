package submit

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
	"fmt"
	"time"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type QuestionStore interface {
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]question.SectionWithQuestions, error)
}

type FormStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (form.GetByIDRow, error)
}

type FormResponseStore interface {
	CreateOrUpdate(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam, questionType []response.QuestionType) (response.FormResponse, error)
}

type Service struct {
	logger *zap.Logger
	tracer trace.Tracer

	formStore     FormStore
	questionStore QuestionStore
	responseStore FormResponseStore
}

func NewService(logger *zap.Logger, formStore FormStore, questionStore QuestionStore, formResponseStore FormResponseStore) *Service {
	return &Service{
		logger:        logger,
		tracer:        otel.Tracer("submit/service"),
		formStore:     formStore,
		questionStore: questionStore,
		responseStore: formResponseStore,
	}
}

// Submit handles a user's submission for a specific form.
// It performs the following steps:
// 1. Retrieves all questions associated with the form.
// 2. Validates the submitted answers against the corresponding questions.
//   - If any validation fails or if an answer references a nonexistent question, it accumulates the errors.
//   - Validates that all required questions have been answered.
//
// 3. If there are validation errors, returns them without saving.
// 4. If validation passes, creates or updates the response record using the answer values and question types.
//
// Returns the saved form response if successful, or a list of validation/database errors otherwise.
func (s *Service) Submit(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam) (response.FormResponse, []error) {
	traceCtx, span := s.tracer.Start(ctx, "Submit")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Check form deadline before processing submission
	formDetails, err := s.formStore.GetByID(traceCtx, formID)
	if err != nil {
		return response.FormResponse{}, []error{err}
	}

	// Validate form deadline
	if formDetails.Deadline.Valid && formDetails.Deadline.Time.Before(time.Now()) {
		return response.FormResponse{}, []error{internal.ErrFormDeadlinePassed}
	}

	list, err := s.questionStore.ListByFormID(traceCtx, formID)
	if err != nil {
		return response.FormResponse{}, []error{err}
	}

	// Validate answers against questions
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

	// Check for required questions that were not answered
	for _, section := range list {
		for _, q := range section.Questions {
			if q.Question().Required && !answeredQuestionIDs[q.Question().ID.String()] {
				validationErrors = append(validationErrors, fmt.Errorf("question ID %s is required but not answered", q.Question().ID.String()))
			}
		}
	}

	if len(validationErrors) > 0 {
		logger.Error("validation errors occurred", zap.Error(fmt.Errorf("validation errors occurred")), zap.Any("errors", validationErrors))
		span.RecordError(fmt.Errorf("validation errors occurred"))
		validationErrors = append([]error{internal.ErrValidationFailed}, validationErrors...)
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

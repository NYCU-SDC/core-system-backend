package submit

import (
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type QuestionStore interface {
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]question.SectionWithAnswerableList, error)
}

type FormStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (form.GetByIDRow, error)
}

type FormResponseStore interface {
	Create(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (response.FormResponse, error)
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
	//traceCtx, span := s.tracer.Start(ctx, "Submit")
	//defer span.End()
	//logger := logutil.WithContext(traceCtx, s.logger)
	//
	//// Check form deadline before processing submission
	//formDetails, err := s.formStore.GetByID(traceCtx, formID)
	//if err != nil {
	//	return response.FormResponse{}, []error{err}
	//}
	//
	//// Validate form deadline
	//if formDetails.Deadline.Valid && formDetails.Deadline.Time.Before(time.Now()) {
	//	return response.FormResponse{}, []error{internal.ErrFormDeadlinePassed}
	//}

	//result, err := s.responseStore.CreateOrUpdate(traceCtx, formID, userID, answers, questionTypes)
	//if err != nil {
	//	logger.Error("failed to create or update form response", zap.Error(err), zap.String("formID", formID.String()), zap.String("userID", userID.String()))
	//	span.RecordError(err)
	//	return response.FormResponse{}, []error{err}
	//}
	//
	//return result, nil
	return response.FormResponse{}, []error{}

}

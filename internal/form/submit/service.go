package submit

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/form/answer"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type AnswerStore interface {
	Upsert(ctx context.Context, formID, responseID uuid.UUID, answers []shared.AnswerParam) ([]answer.Answer, []answer.Answerable, []error)
}

type QuestionStore interface {
	ListSectionsWithAnswersByFormID(ctx context.Context, formID uuid.UUID) ([]question.SectionWithAnswerableList, error)
}

type FormStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (form.GetByIDRow, error)
}

type FormResponseStore interface {
	Create(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (response.FormResponse, error)
	Get(ctx context.Context, id uuid.UUID) (response.FormResponse, []response.SectionWithAnswerableAndAnswer, error)
	GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	UpdateSubmitted(ctx context.Context, id uuid.UUID) (response.FormResponse, error)
}

type Service struct {
	logger *zap.Logger
	tracer trace.Tracer

	formStore     FormStore
	questionStore QuestionStore
	responseStore FormResponseStore
	answerStore   AnswerStore
}

func NewService(logger *zap.Logger, formStore FormStore, questionStore QuestionStore, formResponseStore FormResponseStore, answerStore AnswerStore) *Service {
	return &Service{
		logger:        logger,
		tracer:        otel.Tracer("submit/service"),
		formStore:     formStore,
		questionStore: questionStore,
		responseStore: formResponseStore,
		answerStore:   answerStore,
	}
}

// Submit updates answers for a response, validates all sections are complete, and marks the response as submitted
// Returns an error if any section is not completed or skipped
func (s *Service) Submit(ctx context.Context, responseID uuid.UUID, answers []shared.AnswerParam) (response.FormResponse, []error) {
	traceCtx, span := s.tracer.Start(ctx, "Submit")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Get the form ID associated with this response
	formID, err := s.responseStore.GetFormIDByID(traceCtx, responseID)
	if err != nil {
		logger.Error("failed to get form id by response id", zap.Error(err))
		return response.FormResponse{}, []error{err}
	}

	// Upsert all provided answers
	_, _, errs := s.answerStore.Upsert(traceCtx, formID, responseID, answers)
	if len(errs) > 0 {
		logger.Error("failed to upsert answers", zap.Int("numErrors", len(errs)))
		return response.FormResponse{}, errs
	}

	// Get the updated response with all sections to validate completion
	_, sections, err := s.responseStore.Get(traceCtx, responseID)
	if err != nil {
		logger.Error("failed to get response after upsert", zap.Error(err))
		return response.FormResponse{}, []error{err}
	}

	// Collect all incomplete sections (not completed or skipped)
	notCompleteSections := make([]struct {
		Title    string
		ID       uuid.UUID
		Progress string
	}, 0, len(sections))
	for _, section := range sections {
		currentProgress := section.SectionProgress
		if currentProgress != response.SectionProgressCompleted && currentProgress != response.SectionProgressSkipped {
			notCompleteSections = append(notCompleteSections, struct {
				Title    string
				ID       uuid.UUID
				Progress string
			}{
				Title:    section.Section.Title.String,
				ID:       section.Section.ID,
				Progress: string(currentProgress),
			})

			logger.Warn("section not complete after submit", zap.String("sectionID", section.Section.ID.String()), zap.String("progress", string(currentProgress)))
		}
	}

	// Return error if any sections are incomplete
	if len(notCompleteSections) > 0 {
		logger.Warn("response not complete after submit", zap.Int("numNotCompleteSections", len(notCompleteSections)))
		return response.FormResponse{}, []error{internal.ErrResponseNotComplete{NotCompleteSections: notCompleteSections}}
	}

	// Mark the response as submitted
	formResponse, err := s.responseStore.UpdateSubmitted(ctx, responseID)
	if err != nil {
		logger.Error("failed to update response to submitted", zap.Error(err))
		return response.FormResponse{}, []error{err}
	}

	return formResponse, nil
}

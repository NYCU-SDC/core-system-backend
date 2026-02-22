package submit

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/form/answer"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
	"errors"
	"slices"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	ListByUnit(ctx context.Context, arg form.ListByUnitParams) ([]form.ListByUnitRow, error)
}

type FormResponseStore interface {
	Create(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (response.FormResponse, error)
	Get(ctx context.Context, id uuid.UUID, formID uuid.UUID) (response.FormResponse, []response.SectionWithAnswerableAndAnswer, error)
	GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	UpdateSubmitted(ctx context.Context, id uuid.UUID) (response.FormResponse, error)
	ListBySubmittedBy(ctx context.Context, submittedBy uuid.UUID) ([]response.FormResponse, error)
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
		if errors.Is(err, pgx.ErrNoRows) {
			return response.FormResponse{}, []error{internal.ErrResponseNotFound}
		}
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
	_, sections, err := s.responseStore.Get(traceCtx, responseID, formID)
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

func (s *Service) ListFormsOfUser(ctx context.Context, unitIDs []uuid.UUID, userID uuid.UUID) ([]form.UserForm, error) {
	ctx, span := s.tracer.Start(ctx, "ListFormsOfUser")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	responses, err := s.responseStore.ListBySubmittedBy(ctx, userID)
	if err != nil {
		span.RecordError(err)
		return []form.UserForm{}, err
	}

	formStatusMap := make(map[uuid.UUID]form.UserFormStatus)
	for _, currentResponse := range responses {
		status := form.UserFormStatusInProgress
		if currentResponse.SubmittedAt.Valid {
			status = form.UserFormStatusCompleted
		}
		formStatusMap[currentResponse.FormID] = status
	}

	allForms := make(map[uuid.UUID]form.ListByUnitRow)
	for _, unitID := range unitIDs {
		forms, err := s.formStore.ListByUnit(ctx, form.ListByUnitParams{
			UnitID: pgtype.UUID{Bytes: unitID, Valid: true},
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "list forms by unit")
			span.RecordError(err)
			return []form.UserForm{}, err
		}

		for _, currentForm := range forms {
			allForms[currentForm.ID] = currentForm
		}
	}

	userForms := make([]form.UserForm, 0, len(allForms))
	for formID, row := range allForms {
		status, exists := formStatusMap[formID]
		if !exists {
			status = form.UserFormStatusNotStarted
		}

		userForms = append(userForms, form.UserForm{
			FormID:   formID,
			Title:    row.Title,
			Deadline: row.Deadline,
			Status:   status,
		})
	}

	slices.SortFunc(userForms, func(a, b form.UserForm) int {

		if a.Deadline.Valid != b.Deadline.Valid {
			if a.Deadline.Valid {
				return -1
			}
			return 1
		}

		if a.Deadline.Valid {
			if n := a.Deadline.Time.Compare(b.Deadline.Time); n != 0 {
				return n
			}
		}

		if a.Title < b.Title {
			return -1
		}
		if a.Title > b.Title {
			return 1
		}

		return 0
	})

	return userForms, nil
}

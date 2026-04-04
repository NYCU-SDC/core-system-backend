package response

import (
	"context"
	"fmt"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/answer"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"errors"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Get(ctx context.Context, arg GetParams) (FormResponse, error)
	GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	Create(ctx context.Context, arg CreateParams) (FormResponse, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	ExistsByFormIDAndSubmittedBy(ctx context.Context, arg ExistsByFormIDAndSubmittedByParams) (bool, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListByFormIDAndSubmittedBy(ctx context.Context, arg ListByFormIDAndSubmittedByParams) ([]FormResponse, error)
	ListBySubmittedBy(ctx context.Context, userID uuid.UUID) ([]FormResponse, error)
	UpdateSubmitted(ctx context.Context, id uuid.UUID) (FormResponse, error)
}

type WorkflowResolver interface {
	ResolveSections(ctx context.Context, formID uuid.UUID, answers []answer.Answer, answerableMap map[string]question.Answerable) ([]uuid.UUID, error)
}

type AnswerStore interface {
	List(ctx context.Context, formID, responseID uuid.UUID) ([]answer.Answer, []question.Answerable, map[string]question.Answerable, error)
}

type UserStore interface {
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
}
type SectionWithQuestionStore interface {
	ListSections(ctx context.Context, formID uuid.UUID) (map[string]question.Section, error)
	ListSectionsWithAnswersByFormID(ctx context.Context, formID uuid.UUID) ([]question.SectionWithAnswerableList, error)
}

type SectionWithAnswerableAndAnswer struct {
	Section         question.Section
	SectionProgress SectionProgress

	Answerable []question.Answerable
	Answer     []answer.Answer
}

type FormStore interface {
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer

	answerStore              AnswerStore
	sectionWithQuestionStore SectionWithQuestionStore
	workflowResolver         WorkflowResolver
	formStore                FormStore
	userStore                UserStore
}

func NewService(logger *zap.Logger, db DBTX, answerStore AnswerStore, sectionStore SectionWithQuestionStore, workflowResolver WorkflowResolver, formStore FormStore, userStore UserStore) *Service {
	return &Service{
		logger:  logger,
		queries: New(db),
		tracer:  otel.Tracer("response/service"),

		answerStore:              answerStore,
		sectionWithQuestionStore: sectionStore,
		workflowResolver:         workflowResolver,
		formStore:                formStore,
		userStore:                userStore,
	}
}

// Create creates an empty response (draft) for a given form and user
// Returns an error if the user already has a response for the form
func (s Service) Create(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	formExists, err := s.formStore.Exists(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check form exists")
		span.RecordError(err)
		return FormResponse{}, err
	}
	if !formExists {
		return FormResponse{}, internal.ErrFormNotFound
	}

	// Check if user already has a response for this form
	exists, err := s.queries.ExistsByFormIDAndSubmittedBy(traceCtx, ExistsByFormIDAndSubmittedByParams{
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

// ListByFormIDAndSubmittedBy retrieves all responses submitted by a given user
func (s Service) ListByFormIDAndSubmittedBy(ctx context.Context, formID uuid.UUID, userID uuid.UUID) ([]FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListByFormIDAndSubmittedBy")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exists, err := s.userStore.ExistsByID(traceCtx, userID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check user exists")
		span.RecordError(err)
		return nil, err
	}
	if !exists {
		return nil, internal.ErrUserNotFound
	}

	exists, err = s.formStore.Exists(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check form exists")
		span.RecordError(err)
		return nil, err
	}
	if !exists {
		return nil, internal.ErrFormNotFound
	}

	responses, err := s.queries.ListByFormIDAndSubmittedBy(traceCtx, ListByFormIDAndSubmittedByParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list responses by submitted by")
		span.RecordError(err)
		return nil, err
	}

	return responses, nil
}

func (s Service) ListBySubmittedBy(ctx context.Context, userID uuid.UUID) ([]FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListBySubmittedBy")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exists, err := s.userStore.ExistsByID(traceCtx, userID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check user exists")
		span.RecordError(err)
		return nil, err
	}
	if !exists {
		return nil, internal.ErrUserNotFound
	}

	responses, err := s.queries.ListBySubmittedBy(traceCtx, userID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list responses by submitted by")
		span.RecordError(err)
		return nil, err
	}

	return responses, nil
}

// Get retrieves a form response by ID along with its sections, questions, and answers
// The sections are returned in workflow order (active sections first, then skipped sections)
func (s Service) Get(ctx context.Context, id uuid.UUID, formID uuid.UUID) (FormResponse, []SectionWithAnswerableAndAnswer, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Get the form response
	response, err := s.queries.Get(traceCtx, GetParams{
		ID:     id,
		FormID: formID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FormResponse{}, nil, internal.ErrResponseNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get response by id")
		span.RecordError(err)
		return FormResponse{}, nil, err
	}

	// Get all answers for this response.
	// answerableMap contains Ranking questions that have already had their
	// dynamic choices resolved (via resolveRankingChoices inside List).
	answerPayload, answerable, answerableMap, err := s.answerStore.List(traceCtx, response.FormID, response.ID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", response.ID.String(), logger, "list answers by response id")
		span.RecordError(err)
		return FormResponse{}, nil, err
	}

	// Build answer payload map for quick lookup by question ID
	answerPayloadMap := make(map[string]struct {
		Answer     answer.Answer
		Answerable question.Answerable
	})
	for i := range answerPayload {
		answerPayloadMap[answerPayload[i].QuestionID.String()] = struct {
			Answer     answer.Answer
			Answerable question.Answerable
		}{
			Answer:     answerPayload[i],
			Answerable: answerable[i],
		}
	}

	// Resolve workflow sections for the response
	sectionIDs, sectionActiveMap, err := s.ResolveWorkflowSectionsForResponse(
		traceCtx, response.FormID, answerPayload, answerableMap, response.ID,
	)
	if err != nil {
		span.RecordError(err)
		return FormResponse{}, nil, err
	}

	// Get all sections with their questions
	sectionWithQuestion, err := s.sectionWithQuestionStore.ListSectionsWithAnswersByFormID(traceCtx, response.FormID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "section", "form_id", response.FormID.String(), logger, "list sections with questions by form id")
		span.RecordError(err)
		return FormResponse{}, nil, err
	}

	// Build section map for quick lookup by section ID
	sectionMap := make(map[string]question.SectionWithAnswerableList)
	for _, swq := range sectionWithQuestion {
		sectionMap[swq.Section.ID.String()] = swq
	}

	// Build result list with sections ordered by workflow (active sections first, then skipped)
	var result []SectionWithAnswerableAndAnswer

	// First, add active sections in the order returned by workflow resolver
	for _, sectionID := range sectionIDs {
		sectionWithAnswerableList, exists := sectionMap[sectionID.String()]
		if !exists {
			// This shouldn't happen - workflow returned a section ID that doesn't exist
			logger.DPanic("Section from workflow not found in section list", zap.String("sectionID", sectionID.String()))
			continue
		}

		// Collect all answerables and corresponding answers for this section.
		// Prefer the resolved answerable from answerableMap (which has dynamic
		// choices injected for Ranking questions) over the raw version from the
		// section store.
		var sectionAnswers []answer.Answer
		var sectionAnswerables []question.Answerable

		for _, ans := range sectionWithAnswerableList.AnswerableList {
			questionID := ans.Question().ID.String()

			// Use the resolved answerable if available; fall back to the raw one.
			resolved, hasResolved := answerableMap[questionID]
			if hasResolved {
				sectionAnswerables = append(sectionAnswerables, resolved)
			} else {
				sectionAnswerables = append(sectionAnswerables, ans)
			}

			// Add answer if it exists for this question
			if answerData, hasAnswer := answerPayloadMap[questionID]; hasAnswer {
				sectionAnswers = append(sectionAnswers, answerData.Answer)
			}
		}

		// Calculate section progress based on answers and required questions
		progress := calculateSectionProgress(sectionWithAnswerableList.AnswerableList, answerPayloadMap)

		result = append(result, SectionWithAnswerableAndAnswer{
			Section:         sectionWithAnswerableList.Section,
			SectionProgress: progress,
			Answerable:      sectionAnswerables,
			Answer:          sectionAnswers,
		})
	}

	// Then, add skipped sections (those not in active map)
	// Note: Answers are preserved even for skipped sections
	for _, swq := range sectionWithQuestion {
		sectionIDStr := swq.Section.ID.String()
		if !sectionActiveMap[sectionIDStr] {
			// Collect answerables and corresponding answers for this skipped section.
			// Same resolved-answerable and answer-payload behavior as for active sections.
			var sectionAnswers []answer.Answer
			var sectionAnswerables []question.Answerable

			for _, ans := range swq.AnswerableList {
				questionID := ans.Question().ID.String()

				// Use the resolved answerable if available; fall back to the raw one.
				resolved, hasResolved := answerableMap[questionID]
				if hasResolved {
					sectionAnswerables = append(sectionAnswerables, resolved)
				} else {
					sectionAnswerables = append(sectionAnswerables, ans)
				}

				answerData, hasAnswer := answerPayloadMap[questionID]
				if hasAnswer {
					sectionAnswers = append(sectionAnswers, answerData.Answer)
				}
			}

			result = append(result, SectionWithAnswerableAndAnswer{
				Section:         swq.Section,
				SectionProgress: SectionProgressSkipped,
				Answerable:      sectionAnswerables,
				Answer:          sectionAnswers,
			})
		}
	}

	return response, result, nil
}

func (s Service) GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetFormIDByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	formID, err := s.queries.GetFormIDByID(traceCtx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, internal.ErrResponseNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get form id by response id")
		span.RecordError(err)
		return uuid.Nil, err
	}

	return formID, nil
}

// GetSubmittedBy returns the user ID who submitted the response with the given ID.
// This is used by the answer handler for ownership checks without creating an import cycle.
func (s Service) GetSubmittedBy(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetSubmittedBy")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	formID, err := s.queries.GetFormIDByID(traceCtx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, internal.ErrResponseNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get form id by response id")
		span.RecordError(err)
		return uuid.Nil, err
	}

	response, err := s.queries.Get(traceCtx, GetParams{ID: id, FormID: formID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, internal.ErrResponseNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get response by id")
		span.RecordError(err)
		return uuid.Nil, err
	}

	return response.SubmittedBy, nil
}

// GetByID retrieves a form response by its ID alone (without requiring the formID).
// This is a lightweight lookup used for ownership checks.
func (s Service) GetByID(ctx context.Context, id uuid.UUID) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	formID, err := s.queries.GetFormIDByID(traceCtx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FormResponse{}, internal.ErrResponseNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get form id by response id")
		span.RecordError(err)
		return FormResponse{}, err
	}

	response, err := s.queries.Get(traceCtx, GetParams{ID: id, FormID: formID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FormResponse{}, internal.ErrResponseNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get response by id")
		span.RecordError(err)
		return FormResponse{}, err
	}

	return response, nil
}

// Exists returns whether a response with the given id exists.
func (s Service) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	traceCtx, span := s.tracer.Start(ctx, "Exists")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exists, err := s.queries.Exists(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "check response exists")
		span.RecordError(err)
		return false, err
	}

	return exists, nil
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

func (s Service) UpdateSubmitted(ctx context.Context, id uuid.UUID) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "UpdateSubmitted")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	formResponse, err := s.queries.UpdateSubmitted(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "update response submitted status")
		span.RecordError(err)
		return FormResponse{}, err
	}

	return formResponse, nil
}

// ResolveWorkflowSectionsForResponse runs ResolveSections and builds sectionActiveMap when a workflow exists.
// If ErrWorkflowNotFound, it returns the error and no section ordering/active-map is produced.
// Any other error is wrapped and returned.
func (s Service) ResolveWorkflowSectionsForResponse(
	ctx context.Context,
	formID uuid.UUID,
	answerPayload []answer.Answer,
	answerableMap map[string]question.Answerable,
	responseID uuid.UUID,
) (sectionIDs []uuid.UUID, sectionActiveMap map[string]bool, err error) {
	traceCtx, span := s.tracer.Start(ctx, "ResolveWorkflowSectionsForResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	sectionIDs, err = s.workflowResolver.ResolveSections(traceCtx, formID, answerPayload, answerableMap)
	if err != nil {
		if errors.Is(err, internal.ErrWorkflowNotFound) {
			logger.Error("Workflow not found for response", zap.String("responseID", responseID.String()))
			span.RecordError(err)

			// return empty sectionIDs and sectionActiveMap and the error
			return nil, nil, err
		}

		err = fmt.Errorf("%w: %w", internal.ErrWorkflowResolveSectionsFailed, err)
		logger.Error("Failed to resolve sections for response", zap.Error(err), zap.String("responseID", responseID.String()))
		span.RecordError(err)

		// return empty sectionIDs and sectionActiveMap and the error
		return nil, nil, err
	}

	sectionActiveMap = make(map[string]bool, len(sectionIDs))
	for _, sectionID := range sectionIDs {
		sectionActiveMap[sectionID.String()] = true
	}

	// return the sectionIDs and sectionActiveMap and nil
	return sectionIDs, sectionActiveMap, nil
}

// calculateSectionProgress determines the progress status of a section based on its questions and answers
func calculateSectionProgress(answerables []question.Answerable, answerMap map[string]struct {
	Answer     answer.Answer
	Answerable question.Answerable
}) SectionProgress {
	if len(answerables) == 0 {
		return SectionProgressCompleted
	}

	hasAnyAnswer := false
	requiredCount := 0
	requiredAnsweredCount := 0

	for _, ans := range answerables {
		q := ans.Question()
		questionID := q.ID.String()

		if q.Required {
			requiredCount++
			if _, hasAnswer := answerMap[questionID]; hasAnswer {
				requiredAnsweredCount++
			}
		}

		// Check if this question has an answer
		if _, hasAnswer := answerMap[questionID]; hasAnswer {
			hasAnyAnswer = true
		}
	}

	// If no answers at all, it's NOT_STARTED
	if !hasAnyAnswer {
		return SectionProgressNotStarted
	}

	// If all required questions are answered, it's COMPLETED
	if requiredCount == requiredAnsweredCount {
		return SectionProgressCompleted
	}

	// Otherwise, it's DRAFT (at least one answer, but not all required)
	return SectionProgressDraft
}

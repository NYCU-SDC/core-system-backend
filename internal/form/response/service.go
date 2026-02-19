package response

import (
	"NYCU-SDC/core-system-backend/internal/form/answer"
	"NYCU-SDC/core-system-backend/internal/form/question"
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
	Get(ctx context.Context, id uuid.UUID) (FormResponse, error)
	GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	Create(ctx context.Context, arg CreateParams) (FormResponse, error)
	Exists(ctx context.Context, arg ExistsParams) (bool, error)
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListBySubmittedBy(ctx context.Context, submittedBy uuid.UUID) ([]FormResponse, error)
}

type WorkflowResolver interface {
	ResolveSections(ctx context.Context, formID uuid.UUID, answers []answer.Answer, answerableMap map[string]question.Answerable) ([]uuid.UUID, error)
}

type AnswerStore interface {
	List(ctx context.Context, formID, responseID uuid.UUID) ([]answer.Answer, []question.Answerable, map[string]question.Answerable, error)
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

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer

	answerStore              AnswerStore
	sectionWithQuestionStore SectionWithQuestionStore
	workflowResolver         WorkflowResolver
}

func NewService(logger *zap.Logger, db DBTX, answerStore AnswerStore, sectionStore SectionWithQuestionStore, workflowResolver WorkflowResolver) *Service {
	return &Service{
		logger:  logger,
		queries: New(db),
		tracer:  otel.Tracer("response/service"),

		answerStore:              answerStore,
		sectionWithQuestionStore: sectionStore,
		workflowResolver:         workflowResolver,
	}
}

// Create creates an empty response (draft) for a given form and user
// Returns an error if the user already has a response for the form
func (s Service) Create(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "Create")
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

// ListBySubmittedBy retrieves all responses submitted by a given user
func (s Service) ListBySubmittedBy(ctx context.Context, userID uuid.UUID) ([]FormResponse, error) {
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

// Get retrieves a form response by ID along with its sections, questions, and answers
// The sections are returned in workflow order (active sections first, then skipped sections)
func (s Service) Get(ctx context.Context, id uuid.UUID) (FormResponse, []SectionWithAnswerableAndAnswer, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Get the form response
	response, err := s.queries.Get(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get response by id")
		span.RecordError(err)
		return FormResponse{}, nil, err
	}

	// Get all answers for this response
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

	// Resolve which sections are active based on workflow conditions
	sectionIDs, err := s.workflowResolver.ResolveSections(traceCtx, response.FormID, answerPayload, answerableMap)
	if err != nil {
		err = fmt.Errorf("%w: %w", internal.ErrWorkflowResolveSectionsFailed, err)
		logger.Error("Failed to resolve sections for response", zap.Error(err), zap.String("responseID", response.ID.String()))
		span.RecordError(err)
		return FormResponse{}, nil, err
	}

	// Build active section ID map for quick lookup
	sectionActiveMap := make(map[string]bool)
	for _, sectionID := range sectionIDs {
		sectionActiveMap[sectionID.String()] = true
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
		swq, exists := sectionMap[sectionID.String()]
		if !exists {
			// This shouldn't happen - workflow returned a section ID that doesn't exist
			logger.DPanic("Section from workflow not found in section list", zap.String("sectionID", sectionID.String()))
			continue
		}

		// Collect all answerables and corresponding answers for this section
		var sectionAnswers []answer.Answer
		var sectionAnswerables []question.Answerable

		for _, ans := range swq.AnswerableList {
			questionID := ans.Question().ID.String()

			// Add all answerables (questions) in this section
			sectionAnswerables = append(sectionAnswerables, ans)

			// Add answer if it exists for this question
			if ansData, hasAnswer := answerPayloadMap[questionID]; hasAnswer {
				sectionAnswers = append(sectionAnswers, ansData.Answer)
			}
		}

		// Calculate section progress based on answers and required questions
		progress := calculateSectionProgress(swq.AnswerableList, answerPayloadMap)

		result = append(result, SectionWithAnswerableAndAnswer{
			Section:         swq.Section,
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
			// Collect all answerables and corresponding answers for this skipped section
			var sectionAnswers []answer.Answer
			var sectionAnswerables []question.Answerable

			for _, ans := range swq.AnswerableList {
				questionID := ans.Question().ID.String()

				// Add all answerables (questions) in this section
				sectionAnswerables = append(sectionAnswerables, ans)

				// Preserve existing answers even though section is skipped
				if ansData, hasAnswer := answerPayloadMap[questionID]; hasAnswer {
					sectionAnswers = append(sectionAnswers, ansData.Answer)
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

func (s Service) GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetFormIDByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	formID, err := s.queries.GetFormIDByID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get form id by response id")
		span.RecordError(err)
		return uuid.Nil, nil
	}

	return formID, nil
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

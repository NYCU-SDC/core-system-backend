package submit

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type QuestionStore interface {
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]question.Answerable, error)
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

					// Validate answer value (value is already a string from SubmitHandler)
					err := q.Validate(string(ans.Value))
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

// Update handles a user's partial update (patch) for a form response.
// This is used when users are progressively filling out a form section by section.
// It performs the following steps:
// 1. Collects all questionIds from the submitted answers.
// 2. Batch queries ONLY the questions being answered (efficient!).
// 3. Validates the submitted answers against the corresponding questions.
//   - Performs data validation (character count, host-defined conditions, etc.)
//   - If validation fails, accumulates the errors but continues processing.
//   - Filters out answers that are valid and can be saved.
//
// 4. Saves all VALID answers even if some answers failed validation.
// 5. Returns the response and any validation errors that occurred.
//
// Returns the form response and a list of validation errors (empty if all valid).
func (s *Service) Update(ctx context.Context, userID uuid.UUID, answers []shared.AnswerParam) (response.FormResponse, []error) {
	traceCtx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Validate we have at least one answer
	if len(answers) == 0 {
		return response.FormResponse{}, []error{fmt.Errorf("no answers provided")}
	}

	// Collect all question IDs from answers
	questionIDs := make([]uuid.UUID, 0, len(answers))
	for _, ans := range answers {
		qid, err := internal.ParseUUID(ans.QuestionID)
		if err != nil {
			return response.FormResponse{}, []error{fmt.Errorf("invalid question ID %s: %w", ans.QuestionID, err)}
		}
		questionIDs = append(questionIDs, qid)
	}

	// Batch query ONLY the questions being answered
	questionAnswerables, err := s.questionStore.GetByIDs(traceCtx, questionIDs)
	if err != nil {
		return response.FormResponse{}, []error{fmt.Errorf("failed to get questions: %w", err)}
	}

	if len(questionAnswerables) == 0 {
		return response.FormResponse{}, []error{fmt.Errorf("no valid questions found")}
	}

	// Get formID from first question (assume all questions belong to the same form)
	formID := questionAnswerables[0].FormID()

	// Build question map for fast lookup
	questionMap := make(map[string]question.Answerable)
	for _, answerable := range questionAnswerables {
		questionMap[answerable.Question().ID.String()] = answerable
	}

	// Validate answers and separate valid from invalid ones
	var questionTypes []response.QuestionType
	var validAnswers []shared.AnswerParam
	validationErrors := make([]error, 0)
	for _, ans := range answers {
		answerable, found := questionMap[ans.QuestionID]
		if !found {
			validationErrors = append(validationErrors, fmt.Errorf("question with ID %s not found", ans.QuestionID))
			continue
		}

		validationValue, err := convertAnswerValueForValidation(ans.Value, answerable.Question().Type)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("invalid value format for question ID %s: %w", ans.QuestionID, err))
		}

		// Validate answer value (eg. words、host condition)
		err = answerable.Validate(validationValue)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("validation error for question ID %s: %w", ans.QuestionID, err))
		}

		// Add ALL answers (even those with validation errors) to save as draft
		validAnswers = append(validAnswers, ans)
		questionTypes = append(questionTypes, response.QuestionType(answerable.Question().Type))
	}

	logger.Info("answers processed",
		zap.Int("answeredQuestions", len(validAnswers)),
		zap.Int("validationErrors", len(validationErrors)),
	)

	// If there are no answers at all, return fatal error
	if len(validAnswers) == 0 {
		logger.Warn("no valid answers to save", zap.String("formID", formID.String()), zap.Int("totalAnswers", len(answers)))
		return response.FormResponse{}, []error{fmt.Errorf("no valid questions found for provided answers")}
	}

	// Save all answers as draft
	result, err := s.responseStore.CreateOrUpdate(traceCtx, formID, userID, validAnswers, questionTypes)
	if err != nil {
		logger.Error("failed to create or update form response", zap.Error(err), zap.String("formID", formID.String()), zap.String("userID", userID.String()))
		span.RecordError(err)
		return response.FormResponse{}, []error{err}
	}

	return result, validationErrors
}

// convertAnswerValueForValidation converts a JSONB value (json.RawMessage) to the format expected by validation methods
func convertAnswerValueForValidation(rawValue json.RawMessage, questionType question.QuestionType) (string, error) {
	if len(rawValue) == 0 || string(rawValue) == "null" {
		return "", nil
	}

	switch questionType {
	case "LINEAR_SCALE", "RATING":
		var scaleValue int32
		if err := json.Unmarshal(rawValue, &scaleValue); err != nil {
			return "", fmt.Errorf("failed to parse scale value: %w", err)
		}
		return strconv.FormatInt(int64(scaleValue), 10), nil

	case "OAUTH_CONNECT":
		return string(rawValue), nil

	default:
		var stringArray []string
		if err := json.Unmarshal(rawValue, &stringArray); err != nil {
			return "", fmt.Errorf("failed to parse answer value as string array: %w", err)
		}
		return strings.Join(stringArray, ";"), nil
	}
}

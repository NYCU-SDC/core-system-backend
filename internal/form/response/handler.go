package response

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/answer"
	"NYCU-SDC/core-system-backend/internal/form/question"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type AnswerForQuestionResponse struct {
	ID          string    `json:"id" validate:"required,uuid"`
	ResponseID  string    `json:"responseId" validate:"required,uuid"`
	SubmittedBy string    `json:"submittedBy" validate:"required,uuid"`
	Value       string    `json:"value" validate:"required"`
	CreatedAt   time.Time `json:"createdAt" validate:"required,datetime"` // for sorting
	UpdatedAt   time.Time `json:"updatedAt" validate:"required,datetime"` // for marking if the answer is updated
}

type Response struct {
	ID          string    `json:"id" validate:"required,uuid"`
	SubmittedBy string    `json:"submittedBy" validate:"required,uuid"`
	CreatedAt   time.Time `json:"createdAt" validate:"required,datetime"`
	UpdatedAt   time.Time `json:"updatedAt" validate:"required,datetime"`
}

type ListResponse struct {
	FormID        string     `json:"formId" validate:"required,uuid"`
	ResponseJSONs []Response `json:"responses" validate:"required,dive"`
}

type GetFormResponse struct {
	ID       string           `json:"id" validate:"required,uuid"`
	FormID   string           `json:"formId" validate:"required,uuid"`
	Progress string           `json:"progress" validate:"required,oneof=DRAFT SUBMITTED"`
	Sections []SectionDetails `json:"sections" validate:"required,dive"`
}

type SectionDetails struct {
	ID            string          `json:"id" validate:"required,uuid"`
	Title         string          `json:"title" validate:"required"`
	Progress      string          `json:"progress" validate:"required,oneof=SKIPPED NOT_STARTED DRAFT COMPLETED"`
	AnswerDetails []AnswerDetails `json:"answerDetails" validate:"required,dive"`
}

type AnswerDetails struct {
	Question question.Response `json:"question" validate:"required"`
	Payload  *AnswerPayload    `json:"payload"`
}

type AnswerPayload struct {
	CreatedAt    time.Time       `json:"createdAt" validate:"required"`
	UpdatedAt    time.Time       `json:"updatedAt" validate:"required"`
	ResponseID   string          `json:"responseId" validate:"required,uuid"`
	Answer       json.RawMessage `json:"answer" validate:"required"`
	DisplayValue string          `json:"displayValue" validate:"required"`
}

type AnswersForQuestionResponse struct {
	Question question.Question           `json:"question" validate:"required"`
	Answers  []AnswerForQuestionResponse `json:"answers" validate:"required,dive"`
}

type CreateResponse struct {
	ID string `json:"id" validate:"required,uuid"`
}

type Store interface {
	Get(ctx context.Context, id uuid.UUID) (FormResponse, []SectionWithAnswerableAndAnswer, error)
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error)
	Create(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (FormResponse, error)
	Delete(ctx context.Context, responseID uuid.UUID) error
}

type QuestionStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (question.Answerable, error)
}

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
	questionStore QuestionStore
	tracer        trace.Tracer
}

func NewHandler(logger *zap.Logger, validator *validator.Validate, problemWriter *problem.HttpWriter, store Store, questionStore QuestionStore) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		questionStore: questionStore,
		tracer:        otel.Tracer("response/handler"),
	}
}

// List lists all responses for a form
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "List")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responses, err := h.store.ListByFormID(traceCtx, formID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	listResponse := ListResponse{
		FormID:        formID.String(),
		ResponseJSONs: make([]Response, len(responses)),
	}
	for i, currentResponse := range responses {
		listResponse.ResponseJSONs[i] = Response{
			ID:          currentResponse.ID.String(),
			SubmittedBy: currentResponse.SubmittedBy.String(),
			CreatedAt:   currentResponse.CreatedAt.Time,
			UpdatedAt:   currentResponse.UpdatedAt.Time,
		}
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, listResponse)
}

// Get retrieves a response by id with all sections, questions and answers
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Parse formId from path
	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Parse responseId from path
	responseIDStr := r.PathValue("responseId")
	responseID, err := handlerutil.ParseUUID(responseIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Get response with sections and answers from store
	formResponse, sections, err := h.store.Get(traceCtx, responseID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Verify formID matches
	if formResponse.FormID != formID {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrResponseFormIDMismatch, logger)
		return
	}

	// Convert to response format
	response, err := h.toGetFormResponse(traceCtx, formResponse, sections)
	if err != nil {
		logger.Error("Failed to convert to GetFormResponse", zap.Error(err))
		span.RecordError(err)
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

// toGetFormResponse converts service layer data to handler response format
func (h *Handler) toGetFormResponse(ctx context.Context, formResponse FormResponse, sections []SectionWithAnswerableAndAnswer) (GetFormResponse, error) {
	traceCtx, span := h.tracer.Start(ctx, "toGetFormResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Build answer map for quick lookup by question ID
	answerMap := make(map[string]answer.Answer)

	for _, section := range sections {
		for _, ans := range section.Answer {
			answerMap[ans.QuestionID.String()] = ans
		}
	}

	// Convert sections
	sectionDetails := make([]SectionDetails, len(sections))
	for i, section := range sections {
		// Convert section title (handle null)
		sectionTitle := ""
		if section.Section.Title.Valid {
			sectionTitle = section.Section.Title.String
		}

		// Convert answerables to answer details
		answerDetails := make([]AnswerDetails, len(section.Answerable))
		for j, answerable := range section.Answerable {
			// Convert question to response format
			questionResponse, err := question.ToResponse(answerable)
			if err != nil {
				logger.Error("Failed to convert question to response",
					zap.String("questionID", answerable.Question().ID.String()),
					zap.Error(err))
				span.RecordError(err)
				return GetFormResponse{}, err
			}

			// Build answer details
			answerDetails[j] = AnswerDetails{
				Question: questionResponse,
				Payload:  nil, // Default to nil if no answer
			}

			// If answer exists, populate payload
			questionID := answerable.Question().ID.String()
			if ans, hasAnswer := answerMap[questionID]; hasAnswer {
				displayValue, err := answerable.DisplayValue(ans.Value)
				if err != nil {
					logger.Error("Failed to get display value",
						zap.String("questionID", questionID),
						zap.Error(err))
					span.RecordError(err)
					return GetFormResponse{}, err
				}

				// Decode and encode answer value
				valueStruct, err := answerable.DecodeStorage(ans.Value)
				if err != nil {
					logger.Error("Failed to decode answer value from storage",
						zap.String("questionID", questionID),
						zap.Error(err))
					span.RecordError(err)
					return GetFormResponse{}, err
				}

				payload, err := answerable.EncodeRequest(valueStruct)
				if err != nil {
					logger.Error("Failed to encode answer value for response",
						zap.String("questionID", questionID),
						zap.Error(err))
					span.RecordError(err)
					return GetFormResponse{}, err
				}

				// Build answer payload with proper question type and value
				answerPayload := map[string]interface{}{
					"questionId":   questionID,
					"questionType": strings.ToUpper(string(answerable.Question().Type)),
					"value":        json.RawMessage(payload),
				}

				answerPayloadJSON, err := json.Marshal(answerPayload)
				if err != nil {
					logger.Error("Failed to marshal answer payload",
						zap.String("questionID", questionID),
						zap.Error(err))
					span.RecordError(err)
					return GetFormResponse{}, err
				}

				answerDetails[j].Payload = &AnswerPayload{
					CreatedAt:    ans.CreatedAt.Time,
					UpdatedAt:    ans.UpdatedAt.Time,
					ResponseID:   formResponse.ID.String(),
					Answer:       answerPayloadJSON,
					DisplayValue: displayValue,
				}
			}
		}

		sectionDetails[i] = SectionDetails{
			ID:            section.Section.ID.String(),
			Title:         sectionTitle,
			Progress:      strings.ToUpper(string(section.SectionProgress)),
			AnswerDetails: answerDetails,
		}
	}

	return GetFormResponse{
		ID:       formResponse.ID.String(),
		FormID:   formResponse.FormID.String(),
		Progress: strings.ToUpper(string(formResponse.Progress)),
		Sections: sectionDetails,
	}, nil
}

// Create creates an empty response for a form
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Create")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Extract form ID from path
	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Get authenticated user
	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	// Create empty response
	newResponse, err := h.store.Create(traceCtx, formID, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Return response with 201 Created
	handlerutil.WriteJSONResponse(w, http.StatusCreated, CreateResponse{
		ID: newResponse.ID.String(),
	})
}

// Delete deletes a response by id
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Delete")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("responseId")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	err = h.store.Delete(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

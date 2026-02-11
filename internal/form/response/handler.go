package response

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"net/http"
	"time"

	"NYCU-SDC/core-system-backend/internal"
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

type QuestionAnswerForGetResponse struct {
	QuestionID string `json:"questionId" validate:"required,uuid"`
	Answer     string `json:"answer" validate:"required"`
}

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

type GetResponse struct {
	ID                   string                         `json:"id" validate:"required,uuid"`
	FormID               string                         `json:"formId" validate:"required,uuid"`
	SubmittedBy          string                         `json:"submittedBy" validate:"required,uuid"`
	QuestionsAnswerPairs []QuestionAnswerForGetResponse `json:"questionsAnswerPairs" validate:"required,dive"`
	CreatedAt            time.Time                      `json:"createdAt" validate:"required,datetime"` // for sorting
	UpdatedAt            time.Time                      `json:"updatedAt" validate:"required,datetime"` // for marking if the response is updated
}

type AnswersForQuestionResponse struct {
	Question question.Question           `json:"question" validate:"required"`
	Answers  []AnswerForQuestionResponse `json:"answers" validate:"required,dive"`
}

type CreateResponse struct {
	ID string `json:"id" validate:"required,uuid"`
}

type Store interface {
	Get(ctx context.Context, formID uuid.UUID, responseID uuid.UUID) (FormResponse, []Answer, error)
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error)
	Delete(ctx context.Context, responseID uuid.UUID) error
	GetAnswersByQuestionID(ctx context.Context, questionID uuid.UUID, formID uuid.UUID) ([]GetAnswersByQuestionIDRow, error)
	CreateEmpty(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (FormResponse, error)
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

// ListHandler lists all responses for a form
func (h *Handler) ListHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := internal.ParseUUID(formIDStr)
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

// GetHandler retrieves a response by id
func (h *Handler) GetHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := internal.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	idStr := r.PathValue("responseId")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentResponse, answers, err := h.store.Get(traceCtx, formID, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionAnswerResponses := make([]QuestionAnswerForGetResponse, len(answers))
	for i, answer := range answers {
		q, err := h.questionStore.GetByID(traceCtx, answer.QuestionID)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}

		questionAnswerResponses[i] = QuestionAnswerForGetResponse{
			QuestionID: q.Question().ID.String(),
			Answer:     answer.Value,
		}
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, GetResponse{
		ID:                   currentResponse.ID.String(),
		FormID:               currentResponse.FormID.String(),
		SubmittedBy:          currentResponse.SubmittedBy.String(),
		QuestionsAnswerPairs: questionAnswerResponses,
		CreatedAt:            currentResponse.CreatedAt.Time,
		UpdatedAt:            currentResponse.UpdatedAt.Time,
	})
}

// DeleteHandler deletes a response by id
func (h *Handler) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("responseId")
	id, err := internal.ParseUUID(idStr)
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

// GetAnswersByQuestionIDHandler gets answers by question id
func (h *Handler) GetAnswersByQuestionIDHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetAnswersByQuestionIDHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := internal.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionIDStr := r.PathValue("questionId")
	questionID, err := internal.ParseUUID(questionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	answers, err := h.store.GetAnswersByQuestionID(traceCtx, questionID, formID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentQuestion, err := h.questionStore.GetByID(traceCtx, questionID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionAnswerResponse := AnswersForQuestionResponse{
		Question: currentQuestion.Question(),
		Answers:  make([]AnswerForQuestionResponse, len(answers)),
	}
	for i, answer := range answers {
		questionAnswerResponse.Answers[i] = AnswerForQuestionResponse{
			ID:          answer.ID.String(),
			ResponseID:  answer.ResponseID.String(),
			SubmittedBy: answer.SubmittedBy.String(),
			Value:       answer.Value,
			CreatedAt:   answer.CreatedAt.Time,
			UpdatedAt:   answer.UpdatedAt.Time,
		}
	}
	handlerutil.WriteJSONResponse(w, http.StatusOK, questionAnswerResponse)
}

// CreateFormResponseHandler creates an empty response for a form
func (h *Handler) CreateFormResponseHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "CreateFormResponseHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)
	// Extract form ID from path
	formIDStr := r.PathValue("formId")
	formID, err := internal.ParseUUID(formIDStr)
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
	newResponse, err := h.store.CreateEmpty(traceCtx, formID, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}
	// Return response with 201 Created
	handlerutil.WriteJSONResponse(w, http.StatusCreated, CreateResponse{
		ID: newResponse.ID.String(),
	})
}

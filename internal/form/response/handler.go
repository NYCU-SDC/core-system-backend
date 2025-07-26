package response

import (
	"context"
	"net/http"
	"time"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/user"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type AnswerRequest struct {
	QuestionID string `json:"questionId" validate:"required,uuid"`
	Value      string `json:"value" validate:"required"`
}

type QuestionAnswerForGetResponse struct {
	Question question.Question `json:"question" validate:"required"`
	Answer   string            `json:"answer" validate:"required"`
}

type AnswerForQuestionResponse struct {
	ID          string    `json:"id" validate:"required,uuid"`
	ResponseID  string    `json:"responseId" validate:"required,uuid"`
	SubmittedBy string    `json:"submittedBy" validate:"required,uuid"`
	Value       string    `json:"value" validate:"required"`
	CreatedAt   time.Time `json:"createdAt" validate:"required,datetime"` // for sorting
	UpdatedAt   time.Time `json:"updatedAt" validate:"required,datetime"` // for marking if the answer is updated
}

type SubmitRequest struct {
	Answers []AnswerRequest `json:"answers" validate:"required,dive"`
}
type ResponseJSON struct {
	ID          string    `json:"id" validate:"required,uuid"`
	SubmittedBy string    `json:"submittedBy" validate:"required,uuid"`
	CreatedAt   time.Time `json:"createdAt" validate:"required,datetime"`
	UpdatedAt   time.Time `json:"updatedAt" validate:"required,datetime"`
}

type SubmitResponse struct {
	ID        string    `json:"id" validate:"required,uuid"`
	FormID    string    `json:"formId" validate:"required,uuid"`
	CreatedAt time.Time `json:"createdAt" validate:"required,datetime"`
	UpdatedAt time.Time `json:"updatedAt" validate:"required,datetime"`
}

type ListResponse struct {
	FormID        string         `json:"formId" validate:"required,uuid"`
	ResponseJSONs []ResponseJSON `json:"responses" validate:"required,dive"`
}

type GetResponse struct {
	ID          string                         `json:"id" validate:"required,uuid"`
	FormID      string                         `json:"formId" validate:"required,uuid"`
	SubmittedBy string                         `json:"submittedBy" validate:"required,uuid"`
	Questions   []QuestionAnswerForGetResponse `json:"questions" validate:"required,dive"`
	CreatedAt   time.Time                      `json:"createdAt" validate:"required,datetime"` // for sorting
	UpdatedAt   time.Time                      `json:"updatedAt" validate:"required,datetime"` // for marking if the response is updated
}

type AnswersForQuestionResponse struct {
	Question question.Question           `json:"question" validate:"required"`
	Answers  []AnswerForQuestionResponse `json:"answers" validate:"required,dive"`
}

type Store interface {
	Submit(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []Answer) (Response, error)
	Get(ctx context.Context, formID uuid.UUID, responseID uuid.UUID) (Response, []Answer, error)
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]Response, error)
	Delete(ctx context.Context, responseID uuid.UUID) error
	GetAnswersByQuestionID(ctx context.Context, questionID uuid.UUID, formID uuid.UUID) ([]GetAnswersByQuestionIDRow, error)
}

type QuestionStore interface {
	GetByID(ctx context.Context, questionID uuid.UUID) (question.Question, error)
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

// SubmitHandler submits a response to a form
func (h *Handler) SubmitHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "SubmitResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIdStr := r.PathValue("formId")
	formId, err := internal.ParseUUID(formIdStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var request SubmitRequest
	err = handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &request)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	answers := make([]Answer, len(request.Answers))
	for i, answer := range request.Answers {
		answers[i] = Answer{
			QuestionID: uuid.MustParse(answer.QuestionID),
			Value:      answer.Value,
		}
	}

	newResponse, err := h.store.Submit(traceCtx, formId, currentUser.ID, answers)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	submitResponse := SubmitResponse{
		ID:        newResponse.ID.String(),
		FormID:    newResponse.FormID.String(),
		CreatedAt: newResponse.CreatedAt.Time,
		UpdatedAt: newResponse.UpdatedAt.Time,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, submitResponse)
}

// ListHandler lists all responses for a form
func (h *Handler) ListHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListResponses")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIdStr := r.PathValue("formId")
	formId, err := internal.ParseUUID(formIdStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responses, err := h.store.ListByFormID(traceCtx, formId)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	listResponse := ListResponse{
		FormID:        formId.String(),
		ResponseJSONs: make([]ResponseJSON, len(responses)),
	}
	for i, currentResponse := range responses {
		listResponse.ResponseJSONs[i] = ResponseJSON{
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
	traceCtx, span := h.tracer.Start(r.Context(), "GetResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIdStr := r.PathValue("formId")
	formId, err := internal.ParseUUID(formIdStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responseIdStr := r.PathValue("responseId")
	responseId, err := internal.ParseUUID(responseIdStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentResponse, answers, err := h.store.Get(traceCtx, formId, responseId)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionAnswerResponses := make([]QuestionAnswerForGetResponse, len(answers))
	for i, answer := range answers {
		question, err := h.questionStore.GetByID(traceCtx, answer.QuestionID)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}

		questionAnswerResponses[i] = QuestionAnswerForGetResponse{
			Question: question,
			Answer:   answer.Value,
		}
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, GetResponse{
		ID:          currentResponse.ID.String(),
		FormID:      currentResponse.FormID.String(),
		SubmittedBy: currentResponse.SubmittedBy.String(),
		Questions:   questionAnswerResponses,
		CreatedAt:   currentResponse.CreatedAt.Time,
		UpdatedAt:   currentResponse.UpdatedAt.Time,
	})
}

// DeleteHandler deletes a response by id
func (h *Handler) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	responseIdStr := r.PathValue("responseId")
	responseId, err := internal.ParseUUID(responseIdStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	err = h.store.Delete(traceCtx, responseId)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, nil)
}

// GetAnswersByQuestionIDHandler gets answers by question id
func (h *Handler) GetAnswersByQuestionIDHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetAnswersByQuestionID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIdStr := r.PathValue("formId")
	formId, err := internal.ParseUUID(formIdStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionIdStr := r.PathValue("questionId")
	questionId, err := internal.ParseUUID(questionIdStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	answers, err := h.store.GetAnswersByQuestionID(traceCtx, questionId, formId)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	question, err := h.questionStore.GetByID(traceCtx, questionId)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionAnswerResponse := AnswersForQuestionResponse{
		Question: question,
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

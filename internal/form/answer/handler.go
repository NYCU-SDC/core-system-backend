package answer

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/shared"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Payload represents a text-based answer (for most question types)
type Payload struct {
	QuestionID   string          `json:"questionId" validate:"required,uuid"`
	QuestionType string          `json:"questionType" validate:"required"`
	Value        json.RawMessage `json:"value" validate:"required"`
}

// AnswersRequest is the request body for updating answers
type AnswersRequest struct {
	Answers []Payload `json:"answers" validate:"required,dive"`
}

// GetQuestionResponse is the response for getting a specific question's answer
type GetQuestionResponse struct {
	ID                 string  `json:"id" validate:"required,uuid"`
	QuestionAnswerPair Payload `json:"questionAnswerPair" validate:"required"`
}

// UpdateResponse is the response for updating answers
type UpdateResponse struct {
	ID string `json:"id" validate:"required,uuid"`
}

type Store interface {
	Get(ctx context.Context, formID, responseID, questionID uuid.UUID) (Answer, QuestionType, error)
	Upsert(ctx context.Context, formID, responseID uuid.UUID, answers []shared.AnswerParam) ([]Answer, []error)
}

type QuestionGetter interface {
	GetByID(ctx context.Context, id uuid.UUID) (question.Answerable, error)
}

type ResponseStore interface {
	GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
	questionStore QuestionGetter
	responseStore ResponseStore
	tracer        trace.Tracer
}

func NewHandler(logger *zap.Logger, validator *validator.Validate, problemWriter *problem.HttpWriter, store Store, questionStore QuestionGetter, responseStore ResponseStore) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		questionStore: questionStore,
		responseStore: responseStore,
		tracer:        otel.Tracer("answer/handler"),
	}
}

// GetQuestionResponse gets a specific answer by question ID in the response
// GET /responses/{responseId}/questions/{questionId}
func (h *Handler) GetQuestionResponse(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetQuestionResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Parse responseId from path
	responseIDStr := r.PathValue("responseId")
	responseID, err := internal.ParseUUID(responseIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Parse questionId from path
	questionIDStr := r.PathValue("questionId")
	questionID, err := internal.ParseUUID(questionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Get formID from responseID
	formID, err := h.responseStore.GetFormIDByID(traceCtx, responseID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Get answer from store
	answer, questionType, err := h.store.Get(traceCtx, formID, responseID, questionID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := GetQuestionResponse{
		ID: answer.ID.String(),
		QuestionAnswerPair: Payload{
			QuestionID:   questionID.String(),
			QuestionType: strings.ToUpper(string(questionType)),
			Value:        answer.Value,
		},
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

// UpdateFormResponse updates answers for the response
// PATCH /responses/{responseId}/answers
func (h *Handler) UpdateFormResponse(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateFormResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Parse responseId from path
	responseIDStr := r.PathValue("responseId")
	responseID, err := internal.ParseUUID(responseIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Parse and validate request body
	var req AnswersRequest
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Get formID from responseID
	formID, err := h.responseStore.GetFormIDByID(traceCtx, responseID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	answerParams := make([]shared.AnswerParam, 0, len(req.Answers))
	for _, answerRequest := range req.Answers {
		answerParams = append(answerParams, shared.AnswerParam{
			QuestionID: answerRequest.QuestionID,
			Value:      answerRequest.Value,
		})
	}

	// Upsert answers
	_, errs := h.store.Upsert(traceCtx, formID, responseID, answerParams)
	if len(errs) > 0 {
		errStrings := make([]string, 0, len(errs))
		for _, err := range errs {
			errStrings = append(errStrings, err.Error())
			span.RecordError(err)
		}

		err = handlerutil.NewValidationErrorWithErrors("validation errors occurred while upserting answers", errStrings)
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := UpdateResponse{
		ID: responseID.String(),
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

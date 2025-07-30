package question

import (
	"context"
	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"net/http"
	"time"
)

type Request struct {
	FormID      uuid.UUID
	Required    bool   `json:"required" validate:"required"`
	Type        string `json:"type" validate:"required,oneof=short_text long_text single_choice multiple_choice date"`
	Title       string `json:"title" validate:"required"`
	Description string `json:"description"`
	Order       int32  `json:"order" validate:"required"`
}

type Response struct {
	ID          uuid.UUID `json:"id"`
	FormID      uuid.UUID `json:"formId"`
	Required    bool      `json:"required"`
	Type        string    `json:"type"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	Order       int32     `json:"order"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func ToResponse(answerable Answerable) Response {
	q := answerable.Question()

	return Response{
		ID:          q.ID,
		FormID:      q.FormID,
		Required:    q.Required,
		Type:        string(q.Type),
		Label:       q.Title.String,
		Description: q.Description.String,
		Order:       q.Order,
		CreatedAt:   q.CreatedAt.Time,
		UpdatedAt:   q.UpdatedAt.Time,
	}
}

type Store interface {
	Create(ctx context.Context, input CreateParams) (Answerable, error)
	Update(ctx context.Context, input UpdateParams) (Answerable, error)
	Delete(ctx context.Context, formID uuid.UUID, id uuid.UUID) error
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]Answerable, error)
}

type Handler struct {
	logger *zap.Logger
	tracer trace.Tracer

	validator     *validator.Validate
	problemWriter *problem.HttpWriter

	store Store
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
) *Handler {
	return &Handler{
		logger:        logger,
		tracer:        otel.Tracer("question/handler"),
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
	}
}

func (h *Handler) AddQuestionHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "AddQuestionHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var req Request
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	request := CreateParams{
		FormID:      formID,
		Required:    req.Required,
		Type:        QuestionType(req.Type),
		Title:       pgtype.Text{String: req.Title, Valid: true},
		Description: pgtype.Text{String: req.Description, Valid: true},
		Order:       req.Order,
	}

	createdQuestion, err := h.store.Create(r.Context(), request)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := ToResponse(createdQuestion)

	handlerutil.WriteJSONResponse(w, http.StatusCreated, response)
}

func (h *Handler) UpdateQuestionHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateQuestionHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	questionIDStr := r.PathValue("questionId")
	questionID, err := handlerutil.ParseUUID(questionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var req Request
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	request := UpdateParams{
		ID:          questionID,
		FormID:      formID,
		Required:    req.Required,
		Type:        QuestionType(req.Type),
		Title:       pgtype.Text{String: req.Title, Valid: true},
		Description: pgtype.Text{String: req.Description, Valid: true},
		Order:       req.Order,
	}

	updatedQuestion, err := h.store.Update(traceCtx, request)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := ToResponse(updatedQuestion)

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) DeleteQuestionHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteQuestionHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionIDStr := r.PathValue("questionId")
	questionID, err := handlerutil.ParseUUID(questionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	err = h.store.Delete(traceCtx, formID, questionID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

func (h *Handler) ListQuestionsHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListQuestionsHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questions, err := h.store.ListByFormID(traceCtx, formID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responses := make([]Response, len(questions))
	for i, q := range questions {
		responses[i] = ToResponse(q)
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, responses)
}

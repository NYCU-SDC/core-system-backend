package question

import (
	"context"
	"net/http"
	"time"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Request struct {
	Required    bool           `json:"required" validate:"required"`
	Type        string         `json:"type" validate:"required,oneof=short_text long_text single_choice multiple_choice date"`
	Title       string         `json:"title" validate:"required"`
	Description string         `json:"description"`
	Order       int32          `json:"order" validate:"required"`
	Choices     []ChoiceOption `json:"choices,omitempty" validate:"omitempty,dive"`
}

type Response struct {
	ID          uuid.UUID `json:"id"`
	FormID      uuid.UUID `json:"formId"`
	Required    bool      `json:"required"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Order       int32     `json:"order"`
	Choices     []Choice  `json:"choices,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func ToResponse(answerable Answerable) (Response, error) {
	q := answerable.Question()

	response := Response{
		ID:          q.ID,
		FormID:      q.FormID,
		Required:    q.Required,
		Type:        string(q.Type),
		Title:       q.Title.String,
		Description: q.Description.String,
		Order:       q.Order,
		CreatedAt:   q.CreatedAt.Time,
		UpdatedAt:   q.UpdatedAt.Time,
	}

	// Add choices for choice-based questions
	if q.Type == QuestionTypeSingleChoice || q.Type == QuestionTypeMultipleChoice {
		choices, err := ExtractChoices(q.Metadata)
		if err != nil {
			return response, ErrInvalidChoices{
				QuestionID: q.ID.String(),
				RawData:    q.Metadata,
				Message:    err.Error(),
			}
		}
		response.Choices = choices
	}

	return response, nil
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

func (h *Handler) AddHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "AddHandler")
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

	// Generate and validate metadata for choice-based questions
	metadata, err := GenerateMetadata(req.Type, req.Choices)
	if err != nil {
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
		Metadata:    metadata,
	}

	createdQuestion, err := h.store.Create(r.Context(), request)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response, err := ToResponse(createdQuestion)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, response)
}

func (h *Handler) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("questionId")
	id, err := handlerutil.ParseUUID(idStr)
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

	// Generate and validate metadata for choice-based questions
	metadata, err := GenerateMetadata(req.Type, req.Choices)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	request := UpdateParams{
		ID:          id,
		FormID:      formID,
		Required:    req.Required,
		Type:        QuestionType(req.Type),
		Title:       pgtype.Text{String: req.Title, Valid: true},
		Description: pgtype.Text{String: req.Description, Valid: true},
		Order:       req.Order,
		Metadata:    metadata,
	}

	updatedQuestion, err := h.store.Update(traceCtx, request)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response, err := ToResponse(updatedQuestion)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	idStr := r.PathValue("questionId")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	err = h.store.Delete(traceCtx, formID, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

func (h *Handler) ListHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListHandler")
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
		response, err := ToResponse(q)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}
		responses[i] = response
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, responses)
}

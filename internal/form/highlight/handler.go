package highlight

import (
	"context"
	"net/http"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type setRequest struct {
	QuestionID   *uuid.UUID `json:"questionId"`
	DisplayTitle *string    `json:"displayTitle"`
}

type Store interface {
	Get(ctx context.Context, formID uuid.UUID) (Response, error)
	Set(ctx context.Context, formID uuid.UUID, req Request) (Response, error)
}

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
	tracer        trace.Tracer
}

func NewHandler(logger *zap.Logger, validator *validator.Validate, problemWriter *problem.HttpWriter, store Store) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		tracer:        otel.Tracer("highlight/handler"),
	}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formID, err := handlerutil.ParseUUID(r.PathValue("formId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response, err := h.store.Get(traceCtx, formID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Put")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formID, err := handlerutil.ParseUUID(r.PathValue("formId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var req setRequest
	err = handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response, err := h.store.Set(traceCtx, formID, Request(req))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

package view

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type ViewResponse struct {
	ID        string    `json:"id"`
	FormID    string    `json:"formId"`
	Title     string    `json:"title"`
	Locked    bool      `json:"locked"`
	Order     int32     `json:"order"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type UpdateViewRequest struct {
	Title *string `json:"title"`
	Order *int32  `json:"order"`
}

type Store interface {
	Create(ctx context.Context, formID uuid.UUID) (View, error)
	List(ctx context.Context, formID uuid.UUID) ([]View, error)
	Get(ctx context.Context, formID, viewID uuid.UUID) (View, error)
	UpdateTitle(ctx context.Context, formID, viewID uuid.UUID, title string) (View, error)
	UpdateOrder(ctx context.Context, formID, viewID uuid.UUID, newOrder int32) (View, error)
	Lock(ctx context.Context, formID, viewID uuid.UUID) (View, error)
	Unlock(ctx context.Context, formID, viewID uuid.UUID) (View, error)
	Duplicate(ctx context.Context, formID, viewID uuid.UUID) (View, error)
	Delete(ctx context.Context, formID, viewID uuid.UUID) error
}

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
	tracer        trace.Tracer
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		tracer:        otel.Tracer("view/handler"),
	}
}

func toViewResponse(v View) ViewResponse {
	return ViewResponse{
		ID:        v.ID.String(),
		FormID:    v.FormID.String(),
		Title:     v.Title,
		Locked:    v.Locked,
		Order:     v.Order,
		CreatedAt: v.CreatedAt.Time,
		UpdatedAt: v.UpdatedAt.Time,
	}
}

// Create handles POST /forms/{formId}/views
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Create")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formID, err := handlerutil.ParseUUID(r.PathValue("formId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	v, err := h.store.Create(traceCtx, formID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, toViewResponse(v))
}

// List handles GET /forms/{formId}/views
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "List")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formID, err := handlerutil.ParseUUID(r.PathValue("formId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	views, err := h.store.List(traceCtx, formID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	resp := make([]ViewResponse, 0, len(views))
	for _, v := range views {
		resp = append(resp, toViewResponse(v))
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, resp)
}

// Get handles GET /forms/{formId}/views/{viewId}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formID, err := handlerutil.ParseUUID(r.PathValue("formId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	viewID, err := handlerutil.ParseUUID(r.PathValue("viewId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	v, err := h.store.Get(traceCtx, formID, viewID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, toViewResponse(v))
}

// Update handles PATCH /forms/{formId}/views/{viewId}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Update")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formID, err := handlerutil.ParseUUID(r.PathValue("formId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	viewID, err := handlerutil.ParseUUID(r.PathValue("viewId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var req UpdateViewRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Get current state so we can return a consistent view after partial updates.
	current, err := h.store.Get(traceCtx, formID, viewID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	result := current

	if req.Title != nil {
		result, err = h.store.UpdateTitle(traceCtx, formID, viewID, *req.Title)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}
	}

	if req.Order != nil {
		result, err = h.store.UpdateOrder(traceCtx, formID, viewID, *req.Order)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, toViewResponse(result))
}

// Lock handles POST /forms/{formId}/views/{viewId}/lock
func (h *Handler) Lock(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Lock")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formID, err := handlerutil.ParseUUID(r.PathValue("formId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	viewID, err := handlerutil.ParseUUID(r.PathValue("viewId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	v, err := h.store.Lock(traceCtx, formID, viewID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, toViewResponse(v))
}

// Unlock handles POST /forms/{formId}/views/{viewId}/unlock
func (h *Handler) Unlock(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Unlock")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formID, err := handlerutil.ParseUUID(r.PathValue("formId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	viewID, err := handlerutil.ParseUUID(r.PathValue("viewId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	v, err := h.store.Unlock(traceCtx, formID, viewID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, toViewResponse(v))
}

// Duplicate handles POST /forms/{formId}/views/{viewId}/duplicate
func (h *Handler) Duplicate(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Duplicate")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formID, err := handlerutil.ParseUUID(r.PathValue("formId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	viewID, err := handlerutil.ParseUUID(r.PathValue("viewId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	v, err := h.store.Duplicate(traceCtx, formID, viewID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, toViewResponse(v))
}

// Delete handles DELETE /forms/{formId}/views/{viewId}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Delete")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formID, err := handlerutil.ParseUUID(r.PathValue("formId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	viewID, err := handlerutil.ParseUUID(r.PathValue("viewId"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	if err = h.store.Delete(traceCtx, formID, viewID); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

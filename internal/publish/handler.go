package publish

import (
	"net/http"

	"NYCU-SDC/core-system-backend/internal"
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

type PreviewResponse struct {
	Recipients []uuid.UUID `json:"recipients"`
}

type Request struct {
	OrgID   uuid.UUID   `json:"orgId"`
	UnitIDs []uuid.UUID `json:"unitIds"`
}

type Handler struct {
	logger        *zap.Logger
	tracer        trace.Tracer
	validator     *validator.Validate
	problemWriter *problem.HttpWriter

	service *Service
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	service *Service,
) *Handler {
	return &Handler{
		logger:        logger,
		tracer:        otel.Tracer("publish/handler"),
		validator:     validator,
		problemWriter: problemWriter,
		service:       service,
	}
}

func (h *Handler) PreviewForm(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "PreviewForm")
	defer span.End()
	logger := logutil.WithContext(ctx, h.logger)

	var req Request
	if err := handlerutil.ParseAndValidateRequestBody(ctx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(ctx, w, err, logger)
		return
	}

	list, err := h.service.GetRecipients(ctx, Selection{
		OrgID:   req.OrgID,
		UnitIDs: req.UnitIDs,
	})
	if err != nil {
		h.problemWriter.WriteError(ctx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, PreviewResponse{Recipients: list})
}

func (h *Handler) PublishForm(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "PublishForm")
	defer span.End()
	logger := logutil.WithContext(ctx, h.logger)

	idStr := r.PathValue("id")
	formID, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(ctx, w, err, logger)
		return
	}

	var req Request
	if err := handlerutil.ParseAndValidateRequestBody(ctx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(ctx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(ctx)
	if !ok {
		h.problemWriter.WriteError(ctx, w, internal.ErrNoUserInContext, logger)
		return
	}

	if err := h.service.PublishForm(ctx, formID, req.UnitIDs, currentUser.ID); err != nil {
		h.problemWriter.WriteError(ctx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

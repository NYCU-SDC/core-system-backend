package tenant

import (
	"context"
	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"net/http"
)

type Store interface {
	GetStatusWithHistory(ctx context.Context, slug string) (bool, uuid.UUID, []History, error)
	GetStatus(ctx context.Context, slug string) (bool, uuid.UUID, error)
}

type Handler struct {
	logger        *zap.Logger
	tracer        trace.Tracer
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
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
		tracer:        otel.Tracer("tenant/handler"),
	}
}

type ResponseStatus struct {
	Available bool      `json:"available"`
	OrgId     uuid.UUID `json:"orgId"`
}
type Response struct {
	ResponseStatus `json:"current"`
	History        []History `json:"history"`
}

func (h *Handler) GetStatusWithHistory(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetStatusWithHistory")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	//h.logger.Info("GetStatusWithHistory", zap.String("slug", r.PathValue("slug")))
	available, orgID, history, err := h.store.GetStatusWithHistory(traceCtx, r.PathValue("slug"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, h.logger)
		return
	}
	response := Response{
		ResponseStatus{
			Available: available,
			OrgId:     orgID,
		},
		history,
	}
	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetStatus")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	available, orgID, err := h.store.GetStatus(traceCtx, r.PathValue("slug"))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, h.logger)
		return
	}
	response := ResponseStatus{
		Available: available,
		OrgId:     orgID,
	}
	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

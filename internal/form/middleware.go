package form

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/auth/resolver"
	"context"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"net/http"
)

type FormService interface {
	Archived(ctx context.Context, id uuid.UUID) (bool, error)
}

type Middleware struct {
	logger        *zap.Logger
	tracer        trace.Tracer
	service       FormService
	problemWriter *problem.HttpWriter
}

func NewMiddleware(logger *zap.Logger, service FormService, problemWriter *problem.HttpWriter) *Middleware {
	return &Middleware{
		logger:        logger,
		tracer:        otel.Tracer("form/middleware"),
		service:       service,
		problemWriter: problemWriter,
	}
}

func (m *Middleware) Require(formIDResolver resolver.FormIDResolver) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			m.CheckArchived(formIDResolver, next, w, r)
		}
	}
}

func (m *Middleware) CheckArchived(
	resolver resolver.FormIDResolver,
	next http.HandlerFunc,
	w http.ResponseWriter,
	r *http.Request,
) {
	traceCtx, span := m.tracer.Start(r.Context(), "CheckArchived")
	span.End()
	logger := logutil.WithContext(traceCtx, m.logger)

	formID, err := resolver.ResolveFormID(traceCtx, r)
	if err != nil {
		logger.Warn("resolve form id failed", zap.Error(err))
		m.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	archived, err := m.service.Archived(traceCtx, formID)
	if err != nil {
		m.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	if archived {
		m.problemWriter.WriteError(traceCtx, w, internal.ErrArchivedForm, logger)
		return
	}

	next(w, r)
}

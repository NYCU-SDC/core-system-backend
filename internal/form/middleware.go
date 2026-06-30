package form

import (
	"NYCU-SDC/core-system-backend/internal/auth/resolver"
	"context"
	"net/http"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type FormService interface {
	CheckAvailable(ctx context.Context, id uuid.UUID) error
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
			m.CheckAvailable(formIDResolver, next, w, r)
		}
	}
}

func (m *Middleware) CheckAvailable(
	resolver resolver.FormIDResolver,
	next http.HandlerFunc,
	w http.ResponseWriter,
	r *http.Request,
) {
	traceCtx, span := m.tracer.Start(r.Context(), "CheckAvailable")
	defer span.End()
	logger := logutil.WithContext(traceCtx, m.logger)

	formID, err := resolver.ResolveFormID(traceCtx, r)
	if err != nil {
		logger.Warn("resolve form id failed", zap.Error(err))
		m.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	err = m.service.CheckAvailable(traceCtx, formID)
	if err != nil {
		m.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	next(w, r)
}

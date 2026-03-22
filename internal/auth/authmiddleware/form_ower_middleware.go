package authmiddleware

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/auth/resolver"
	"context"
	"errors"
	"net/http"

	"NYCU-SDC/core-system-backend/internal/user"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type FormOwnerService interface {
	GetCreatorByFormID(ctx context.Context, formID uuid.UUID) (uuid.UUID, error)
}

type FormOwnerMiddleware struct {
	tracer        trace.Tracer
	logger        *zap.Logger
	service       FormOwnerService
	problemWriter *problem.HttpWriter
}

func NewFormOwnerMiddleware(
	service FormOwnerService,
	logger *zap.Logger,
	problemWriter *problem.HttpWriter,
) *FormOwnerMiddleware {

	return &FormOwnerMiddleware{
		tracer:        otel.Tracer("auth/middleware"),
		logger:        logger,
		service:       service,
		problemWriter: problemWriter,
	}
}

func (m *FormOwnerMiddleware) Require(
	resolver resolver.FormIdResolver,
) func(http.HandlerFunc) http.HandlerFunc {

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			m.checkOwner(resolver, next, w, r)
		}
	}
}

func (m *FormOwnerMiddleware) checkOwner(
	resolver resolver.FormIdResolver,
	next http.HandlerFunc,
	w http.ResponseWriter,
	r *http.Request,
) {
	traceCtx, span := m.tracer.Start(r.Context(), "FormOwnerMiddleware")
	defer span.End()
	logger := logutil.WithContext(traceCtx, m.logger)

	u, ok := user.GetFromContext(traceCtx)
	if !ok {
		m.problemWriter.WriteError(traceCtx, w, internal.ErrUnauthorizedError, logger)
		return
	}

	formID, err := resolver.ResolveFormID(traceCtx, r)
	if err != nil {
		logger.Warn("resolve form id failed", zap.Error(err))
		m.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	creatorID, err := m.service.GetCreatorByFormID(traceCtx, formID)
	if err != nil {
		if errors.Is(err, internal.ErrFormNotFound) {
			logger.Warn("form not found",
				zap.String("form_id", formID.String()),
			)

			m.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}

		logger.Error("failed to get form creator",
			zap.String("form_id", formID.String()),
			zap.Error(err),
		)

		m.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	if creatorID != u.ID {
		logger.Warn("permission denied (not form owner)",
			zap.String("user_id", u.ID.String()),
			zap.String("form_id", formID.String()),
			zap.String("creator_id", creatorID.String()),
		)

		m.problemWriter.WriteError(traceCtx, w, internal.ErrPermissionDenied, logger)
		return
	}

	next(w, r)
}

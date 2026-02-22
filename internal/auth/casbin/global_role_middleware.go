package casbin

import (
	"NYCU-SDC/core-system-backend/internal"
	"net/http"

	"NYCU-SDC/core-system-backend/internal/user"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/casbin/casbin/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type GlobalMiddleware struct {
	tracer        trace.Tracer
	logger        *zap.Logger
	enforcer      *casbin.Enforcer
	problemWriter *problem.HttpWriter
}

func NewGlobalMiddleware(
	logger *zap.Logger,
	problemWriter *problem.HttpWriter,
	enforcer *casbin.Enforcer,
) *GlobalMiddleware {
	return &GlobalMiddleware{
		tracer:        otel.Tracer("auth/middleware"),
		logger:        logger,
		enforcer:      enforcer,
		problemWriter: problemWriter,
	}
}

func (m *GlobalMiddleware) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		traceCtx, span := m.tracer.Start(r.Context(), "GlobalAuthMiddleware")
		defer span.End()
		logger := logutil.WithContext(traceCtx, m.logger)

		u, ok := user.GetFromContext(traceCtx)
		if !ok {
			m.problemWriter.WriteError(traceCtx, w, internal.ErrUnauthorizedError, logger)
			return
		}

		if len(u.Role) == 0 {
			m.problemWriter.WriteError(traceCtx, w, internal.ErrPermissionDenied, logger)
			return
		}

		userID := u.ID.String()
		domain := "global"

		for _, role := range u.Role {
			allowed, err := m.enforcer.Enforce(
				role,
				domain,
				r.URL.Path,
				r.Method,
			)

			if err != nil {
				logger.Error("casbin enforce error", zap.Error(err))
				m.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
				return
			}

			if allowed {
				next(w, r)
				return
			}
		}

		logger.Warn("global permission denied",
			zap.String("user_id", userID),
			zap.String("path", r.URL.Path),
			zap.String("method", r.Method),
		)

		m.problemWriter.WriteError(traceCtx, w, internal.ErrPermissionDenied, logger)
	}
}

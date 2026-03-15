package authmiddleware

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/auth"
	"net/http"

	"NYCU-SDC/core-system-backend/internal/user"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type GlobalRoleMiddleware struct {
	tracer        trace.Tracer
	logger        *zap.Logger
	problemWriter *problem.HttpWriter
}

func NewGlobalRoleMiddleware(
	logger *zap.Logger,
	problemWriter *problem.HttpWriter,
) *GlobalRoleMiddleware {

	return &GlobalRoleMiddleware{
		tracer:        otel.Tracer("auth/middleware"),
		logger:        logger,
		problemWriter: problemWriter,
	}
}

func (m *GlobalRoleMiddleware) Require(required auth.Role) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {

		return func(w http.ResponseWriter, r *http.Request) {

			traceCtx, span := m.tracer.Start(r.Context(), "GlobalRoleMiddleware")
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

			for _, roleStr := range u.Role {
				role, ok := auth.ParseRole(roleStr)
				if !ok {
					logger.Warn("invalid global role",
						zap.String("user_id", u.ID.String()),
						zap.String("role", roleStr),
					)
					continue
				}

				if role.Allow(required) {
					next(w, r)
					return
				}
			}

			logger.Warn("global permission denied",
				zap.String("user_id", u.ID.String()),
				zap.Strings("user_roles", u.Role),
				zap.String("required_role", required.String()),
				zap.String("path", r.URL.Path),
				zap.String("method", r.Method),
			)

			m.problemWriter.WriteError(traceCtx, w, internal.ErrPermissionDenied, logger)
		}
	}
}

package authmiddleware

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/auth"
	"NYCU-SDC/core-system-backend/internal/unit"
	"context"
	"net/http"

	"NYCU-SDC/core-system-backend/internal/user"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Resolver interface {
	ResolveUnitID(ctx context.Context, r *http.Request) (uuid.UUID, error)
}

type UnitRoleService interface {
	GetMemberRole(ctx context.Context, unitID uuid.UUID, memberID uuid.UUID) (unit.UnitRole, error)
}

type UnitRoleMiddleware struct {
	tracer        trace.Tracer
	logger        *zap.Logger
	service       UnitRoleService
	problemWriter *problem.HttpWriter
}

func NewUnitRoleMiddleware(
	service UnitRoleService,
	logger *zap.Logger,
	problemWriter *problem.HttpWriter,
) *UnitRoleMiddleware {

	return &UnitRoleMiddleware{
		tracer:        otel.Tracer("auth/middleware"),
		logger:        logger,
		service:       service,
		problemWriter: problemWriter,
	}
}

func (m *UnitRoleMiddleware) Require(
	required auth.Role,
	resolver Resolver,
) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {

		return func(w http.ResponseWriter, r *http.Request) {

			traceCtx, span := m.tracer.Start(r.Context(), "UnitRoleMiddleware")
			defer span.End()
			logger := logutil.WithContext(traceCtx, m.logger)

			u, ok := user.GetFromContext(traceCtx)
			if !ok {
				m.problemWriter.WriteError(traceCtx, w, internal.ErrUnauthorizedError, logger)
				return
			}

			unitID, err := resolver.ResolveUnitID(traceCtx, r)
			if err != nil {
				logger.Warn("resolve unit id failed", zap.Error(err))
				m.problemWriter.WriteError(traceCtx, w, err, logger)
				return
			}

			dbRole, err := m.service.GetMemberRole(traceCtx, unitID, u.ID)
			if err != nil {
				logger.Warn("permission denied (not unit member)",
					zap.String("user_id", u.ID.String()),
					zap.String("unit_id", unitID.String()),
				)

				m.problemWriter.WriteError(traceCtx, w, internal.ErrPermissionDenied, logger)
				return
			}

			role, ok := auth.ParseRole(string(dbRole))
			if !ok {
				logger.Warn("invalid role in database",
					zap.String("user_id", u.ID.String()),
					zap.String("unit_id", unitID.String()),
					zap.String("db_role", string(dbRole)),
				)

				m.problemWriter.WriteError(traceCtx, w, internal.ErrPermissionDenied, logger)
				return
			}

			if !role.Allow(required) {
				logger.Warn("permission denied",
					zap.String("user_id", u.ID.String()),
					zap.String("unit_id", unitID.String()),
					zap.String("required_role", required.String()),
					zap.String("user_role", role.String()),
				)

				m.problemWriter.WriteError(traceCtx, w, internal.ErrPermissionDenied, logger)
				return
			}

			next(w, r)
		}
	}
}

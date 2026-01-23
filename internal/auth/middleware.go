package auth

import (
	"context"
	"net/http"

	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/internal/user"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/casbin/casbin/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type unitReader interface {
	GetMemberRole(ctx context.Context, unitID uuid.UUID, memberID uuid.UUID) (unit.UnitRole, error)
}

type Middleware struct {
	tracer   trace.Tracer
	logger   *zap.Logger
	enforcer *casbin.Enforcer
	unitSvc  unitReader
}

func NewMiddleware(
	logger *zap.Logger,
	enforcer *casbin.Enforcer,
	unitSvc unitReader,
) *Middleware {
	return &Middleware{
		tracer:   otel.Tracer("auth/middleware"),
		logger:   logger,
		enforcer: enforcer,
		unitSvc:  unitSvc,
	}
}

func (m *Middleware) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		traceCtx, span := m.tracer.Start(r.Context(), "AuthMiddleware")
		defer span.End()
		logger := logutil.WithContext(traceCtx, m.logger)

		u, ok := user.GetFromContext(traceCtx)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		unitIDStr := r.PathValue("unitId")
		if unitIDStr == "" {
			next(w, r)
			return
		}

		unitID, err := uuid.Parse(unitIDStr)
		if err != nil {
			http.Error(w, "invalid unit id", http.StatusBadRequest)
			return
		}

		role, err := m.unitSvc.GetMemberRole(traceCtx, unitID, u.ID)
		if err != nil {
			logger.Warn("not unit member",
				zap.String("user_id", u.ID.String()),
				zap.String("unit_id", unitID.String()),
			)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		userID := u.ID.String()
		domain := unitID.String()

		_, _ = m.enforcer.DeleteRolesForUserInDomain(userID, domain)

		_, _ = m.enforcer.AddGroupingPolicy(
			userID,
			string(role), // admin / member
			domain,
		)
		
		allowed, err := m.enforcer.Enforce(
			userID,
			domain,
			r.URL.Path,
			r.Method,
		)
		if err != nil {
			logger.Error("casbin enforce error", zap.Error(err))
			http.Error(w, "internal error", 500)
			return
		}

		if !allowed {
			logger.Warn("permission denied",
				zap.String("user_id", userID),
				zap.String("unit_id", domain),
				zap.String("path", r.URL.Path),
				zap.String("method", r.Method),
				zap.String("role", string(role)),
			)
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

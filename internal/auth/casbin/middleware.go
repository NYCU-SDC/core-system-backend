package casbin

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"

	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/internal/user"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/casbin/casbin/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type unitReader interface {
	GetMemberRole(ctx context.Context, unitID uuid.UUID, memberID uuid.UUID) (unit.UnitRole, error)
}

type tenantReader interface {
	GetSlugStatus(ctx context.Context, slug string) (bool, uuid.UUID, error)
}

type formReader interface {
	GetUnitIDByFormID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	GetUnitIDBySectionID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

type Middleware struct {
	tracer        trace.Tracer
	logger        *zap.Logger
	enforcer      *casbin.Enforcer
	unitReader    unitReader
	tenantReader  tenantReader
	formReader    formReader
	problemWriter *problem.HttpWriter
}

func NewMiddleware(
	logger *zap.Logger,
	problemWriter *problem.HttpWriter,
	enforcer *casbin.Enforcer,
	unitReader unitReader,
	tenantReader tenantReader,
	formReader formReader,
) *Middleware {
	return &Middleware{
		tracer:        otel.Tracer("auth/middleware"),
		logger:        logger,
		enforcer:      enforcer,
		unitReader:    unitReader,
		tenantReader:  tenantReader,
		formReader:    formReader,
		problemWriter: problemWriter,
	}
}

func (m *Middleware) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		traceCtx, span := m.tracer.Start(r.Context(), "AuthMiddleware")
		defer span.End()
		logger := logutil.WithContext(traceCtx, m.logger)

		u, ok := user.GetFromContext(traceCtx)
		if !ok {
			m.problemWriter.WriteError(traceCtx, w, internal.ErrUnauthorizedError, logger)
			return
		}

		var unitID uuid.UUID
		unitIDStr := r.PathValue("unitId")
		slug := r.PathValue("slug")
		formIDStr := r.PathValue("formId")
		sectionIDStr := r.PathValue("sectionId")

		if unitIDStr != "" {
			// unit
			parsed, err := uuid.Parse(unitIDStr)
			if err != nil {
				logger.Error("get uuid failed", zap.Error(err))
				m.problemWriter.WriteError(traceCtx, w, internal.ErrValidationFailed, logger)
				return
			}
			unitID = parsed

		} else if slug != "" {
			// org
			exist, orgID, err := m.tenantReader.GetSlugStatus(traceCtx, slug)
			if err != nil {
				logger.Error("get slug status failed", zap.Error(err))
				m.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
				return
			}

			if !exist {
				logger.Warn("slug not exists", zap.String("slug", slug))
				m.problemWriter.WriteError(traceCtx, w, internal.ErrOrgSlugNotFound, logger)
				return
			}

			unitID = orgID

		} else if formIDStr != "" {
			formID, err := uuid.Parse(formIDStr)
			if err != nil {
				m.problemWriter.WriteError(traceCtx, w, internal.ErrValidationFailed, logger)
				return
			}

			unitID, err = m.formReader.GetUnitIDByFormID(traceCtx, formID)
			if err != nil {
				logger.Warn("form not found", zap.Error(err))
				m.problemWriter.WriteError(traceCtx, w, internal.ErrNotFound, logger)
				return
			}

		} else if sectionIDStr != "" {
			sectionID, err := uuid.Parse(sectionIDStr)
			if err != nil {
				m.problemWriter.WriteError(traceCtx, w, internal.ErrValidationFailed, logger)
				return
			}

			unitID, err = m.formReader.GetUnitIDBySectionID(traceCtx, sectionID)
			if err != nil {
				logger.Warn("section not found", zap.Error(err))
				m.problemWriter.WriteError(traceCtx, w, internal.ErrNotFound, logger)
				return
			}

		} else {
			next(w, r)
			return
		}

		role, err := m.unitReader.GetMemberRole(traceCtx, unitID, u.ID)
		if err != nil {
			logger.Warn("permission denied (not unit member)",
				zap.String("user_id", u.ID.String()),
				zap.String("unit_id", unitID.String()),
			)
			m.problemWriter.WriteError(traceCtx, w, internal.ErrPermissionDenied, logger)
			return
		}

		userID := u.ID.String()
		subject := string(role)
		domain := unitID.String()

		allowed, err := m.enforcer.Enforce(
			subject,
			domain,
			r.URL.Path,
			r.Method,
		)

		if err != nil {
			logger.Error("casbin enforce error", zap.Error(err))
			m.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
			return
		}

		if !allowed {
			logger.Warn("permission denied",
				zap.String("user_id", userID),
				zap.String("unit_id", domain),
				zap.String("path", r.URL.Path),
				zap.String("method", r.Method),
				zap.String("role", subject),
			)
			m.problemWriter.WriteError(traceCtx, w, internal.ErrPermissionDenied, logger)
			return
		}

		next(w, r)
	}
}

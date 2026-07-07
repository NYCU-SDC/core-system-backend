package tenant

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"net/http"
)

type reader interface {
	Get(ctx context.Context, id uuid.UUID) (Tenant, error)
	GetSlugStatus(ctx context.Context, slug string) (bool, uuid.UUID, error)
}

type Middleware struct {
	tracer        trace.Tracer
	logger        *zap.Logger
	masterDBPool  *pgxpool.Pool
	problemWriter *problem.HttpWriter
	reader        reader
}

func NewMiddleware(
	logger *zap.Logger,
	masterDBPool *pgxpool.Pool,
	problemWriter *problem.HttpWriter,
	reader reader,
) *Middleware {
	return &Middleware{
		tracer:        otel.Tracer("tenant/middleware"),
		logger:        logger,
		reader:        reader,
		masterDBPool:  masterDBPool,
		problemWriter: problemWriter,
	}
}

func (m *Middleware) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		traceCtx, span := m.tracer.Start(r.Context(), "TenantMiddleware")
		defer span.End()
		logger := logutil.WithContext(traceCtx, m.logger)

		slug := r.PathValue("slug")
		if slug == "" {
			logger.Error("User slug is empty", zap.String("path", r.URL.Path))
			m.problemWriter.WriteError(traceCtx, w, handlerutil.ErrInternalServer, logger)
			return
		}

		exists, orgID, err := m.reader.GetSlugStatus(traceCtx, slug)
		if err != nil {
			span.RecordError(err)
			m.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}
		if !exists {
			m.problemWriter.WriteError(traceCtx, w, internal.ErrOrgSlugNotFound, logger)
			return
		}

		tenant, err := m.reader.Get(traceCtx, orgID)
		if err != nil {
			span.RecordError(err)
			m.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}

		var conn DBTX
		if tenant.DbStrategy == DbStrategyShared {
			conn = m.masterDBPool
		} else {
			logger.Error("unsupported tenant database strategy", zap.String("strategy", string(tenant.DbStrategy)))
			m.problemWriter.WriteError(traceCtx, w, handlerutil.ErrInternalServer, logger)
			return
		}

		ctx := context.WithValue(traceCtx, internal.OrgIDContextKey, orgID)
		ctx = context.WithValue(ctx, internal.OrgSlugContextKey, slug)
		ctx = context.WithValue(ctx, internal.DBConnectionKey, conn)

		next(w, r.WithContext(ctx))
	}
}

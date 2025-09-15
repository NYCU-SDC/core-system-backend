package tenant

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
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
	GetBySlug(ctx context.Context, slug string) (Tenant, error)
}

type Middleware struct {
	tracer       trace.Tracer
	logger       *zap.Logger
	masterDBPool *pgxpool.Pool

	reader reader
}

func NewMiddleware(
	logger *zap.Logger,
	masterDBPool *pgxpool.Pool,

	reader reader,
) *Middleware {
	return &Middleware{
		tracer:       otel.Tracer("tenant/middleware"),
		logger:       logger,
		reader:       reader,
		masterDBPool: masterDBPool,
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
			problem.New().WriteError(traceCtx, w, handlerutil.ErrInternalServer, logger)
			return
		}

		org, err := m.reader.GetBySlug(traceCtx, slug)
		if err != nil {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "organizations", "slug", slug, logger, "get org ID by slug")
			span.RecordError(err)
			problem.New().WriteError(traceCtx, w, err, logger)
			return
		}

		tenant, err := m.reader.Get(traceCtx, org.ID)
		if err != nil {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "tenant", "id", org.ID.String(), logger, "get tenant by ID")
			span.RecordError(err)
			problem.New().WriteError(traceCtx, w, err, logger)
			return
		}

		var conn DBTX
		if tenant.DbStrategy == DbStrategyShared {
			conn = m.masterDBPool
		} else {
			logger.Error("unsupported tenant database strategy", zap.String("strategy", string(tenant.DbStrategy)))
			problem.New().WriteError(traceCtx, w, handlerutil.ErrInternalServer, logger)
			return
		}

		ctx := context.WithValue(traceCtx, internal.OrgIDContextKey, org.ID)
		ctx = context.WithValue(ctx, internal.OrgSlugContextKey, slug)
		ctx = context.WithValue(ctx, internal.DBConnectionKey, conn)

		next(w, r.WithContext(ctx))
	}
}

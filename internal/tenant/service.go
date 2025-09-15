package tenant

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"database/sql"
	"errors"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/jackc/pgx/v5/pgtype"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Create(ctx context.Context, param CreateParams) (Tenant, error)
	Get(ctx context.Context, id uuid.UUID) (Tenant, error)
	Update(ctx context.Context, param UpdateParams) (Tenant, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetBySlug(ctx context.Context, slug string) (Tenant, error)
}

type Service struct {
	logger *zap.Logger
	tracer trace.Tracer
	query  Querier
}

func NewService(logger *zap.Logger, db DBTX) *Service {
	return &Service{
		logger: logger,
		tracer: otel.Tracer("tenant/service"),
		query:  New(db),
	}
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Tenant, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := internal.WithContext(traceCtx, s.logger)

	tenant, err := s.query.Get(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "tenants", "id", id.String(), logger, "get tenant by id")
		span.RecordError(err)
		return Tenant{}, err
	}

	return tenant, nil
}

func (s *Service) Create(ctx context.Context, slug string, id uuid.UUID, ownerID uuid.UUID) (Tenant, error) {
	traceCtx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := internal.WithContext(traceCtx, s.logger)

	tenant, err := s.query.Create(traceCtx, CreateParams{
		ID:         id,
		Slug:       slug,
		DbStrategy: DbStrategyShared,
		OwnerID:    pgtype.UUID{Bytes: ownerID, Valid: true},
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "tenants", "id", id.String(), logger, "create tenant by id")
		span.RecordError(err)
		return Tenant{}, err
	}

	logger.Info("tenant created", zap.String("tenant_id", tenant.ID.String()), zap.String("db_strategy", string(tenant.DbStrategy)))

	return tenant, nil
}

func (s *Service) Update(ctx context.Context, param UpdateParams) (Tenant, error) {
	traceCtx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := internal.WithContext(traceCtx, s.logger)

	tenant, err := s.query.Update(traceCtx, param)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "tenants", "id", param.ID.String(), logger, "update tenant by id")
		span.RecordError(err)
		return Tenant{}, err
	}

	logger.Info("tenant updated", zap.String("tenant_id", tenant.ID.String()), zap.String("db_strategy", string(tenant.DbStrategy)))

	return tenant, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := internal.WithContext(traceCtx, s.logger)

	err := s.query.Delete(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "tenants", "id", id.String(), logger, "delete tenant by id")
		span.RecordError(err)
		return err
	}

	logger.Info("tenant deleted", zap.String("tenant_id", id.String()))

	return nil
}

func (s *Service) ValidateSlugUniqueness(ctx context.Context, slug string) (bool, error) {
	traceCtx, span := s.tracer.Start(ctx, "ValidateSlugUniqueness")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	org, err := s.query.GetBySlug(traceCtx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Slug is unique
			return true, nil
		}
		err = databaseutil.WrapDBError(err, logger, "validate slug uniqueness")
		span.RecordError(err)
		return false, err
	}

	// Slug already exists
	logger.Info("Slug already exists", zap.String("slug", slug), zap.String("org_id", org.ID.String()))
	return false, nil
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (Tenant, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetBySlug")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	org, err := s.query.GetBySlug(traceCtx, slug)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get organization id by slug")
		span.RecordError(err)
		return Tenant{}, err
	}

	return org, nil
}

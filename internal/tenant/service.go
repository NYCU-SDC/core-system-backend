package tenant

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"errors"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Create(ctx context.Context, param CreateParams) (Tenant, error)
	Get(ctx context.Context, id uuid.UUID) (Tenant, error)
	Update(ctx context.Context, param UpdateParams) (Tenant, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ExistsBySlug(ctx context.Context, slug string) (bool, error)
	GetSlugStatus(ctx context.Context, slug string) (pgtype.UUID, error)
	GetSlugHistory(ctx context.Context, slug string) ([]GetSlugHistoryRow, error)
	CreateSlugHistory(ctx context.Context, arg CreateSlugHistoryParams) (SlugHistory, error)
	UpdateSlugHistory(ctx context.Context, arg UpdateSlugHistoryParams) ([]pgtype.UUID, error)
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

func (s *Service) Create(ctx context.Context, id uuid.UUID, ownerID uuid.UUID, slug string) (Tenant, error) {
	traceCtx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := internal.WithContext(traceCtx, s.logger)

	tenant, err := s.query.Create(traceCtx, CreateParams{
		ID:         id,
		DbStrategy: DbStrategyShared,
		OwnerID:    pgtype.UUID{Bytes: ownerID, Valid: true},
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "tenants", "id", id.String(), logger, "create tenant by id")
		span.RecordError(err)
		return Tenant{}, err
	}
	logger.Info("tenant created", zap.String("tenant_id", tenant.ID.String()), zap.String("db_strategy", string(tenant.DbStrategy)))

	history, err := s.query.CreateSlugHistory(traceCtx, CreateSlugHistoryParams{
		Slug:  slug,
		OrgID: pgtype.UUID{Bytes: id, Valid: true},
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create history")
		span.RecordError(err)
		return Tenant{}, err
	}
	logger.Info("history created", zap.String("slug", slug), zap.Int32("history_id", history.ID))

	return tenant, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, slug string, dbStrategy DbStrategy) (Tenant, error) {
	traceCtx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := internal.WithContext(traceCtx, s.logger)

	tenant, err := s.query.Update(traceCtx, UpdateParams{
		ID:         id,
		DbStrategy: dbStrategy,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "tenants", "id", id.String(), logger, "update tenant by id")
		span.RecordError(err)
		return Tenant{}, err
	}
	logger.Info("tenant updated", zap.String("tenant_id", tenant.ID.String()), zap.String("db_strategy", string(tenant.DbStrategy)), zap.String("slug", slug))

	_, err = s.query.UpdateSlugHistory(traceCtx, UpdateSlugHistoryParams{
		Slug:  slug,
		OrgID: pgtype.UUID{Bytes: tenant.ID, Valid: true},
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "slug_history", "org_id", tenant.ID.String(), logger, "update slug history by org id")
		span.RecordError(err)
		return Tenant{}, err
	}

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

func (s *Service) SlugExists(ctx context.Context, slug string) (bool, error) {
	traceCtx, span := s.tracer.Start(ctx, "SlugExists")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exists, err := s.query.ExistsBySlug(traceCtx, slug)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "validate slug uniqueness")
		span.RecordError(err)
		return false, err
	}

	return exists, nil
}

func (s *Service) GetSlugStatusWithHistory(ctx context.Context, slug string) (bool, uuid.UUID, []GetSlugHistoryRow, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetSlugStatusWithHistory")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	history, err := s.query.GetSlugHistory(traceCtx, slug)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get slug history")
		span.RecordError(err)
		return true, uuid.UUID{}, nil, err
	}

	if len(history) > 0 && !history[0].EndedAt.Valid {
		return false, history[0].OrgID.Bytes, history, nil
	}

	return true, uuid.UUID{}, history, nil
}

func (s *Service) GetSlugStatus(ctx context.Context, slug string) (bool, uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetSlugStatus")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	orgID, err := s.query.GetSlugStatus(traceCtx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, uuid.UUID{}, nil
		}
		err = databaseutil.WrapDBError(err, logger, "get slug status")
		span.RecordError(err)
		return false, uuid.UUID{}, err
	}

	return true, orgID.Bytes, nil
}

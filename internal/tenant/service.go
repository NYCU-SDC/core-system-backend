package tenant

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"time"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/jackc/pgx/v5/pgtype"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	ExistsBySlug(ctx context.Context, slug string) (bool, error)
	Create(ctx context.Context, param CreateParams) (Tenant, error)
	Get(ctx context.Context, id uuid.UUID) (Tenant, error)
	Update(ctx context.Context, param UpdateParams) (Tenant, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetBySlug(ctx context.Context, slug string) (Tenant, error)
	GetHistory(ctx context.Context, slug string) ([]History, error)
	CreateHistory(ctx context.Context, arg CreateHistoryParams) (History, error)
	UpdateHistory(ctx context.Context, arg UpdateHistoryParams) (History, error)
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

func (s *Service) Create(ctx context.Context, slug string, id uuid.UUID, ownerID uuid.UUID, ownerName string) (Tenant, error) {
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

func (s *Service) Update(ctx context.Context, id uuid.UUID, slug string, dbStrategy DbStrategy) (Tenant, error) {
	traceCtx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := internal.WithContext(traceCtx, s.logger)

	tenant, err := s.query.Update(traceCtx, UpdateParams{
		ID:         id,
		Slug:       slug,
		DbStrategy: dbStrategy,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "tenants", "id", id.String(), logger, "update tenant by id")
		span.RecordError(err)
		return Tenant{}, err
	}

	logger.Info("tenant updated", zap.String("tenant_id", tenant.ID.String()), zap.String("db_strategy", string(tenant.DbStrategy)), zap.String("slug", slug))

	// current tenant stop using the slug
	_, err = s.query.UpdateHistory(traceCtx, UpdateHistoryParams{
		Slug:    slug,
		OrgID:   pgtype.UUID{Bytes: tenant.ID, Valid: true},
		EndedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
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

func deriveStatus(history []History) (bool, string) {
	for _, h := range history {
		if !h.EndedAt.Valid {
			return false, h.OrgID.String() // currently assigned
		}
	}
	return true, ""
}

func (s *Service) GetHistoryBySlug(ctx context.Context, slug string) ([]History, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetHistoryBySlug")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	history, err := s.query.GetHistory(traceCtx, slug)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get slug history")
		span.RecordError(err)
		return nil, err
	}

	return history, nil
}

func (s *Service) GetStatusWithHistory(ctx context.Context, slug string) (bool, uuid.UUID, []History, error) {
	history, err := s.GetHistoryBySlug(ctx, slug)
	if err != nil {
		return true, uuid.UUID{}, nil, err
	}

	available, currentOrgIDStr := deriveStatus(history)
	currentOrgID, err := uuid.Parse(currentOrgIDStr)
	if err != nil {
		return true, uuid.UUID{}, nil, err
	}

	return available, currentOrgID, history, nil
}

func (s *Service) GetStatus(ctx context.Context, slug string) (bool, uuid.UUID, error) {
	history, err := s.GetHistoryBySlug(ctx, slug)
	if err != nil {
		return true, uuid.UUID{}, err
	}

	available, currentOrgIDStr := deriveStatus(history)
	currentOrgID, err := uuid.Parse(currentOrgIDStr)
	if err != nil {
		return true, uuid.UUID{}, err
	}
	return available, currentOrgID, nil
}

func (s *Service) CreateHistory(ctx context.Context, slug string, orgID uuid.UUID, orgName string) (History, error) {
	traceCtx, span := s.tracer.Start(ctx, "CreateHistory")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	history, err := s.query.CreateHistory(traceCtx, CreateHistoryParams{
		Slug:      slug,
		OrgID:     pgtype.UUID{Bytes: orgID, Valid: true},
		Orgname:   pgtype.Text{String: orgName, Valid: true},
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		EndedAt:   pgtype.Timestamptz{Valid: false},
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create history")
		span.RecordError(err)
		return History{}, err
	}

	logger.Info("history created", zap.String("slug", slug), zap.String("org_id", orgID.String()), zap.String("org_name", orgName))

	return history, nil
}

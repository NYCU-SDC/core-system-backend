package form

import (
	"context"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Create(ctx context.Context, params CreateParams) (Form, error)
	Update(ctx context.Context, params UpdateParams) (Form, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (Form, error)
	List(ctx context.Context) ([]Form, error)
}

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer
}

func NewService(logger *zap.Logger, db DBTX) *Service {
	return &Service{
		logger:  logger,
		queries: New(db),
		tracer:  otel.Tracer("forms/service"),
	}
}

func (s *Service) Create(ctx context.Context, req Request, userID uuid.UUID) (Form, error) {
	ctx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	form, err := s.queries.Create(ctx, CreateParams{
		Title:       req.Title,
		Description: pgtype.Text{String: req.Description, Valid: true},
		LastEditor:  userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create form")
		span.RecordError(err)
		return Form{}, err
	}
	return form, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, request Request, userID uuid.UUID) (Form, error) {
	ctx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	form, err := s.queries.Update(ctx, UpdateParams{
		ID:          id,
		Title:       request.Title,
		Description: pgtype.Text{String: request.Description, Valid: true},
		LastEditor:  userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update form")
		span.RecordError(err)
		return Form{}, err
	}
	return form, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	err := s.queries.Delete(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "delete form")
		span.RecordError(err)
	}
	return err
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Form, error) {
	ctx, span := s.tracer.Start(ctx, "GetFormByID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	form, err := s.queries.GetByID(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get form by id")
		span.RecordError(err)
		return Form{}, err
	}
	return form, nil
}

func (s *Service) List(ctx context.Context) ([]Form, error) {
	ctx, span := s.tracer.Start(ctx, "ListForms")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)
	forms, err := s.queries.List(ctx)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list forms")
		span.RecordError(err)
		return nil, err
	}
	return forms, nil
}

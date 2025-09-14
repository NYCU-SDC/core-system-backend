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
	ListByUnit(ctx context.Context, unitID pgtype.UUID) ([]Form, error)
	SetStatus(ctx context.Context, arg SetStatusParams) (Form, error)
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

func (s *Service) Create(ctx context.Context, req Request, unitID uuid.UUID, userID uuid.UUID) (Form, error) {
	ctx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	newForm, err := s.queries.Create(ctx, CreateParams{
		Title:       req.Title,
		Description: pgtype.Text{String: req.Description, Valid: true},
		UnitID:      pgtype.UUID{Bytes: unitID, Valid: true},
		LastEditor:  userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create form")
		span.RecordError(err)
		return Form{}, err
	}

	return newForm, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, request Request, userID uuid.UUID) (Form, error) {
	ctx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	updatedForm, err := s.queries.Update(ctx, UpdateParams{
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

	return updatedForm, nil
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

	currentForm, err := s.queries.GetByID(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get form by id")
		span.RecordError(err)
		return Form{}, err
	}

	return currentForm, nil
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

func (s *Service) ListByUnit(ctx context.Context, unitID uuid.UUID) ([]Form, error) {
	ctx, span := s.tracer.Start(ctx, "ListByUnit")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	forms, err := s.queries.ListByUnit(ctx, pgtype.UUID{Bytes: unitID, Valid: true})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list forms by unit")
		span.RecordError(err)
		return nil, err
	}

	return forms, nil
}

func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status Status, userID uuid.UUID) (Form, error) {
	ctx, span := s.tracer.Start(ctx, "SetStatus")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	updated, err := s.queries.SetStatus(ctx, SetStatusParams{
		ID:         id,
		Status:     status,
		LastEditor: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "set form status")
		span.RecordError(err)
		return Form{}, err
	}

	return updated, nil
}

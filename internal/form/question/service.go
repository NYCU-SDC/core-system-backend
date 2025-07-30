package question

import (
	"context"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Create(ctx context.Context, params CreateParams) (Question, error)
	Update(ctx context.Context, params UpdateParams) (Question, error)
	Delete(ctx context.Context, params DeleteParams) error
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]Question, error)
	GetByID(ctx context.Context, id uuid.UUID) (Question, error)
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
		tracer:  otel.Tracer("question/service"),
	}
}

func (s *Service) Create(ctx context.Context, input CreateParams) (Question, error) {
	ctx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	q, err := s.queries.Create(ctx, input)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create question")
		span.RecordError(err)
		return Question{}, err
	}
	return q, nil
}

func (s *Service) Update(ctx context.Context, input UpdateParams) (Question, error) {
	ctx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	q, err := s.queries.Update(ctx, input)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update question")
		span.RecordError(err)
		return Question{}, err
	}
	return q, nil
}

func (s *Service) Delete(ctx context.Context, formID uuid.UUID, id uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	err := s.queries.Delete(ctx, DeleteParams{
		FormID: formID,
		ID:     id,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "delete question")
		span.RecordError(err)
	}
	return err
}

func (s *Service) ListByFormID(ctx context.Context, formID uuid.UUID) ([]Question, error) {
	ctx, span := s.tracer.Start(ctx, "ListByFormID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	list, err := s.queries.ListByFormID(ctx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list questions by form id")
		span.RecordError(err)
		return nil, err
	}
	return list, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Question, error) {
	ctx, span := s.tracer.Start(ctx, "GetByID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	q, err := s.queries.GetByID(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get question by id")
		span.RecordError(err)
		return Question{}, err
	}
	return q, nil
}

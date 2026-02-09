package question

import (
	"NYCU-SDC/core-system-backend/internal"
	"cmp"
	"context"
	"slices"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Create(ctx context.Context, params CreateParams) (CreateRow, error)
	Update(ctx context.Context, params UpdateParams) (UpdateRow, error)
	UpdateOrder(ctx context.Context, params UpdateOrderParams) (UpdateOrderRow, error)
	DeleteAndReorder(ctx context.Context, arg DeleteAndReorderParams) error
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]ListByFormIDRow, error)
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]GetByIDsRow, error)
}

type Answerable interface {
	Question() Question
	FormID() uuid.UUID
	Validate(value string) error
}

type SectionWithQuestions struct {
	Section   Section
	Questions []Answerable
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

func (s *Service) Create(ctx context.Context, input CreateParams) (Answerable, error) {
	ctx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	row, err := s.queries.Create(ctx, input)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create question")
		span.RecordError(err)
		return nil, err
	}

	return NewAnswerable(row.ToQuestion(), row.FormID)
}

func (s *Service) Update(ctx context.Context, input UpdateParams) (Answerable, error) {
	ctx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	row, err := s.queries.Update(ctx, input)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update question")
		span.RecordError(err)
		return nil, err
	}

	return NewAnswerable(row.ToQuestion(), row.FormID)
}

func (s *Service) UpdateOrder(ctx context.Context, input UpdateOrderParams) (Answerable, error) {
	ctx, span := s.tracer.Start(ctx, "UpdateOrder")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	row, err := s.queries.UpdateOrder(ctx, input)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update order for the questions")
		span.RecordError(err)
		return nil, err
	}

	return NewAnswerable(row.ToQuestion(), row.FormID)
}

func (s *Service) DeleteAndReorder(ctx context.Context, sectionID uuid.UUID, id uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "DeleteAndReorder")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	err := s.queries.DeleteAndReorder(ctx, DeleteAndReorderParams{
		SectionID: sectionID,
		ID:        id,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "delete and re-index remaining questions")
		span.RecordError(err)
		return err
	}

	return nil
}

func (s *Service) ListByFormID(ctx context.Context, formID uuid.UUID) ([]SectionWithQuestions, error) {
	ctx, span := s.tracer.Start(ctx, "ListByFormID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	list, err := s.queries.ListByFormID(ctx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list questions by form id")
		span.RecordError(err)
		return nil, err
	}

	sectionMap := make(map[uuid.UUID]*SectionWithQuestions)
	for _, row := range list {
		sectionID := row.SectionID

		_, exist := sectionMap[sectionID]
		if !exist {
			sectionMap[sectionID] = &SectionWithQuestions{
				Section: Section{
					ID:          sectionID,
					FormID:      row.FormID,
					Title:       row.Title,
					Description: row.Description,
					CreatedAt:   row.CreatedAt,
					UpdatedAt:   row.UpdatedAt,
				},
				Questions: []Answerable{},
			}
		}

		// Check if question exists
		if row.ID.Valid {
			q := Question{
				ID:          row.ID.Bytes,
				SectionID:   sectionID,
				Required:    row.Required.Bool,
				Type:        row.Type.QuestionType,
				Title:       row.QuestionTitle,
				Description: row.QuestionDescription,
				Metadata:    row.Metadata,
				Order:       row.Order.Int32,
				SourceID:    row.SourceID,
				CreatedAt:   row.QuestionCreatedAt,
				UpdatedAt:   row.QuestionUpdatedAt,
			}
			answerable, err := NewAnswerable(q, row.FormID)
			if err != nil {
				err = databaseutil.WrapDBError(err, logger, "create answerable from question")
				span.RecordError(err)
				return nil, err
			}

			sectionMap[sectionID].Questions = append(sectionMap[sectionID].Questions, answerable)
		}
	}

	result := make([]SectionWithQuestions, 0, len(sectionMap))
	for _, q := range sectionMap {
		result = append(result, *q)
	}

	slices.SortFunc(result, func(a, b SectionWithQuestions) int {
		return cmp.Compare(a.Section.ID.String(), b.Section.ID.String())
	})

	return result, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Answerable, error) {
	ctx, span := s.tracer.Start(ctx, "GetByID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	rows, err := s.queries.GetByIDs(ctx, []uuid.UUID{id})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get question by id")
		span.RecordError(err)
		return nil, err
	}

	if len(rows) == 0 {
		return nil, internal.ErrNotFound
	}

	row := rows[0]
	q := row.ToQuestion()
	return NewAnswerable(q, row.FormID)
}

func (s *Service) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]Answerable, error) {
	ctx, span := s.tracer.Start(ctx, "GetByIDs")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	rows, err := s.queries.GetByIDs(ctx, ids)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get question by an array of ids")
		span.RecordError(err)
		return nil, err
	}

	if len(rows) == 0 {
		return nil, internal.ErrNotFound
	}

	result := make([]Answerable, len(rows))
	for i, row := range rows {
		q := row.ToQuestion()
		answerable, err := NewAnswerable(q, row.FormID)
		if err != nil {
			span.RecordError(err)
			return nil, err
		}
		result[i] = answerable
	}

	return result, nil
}

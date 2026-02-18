package question

import (
	"cmp"
	"context"
	"encoding/json"
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
	ListSectionsByFormID(ctx context.Context, formID uuid.UUID) ([]Section, error)
	ListSectionsWithAnswersByFormID(ctx context.Context, formID uuid.UUID) ([]ListSectionsWithAnswersByFormIDRow, error)
	GetByID(ctx context.Context, id uuid.UUID) (GetByIDRow, error)
}

type Answerable interface {
	Question() Question
	FormID() uuid.UUID

	// Validate checks if the provided answer is valid according to the question's type and constraints.
	Validate(rawValue json.RawMessage) error

	// DisplayValue converts the answer to simple string for human to read
	DisplayValue(rawValue json.RawMessage) (string, error)

	// DecodeRequest decodes the raw JSON value from the request into the appropriate Go type based on the question type.
	DecodeRequest(rawValue json.RawMessage) (any, error)

	// DecodeStorage decodes the raw JSON value from the database into the appropriate Go type based on the question type.
	DecodeStorage(rawValue json.RawMessage) (any, error)

	// EncodeRequest encodes the Go value into raw JSON for storage in the database or for sending in a response, based on the question type.
	EncodeRequest(answer any) (json.RawMessage, error)

	// MatchesPattern checks if the answer matches the given regex pattern.
	// Used for workflow condition evaluation.
	// Returns false with error if the rawValue format is invalid (data corruption).
	// Returns false with nil if the pattern is invalid (logs error internally).
	MatchesPattern(rawValue json.RawMessage, pattern string) (bool, error)
}

type SectionWithAnswerableList struct {
	Section        Section
	AnswerableList []Answerable
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

func (s *Service) ListSections(ctx context.Context, formID uuid.UUID) (map[string]Section, error) {
	ctx, span := s.tracer.Start(ctx, "ListSections")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	list, err := s.queries.ListSectionsByFormID(ctx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list sections by form id")
		span.RecordError(err)
		return nil, err
	}

	sectionMap := make(map[string]Section)
	for _, section := range list {
		sectionMap[section.ID.String()] = section
	}

	return sectionMap, nil
}

func (s *Service) ListSectionsWithAnswersByFormID(ctx context.Context, formID uuid.UUID) ([]SectionWithAnswerableList, error) {
	ctx, span := s.tracer.Start(ctx, "ListSectionsWithAnswersByFormID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	list, err := s.queries.ListSectionsWithAnswersByFormID(ctx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list questions by form id")
		span.RecordError(err)
		return nil, err
	}

	sectionMap := make(map[uuid.UUID]*SectionWithAnswerableList)
	for _, row := range list {
		sectionID := row.SectionID

		_, exist := sectionMap[sectionID]
		if !exist {
			sectionMap[sectionID] = &SectionWithAnswerableList{
				Section: Section{
					ID:          sectionID,
					FormID:      row.FormID,
					Title:       row.Title,
					Description: row.Description,
					CreatedAt:   row.CreatedAt,
					UpdatedAt:   row.UpdatedAt,
				},
				AnswerableList: []Answerable{},
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

			sectionMap[sectionID].AnswerableList = append(sectionMap[sectionID].AnswerableList, answerable)
		}
	}

	result := make([]SectionWithAnswerableList, 0, len(sectionMap))
	for _, q := range sectionMap {
		result = append(result, *q)
	}

	slices.SortFunc(result, func(a, b SectionWithAnswerableList) int {
		return cmp.Compare(a.Section.ID.String(), b.Section.ID.String())
	})

	return result, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Answerable, error) {
	ctx, span := s.tracer.Start(ctx, "GetByID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	row, err := s.queries.GetByID(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get question by id")
		span.RecordError(err)
		return nil, err
	}

	q := row.ToQuestion()
	return NewAnswerable(q, row.FormID)
}

// GetAnswerableMapByFormID returns a map of question ID (as string) to Answerable for efficient lookups.
func (s *Service) GetAnswerableMapByFormID(ctx context.Context, formID uuid.UUID) (map[string]Answerable, error) {
	ctx, span := s.tracer.Start(ctx, "GetAnswerableMapByFormID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	list, err := s.queries.ListSectionsWithAnswersByFormID(ctx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list questions by form id")
		span.RecordError(err)
		return nil, err
	}

	answerableMap := make(map[string]Answerable)
	for _, row := range list {
		// Check if question exists
		if row.ID.Valid {
			q := Question{
				ID:          row.ID.Bytes,
				SectionID:   row.SectionID,
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

			answerableMap[q.ID.String()] = answerable
		}
	}

	return answerableMap, nil
}

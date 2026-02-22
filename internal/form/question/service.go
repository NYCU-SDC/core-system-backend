package question

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"

	"NYCU-SDC/core-system-backend/internal"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Create(ctx context.Context, params CreateParams) (CreateRow, error)
	Update(ctx context.Context, params UpdateParams) (UpdateRow, error)
	UpdateOrder(ctx context.Context, params UpdateOrderParams) (UpdateOrderRow, error)
	DeleteAndReorder(ctx context.Context, arg DeleteAndReorderParams) error
	SectionExists(ctx context.Context, id uuid.UUID) (bool, error)
	ListOrderBySectionID(ctx context.Context, sectionID uuid.UUID) ([]ListOrderBySectionIDRow, error)
	ListSectionsByFormID(ctx context.Context, formID uuid.UUID) ([]Section, error)
	ListSectionsWithAnswersByFormID(ctx context.Context, formID uuid.UUID) ([]ListSectionsWithAnswersByFormIDRow, error)
	GetByID(ctx context.Context, id uuid.UUID) (GetByIDRow, error)
	ListTypesByIDs(ctx context.Context, ids []uuid.UUID) ([]ListTypesByIDsRow, error)
	UpdateSection(ctx context.Context, arg UpdateSectionParams) (Section, error)
}

// FormStore is used to check form existence for operations that require it (e.g. list sections by form ID).
type FormStore interface {
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
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
	logger    *zap.Logger
	queries   Querier
	formStore FormStore
	tracer    trace.Tracer
}

func NewService(logger *zap.Logger, db DBTX, formStore FormStore) *Service {
	return &Service{
		logger:    logger,
		queries:   New(db),
		formStore: formStore,
		tracer:    otel.Tracer("question/service"),
	}
}

func (s *Service) Create(ctx context.Context, input CreateParams) (Answerable, error) {
	ctx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	exists, err := s.queries.SectionExists(ctx, input.SectionID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check section exists")
		span.RecordError(err)
		return nil, err
	}
	if !exists {
		return nil, internal.ErrSectionNotFound
	}

	orders, err := s.queries.ListOrderBySectionID(ctx, input.SectionID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list question orders")
		span.RecordError(err)
		return nil, err
	}
	count := len(orders)

	// Clamp requested order to [1, count+1]
	effectiveOrder := input.Order
	if effectiveOrder < 1 {
		effectiveOrder = 1
	}
	if effectiveOrder > int32(count+1) {
		effectiveOrder = int32(count + 1)
	}

	createInput := input
	createInput.Order = effectiveOrder

	// Insert in the middle: create at end then use UpdateOrder to place correctly
	if effectiveOrder <= int32(count) {
		createInput.Order = int32(count + 1)
	}
	row, err := s.queries.Create(ctx, createInput)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create question")
		span.RecordError(err)
		return nil, err
	}

	if effectiveOrder <= int32(count) {
		orderRow, err := s.queries.UpdateOrder(ctx, UpdateOrderParams{
			SectionID: input.SectionID,
			ID:        row.ID,
			Order:     effectiveOrder,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "update question order")
			span.RecordError(err)
			return nil, err
		}
		return NewAnswerable(orderRow.ToQuestion(), orderRow.FormID)
	}

	return NewAnswerable(row.ToQuestion(), row.FormID)
}

// Update updates question fields and, if order differs from the current order, updates order (clamped to [1, count]).
func (s *Service) Update(ctx context.Context, input UpdateParams, order int32) (Answerable, error) {
	ctx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	row, err := s.queries.Update(ctx, input)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update question")
		span.RecordError(err)
		return nil, err
	}

	if row.Order == order {
		answerable, err := NewAnswerable(row.ToQuestion(), row.FormID)
		if err != nil {
			return nil, err
		}
		return answerable, nil
	}

	orders, err := s.queries.ListOrderBySectionID(ctx, input.SectionID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list question orders")
		span.RecordError(err)
		return nil, err
	}

	count := len(orders)
	effectiveOrder := order
	if effectiveOrder < 1 {
		effectiveOrder = 1
	} else if effectiveOrder > int32(count) {
		effectiveOrder = int32(count)
	}

	orderRow, err := s.queries.UpdateOrder(ctx, UpdateOrderParams{
		SectionID: input.SectionID,
		ID:        input.ID,
		Order:     effectiveOrder,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update order for the questions")
		span.RecordError(err)
		return nil, err
	}
	return NewAnswerable(orderRow.ToQuestion(), orderRow.FormID)
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

	exists, err := s.formStore.Exists(ctx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check form exists")
		span.RecordError(err)
		return nil, err
	}
	if !exists {
		return nil, internal.ErrFormNotFound
	}

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

// ListTypesByIDs returns a map of question ID (as string) to QuestionType for the given IDs.
// This is a batch query to avoid N+1 queries when checking question types for multiple questions.
func (s *Service) ListTypesByIDs(ctx context.Context, ids []uuid.UUID) (map[string]QuestionType, error) {
	ctx, span := s.tracer.Start(ctx, "ListTypesByIDs")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	rows, err := s.queries.ListTypesByIDs(ctx, ids)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list question types by ids")
		span.RecordError(err)
		return nil, err
	}

	result := make(map[string]QuestionType, len(rows))
	for _, row := range rows {
		result[row.ID.String()] = row.Type
	}
	return result, nil
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

func (s *Service) UpdateSection(ctx context.Context, sectionID uuid.UUID, formID uuid.UUID, title string, description string) (Section, error) {
	ctx, span := s.tracer.Start(ctx, "UpdateSection")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	section, err := s.queries.UpdateSection(ctx, UpdateSectionParams{
		ID:          sectionID,
		Title:       pgtype.Text{String: title, Valid: true},
		Description: pgtype.Text{String: description, Valid: len(description) > 0},
		FormID:      formID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update section")
		span.RecordError(err)
		return Section{}, err
	}

	return section, nil
}

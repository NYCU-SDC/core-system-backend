package form

import (
	"context"
	"errors"
	"time"

	"NYCU-SDC/core-system-backend/internal"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	"github.com/jackc/pgx/v5"

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
	Patch(ctx context.Context, params PatchParams) (PatchRow, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Get(ctx context.Context, id uuid.UUID) (GetRow, error)
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]GetByIDsRow, error)
	List(ctx context.Context, arg ListParams) ([]ListRow, error)
	ListByUnit(ctx context.Context, arg ListByUnitParams) ([]ListByUnitRow, error)
	GetStatus(ctx context.Context, id uuid.UUID) (Status, error)
	SetStatus(ctx context.Context, arg SetStatusParams) (Form, error)
	UploadCoverImage(ctx context.Context, arg UploadCoverImageParams) (uuid.UUID, error)
	GetCoverImage(ctx context.Context, id uuid.UUID) ([]byte, error)
	GetUnitID(ctx context.Context, id uuid.UUID) (pgtype.UUID, error)
	GetUnitIDBySectionID(ctx context.Context, id uuid.UUID) (pgtype.UUID, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	GetCreator(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	GetIDBySectionID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

type UserFormStatus string

const (
	UserFormStatusNotStarted UserFormStatus = "NOT_STARTED"
	UserFormStatusInProgress UserFormStatus = "IN_PROGRESS"
	UserFormStatusCompleted  UserFormStatus = "COMPLETED"
)

type UserForm struct {
	FormID            uuid.UUID
	Title             string
	Deadline          pgtype.Timestamptz
	Status            UserFormStatus
	ResponseIDs       []uuid.UUID
	AllowEditResponse bool
}

type Service struct {
	logger        *zap.Logger
	queries       Querier
	tracer        trace.Tracer
	markdownStore MarkdownStore
}

type MarkdownStore interface {
	ProcessAPIText(ctx context.Context, raw []byte) (canonicalJSON []byte, cleanHTML string, err error)
	PreviewSnippet(ctx context.Context, raw []byte, maxRunes int) (string, error)
}

func NewService(logger *zap.Logger, db DBTX, markdownStore MarkdownStore) *Service {
	return &Service{
		logger:        logger,
		queries:       New(db),
		tracer:        otel.Tracer("forms/service"),
		markdownStore: markdownStore,
	}
}

func (s *Service) Create(ctx context.Context, request Request, unitID uuid.UUID, userID uuid.UUID) (CreateRow, error) {
	ctx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	fields, err := buildFormFieldsFromRequest(ctx, s.markdownStore, request)
	if err != nil {
		span.RecordError(err)
		return CreateRow{}, err
	}

	newForm, err := s.queries.Create(ctx, CreateParams{
		Title:                  fields.title,
		DescriptionJson:        fields.descriptionJSON,
		DescriptionHtml:        fields.descriptionHTML,
		PreviewMessage:         fields.previewMessage,
		UnitID:                 pgtype.UUID{Bytes: unitID, Valid: true},
		CreatedBy:              userID,
		LastEditor:             userID,
		Deadline:               fields.deadline,
		PublishTime:            fields.publishTime,
		MessageAfterSubmission: fields.messageAfterSubmission,
		GoogleSheetUrl:         fields.googleSheetURL,
		Visibility:             fields.visibility,
		DressingColor:          fields.dressingColor,
		DressingHeaderFont:     fields.dressingHeaderFont,
		DressingQuestionFont:   fields.dressingQuestionFont,
		DressingTextFont:       fields.dressingTextFont,
		AllowEditResponse:      fields.allowEditResponse,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create form")
		span.RecordError(err)
		return CreateRow{}, err
	}

	return newForm, nil
}

// PatchParams applies a form row patch built from sql-level params (used by workflow for last_editor sync, etc.).
func (s *Service) PatchParams(ctx context.Context, params PatchParams) (PatchRow, error) {
	ctx, span := s.tracer.Start(ctx, "PatchParams")
	defer span.End()

	logger := logutil.WithContext(ctx, s.logger)
	row, err := s.queries.Patch(ctx, params)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "patch form")
		span.RecordError(err)
		return PatchRow{}, err
	}

	return row, nil
}

func (s *Service) Patch(ctx context.Context, id uuid.UUID, request PatchRequest, userID uuid.UUID) (PatchRow, error) {
	ctx, span := s.tracer.Start(ctx, "Patch")
	defer span.End()

	params := PatchParams{
		ID:                     id,
		LastEditor:             userID,
		Title:                  optionalPtrText(request.Title),
		PreviewMessage:         optionalPtrText(request.PreviewMessage),
		Deadline:               optionalPtrTimestamptz(request.Deadline),
		PublishTime:            optionalPtrTimestamptz(request.PublishTime),
		MessageAfterSubmission: optionalPtrText(request.MessageAfterSubmission),
		GoogleSheetUrl:         optionalPtrText(request.GoogleSheetURL),
	}

	if request.Description.Set {
		descJSON, descHTML, err := s.markdownStore.ProcessAPIText(ctx, request.Description.Value)
		if err != nil {
			span.RecordError(err)
			return PatchRow{}, err
		}
		params.DescriptionJson = descJSON
		params.DescriptionHtml = pgtype.Text{String: descHTML, Valid: true}
	}

	v := request.Visibility
	if v != nil {
		params.Visibility = NullVisibility{
			Visibility: visibilityFromAPIFormat(*v),
			Valid:      true,
		}
	}

	d := request.Dressing
	if d != nil {
		params.DressingColor = nonEmptyText(d.Color)
		params.DressingHeaderFont = nonEmptyText(d.HeaderFont)
		params.DressingQuestionFont = nonEmptyText(d.QuestionFont)
		params.DressingTextFont = nonEmptyText(d.TextFont)
	}

	a := request.AllowEditResponse
	if a != nil {
		params.AllowEditResponse = pgtype.Bool{Bool: *a, Valid: true}
	}

	return s.PatchParams(ctx, params)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	err := s.queries.Delete(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "delete form")
		span.RecordError(err)
		return err
	}

	return nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (GetRow, error) {
	ctx, span := s.tracer.Start(ctx, "GetForm")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	currentForm, err := s.queries.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.RecordError(internal.ErrFormNotFound)
			return GetRow{}, internal.ErrFormNotFound
		}
		err = databaseutil.WrapDBError(err, logger, "get form by id")
		span.RecordError(err)
		return GetRow{}, err
	}

	return currentForm, nil
}

func (s *Service) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]GetByIDsRow, error) {
	ctx, span := s.tracer.Start(ctx, "GetFormsByIDs")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	forms, err := s.queries.GetByIDs(ctx, ids)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get forms by ids")
		span.RecordError(err)
		return nil, err
	}

	return forms, nil
}

// Exists reports whether a form with the given ID exists (so response package can use *form.Service as FormStore without form importing response).
func (s *Service) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	ctx, span := s.tracer.Start(ctx, "FormExists")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	exists, err := s.queries.Exists(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check form exists")
		span.RecordError(err)
		return false, err
	}

	return exists, nil
}

// List returns all forms matching the given filters.
// Pass an empty string for status or visibility to skip that filter.
// Set excludeExpired to true to exclude forms whose deadline has already passed (i.e. deadline >= now()).
func (s *Service) List(ctx context.Context, status Status, visibility Visibility, excludeExpired bool) ([]ListRow, error) {
	ctx, span := s.tracer.Start(ctx, "ListForms")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	params := ListParams{
		Status:     NullStatus{Status: status, Valid: status != ""},
		Visibility: NullVisibility{Visibility: visibility, Valid: visibility != ""},
	}

	if excludeExpired {
		params.DeadlineAfter = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}

	forms, err := s.queries.List(ctx, params)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list forms")
		span.RecordError(err)
		return []ListRow{}, err
	}

	return forms, nil
}

func (s *Service) ListByUnit(ctx context.Context, arg ListByUnitParams) ([]ListByUnitRow, error) {
	ctx, span := s.tracer.Start(ctx, "ListByUnit")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	forms, err := s.queries.ListByUnit(ctx, arg)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list forms by unit")
		span.RecordError(err)
		return []ListByUnitRow{}, err
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

func (s *Service) UploadCoverImage(ctx context.Context, formID uuid.UUID, imageData []byte, coverImageURL string) error {
	ctx, span := s.tracer.Start(ctx, "UploadCoverImage")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	_, err := s.queries.UploadCoverImage(ctx, UploadCoverImageParams{
		FormID:    formID,
		ImageData: imageData,
		CoverImageUrl: pgtype.Text{
			String: coverImageURL,
			Valid:  coverImageURL != "",
		},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return handlerutil.NewNotFoundError("forms", "id", formID.String(), "form not found")
		}
		err = databaseutil.WrapDBError(err, logger, "upload cover image")
		span.RecordError(err)
		return err
	}

	return nil
}

func (s *Service) GetCoverImage(ctx context.Context, id uuid.UUID) ([]byte, error) {
	ctx, span := s.tracer.Start(ctx, "GetCoverImage")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	imageData, err := s.queries.GetCoverImage(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get form cover image")
		span.RecordError(err)
		return nil, err
	}

	return imageData, nil
}

func (s *Service) GetUnitID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	ctx, span := s.tracer.Start(ctx, "GetUnitID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	unitID, err := s.queries.GetUnitID(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get unit id by form id")
		span.RecordError(err)
		return uuid.Nil, err
	}

	if !unitID.Valid {
		err := errors.New("unit id is null")
		logger.Error("invalid unit id", zap.Error(err))
		span.RecordError(err)
		return uuid.Nil, err
	}

	return unitID.Bytes, nil
}

func (s *Service) GetUnitIDBySectionID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	ctx, span := s.tracer.Start(ctx, "GetUnitIDBySectionID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	unitID, err := s.queries.GetUnitIDBySectionID(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get unit id by section id")
		span.RecordError(err)
		return uuid.Nil, err
	}

	if !unitID.Valid {
		err := errors.New("unit id is null")
		logger.Error("invalid unit id", zap.Error(err))
		span.RecordError(err)
		return uuid.Nil, err
	}

	return unitID.Bytes, nil
}

func (s *Service) GetCreator(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	ctx, span := s.tracer.Start(ctx, "GetCreator")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	creatorID, err := s.queries.GetCreator(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.RecordError(internal.ErrFormNotFound)
			return uuid.Nil, internal.ErrFormNotFound
		}

		err = databaseutil.WrapDBError(err, logger, "get creator by form id")
		span.RecordError(err)
		return uuid.Nil, err
	}

	return creatorID, nil
}

func (s *Service) GetIDBySectionID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	ctx, span := s.tracer.Start(ctx, "GetFormIDBySectionID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	formID, err := s.queries.GetIDBySectionID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.RecordError(internal.ErrSectionNotFound)
			return uuid.Nil, internal.ErrSectionNotFound
		}

		err = databaseutil.WrapDBError(err, logger, "get form id by section id")
		span.RecordError(err)
		return uuid.Nil, err
	}

	return formID, nil
}

// IsArchived check if the form is archived, which should not allow any response
func (s *Service) IsArchived(ctx context.Context, id uuid.UUID) (bool, error) {
	traceCtx, span := s.tracer.Start(ctx, "IsArchived")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	status, err := s.queries.GetStatus(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get status")
		return false, err
	}

	return status == StatusArchived, nil
}

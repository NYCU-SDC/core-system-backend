package form

import (
	"context"
	"errors"
	"time"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/markdown"

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
	GetByID(ctx context.Context, id uuid.UUID) (GetByIDRow, error)
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]GetByIDsRow, error)
	List(ctx context.Context, arg ListParams) ([]ListRow, error)
	ListByUnit(ctx context.Context, arg ListByUnitParams) ([]ListByUnitRow, error)
	SetStatus(ctx context.Context, arg SetStatusParams) (Form, error)
	UploadCoverImage(ctx context.Context, arg UploadCoverImageParams) (uuid.UUID, error)
	GetCoverImage(ctx context.Context, id uuid.UUID) ([]byte, error)
	GetUnitIDByID(ctx context.Context, id uuid.UUID) (pgtype.UUID, error)
	GetUnitIDBySectionID(ctx context.Context, id uuid.UUID) (pgtype.UUID, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	GetCreatorByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	GetIDBySectionID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

type UserFormStatus string

const (
	UserFormStatusNotStarted UserFormStatus = "NOT_STARTED"
	UserFormStatusInProgress UserFormStatus = "IN_PROGRESS"
	UserFormStatusCompleted  UserFormStatus = "COMPLETED"
)

type UserForm struct {
	FormID   uuid.UUID
	Title    string
	Deadline pgtype.Timestamptz
	Status   UserFormStatus
}

type formFields struct {
	title                  string
	descriptionJSON        []byte
	descriptionHTML        string
	previewMessage         pgtype.Text
	deadline               pgtype.Timestamptz
	publishTime            pgtype.Timestamptz
	googleSheetURL         pgtype.Text
	messageAfterSubmission string
	visibility             Visibility
	dressingColor          pgtype.Text
	dressingHeaderFont     pgtype.Text
	dressingQuestionFont   pgtype.Text
	dressingTextFont       pgtype.Text
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

// visibilityFromAPIFormat converts API visibility format (uppercase) to database format (lowercase).
func visibilityFromAPIFormat(v string) Visibility {
	switch v {
	case "PUBLIC":
		return VisibilityPublic
	case "PRIVATE":
		return VisibilityPrivate
	default:
		// Fallback for backward compatibility
		return Visibility(v)
	}
}

func buildFormFieldsFromRequest(request Request) (formFields, error) {
	form := formFields{}

	if request.Deadline != nil {
		form.deadline = pgtype.Timestamptz{Time: *request.Deadline, Valid: true}
	} else {
		form.deadline = pgtype.Timestamptz{Valid: false}
	}

	if request.PublishTime != nil {
		form.publishTime = pgtype.Timestamptz{Time: *request.PublishTime, Valid: true}
	} else {
		form.publishTime = pgtype.Timestamptz{Valid: false}
	}

	descJSON, descHTML, err := markdown.ProcessAPIInput([]byte(request.Description))
	if err != nil {
		return formFields{}, err
	}
	form.descriptionJSON = descJSON
	form.descriptionHTML = descHTML

	preview := request.PreviewMessage
	if preview == "" {
		snip, err := markdown.PreviewSnippet(descJSON, 25)
		if err != nil {
			return formFields{}, err
		}
		preview = snip
	}

	if request.Dressing != nil {
		form.dressingColor = pgtype.Text{String: request.Dressing.Color, Valid: request.Dressing.Color != ""}
		form.dressingHeaderFont = pgtype.Text{String: request.Dressing.HeaderFont, Valid: request.Dressing.HeaderFont != ""}
		form.dressingQuestionFont = pgtype.Text{String: request.Dressing.QuestionFont, Valid: request.Dressing.QuestionFont != ""}
		form.dressingTextFont = pgtype.Text{String: request.Dressing.TextFont, Valid: request.Dressing.TextFont != ""}
	} else {
		form.dressingColor = pgtype.Text{Valid: false}
		form.dressingHeaderFont = pgtype.Text{Valid: false}
		form.dressingQuestionFont = pgtype.Text{Valid: false}
		form.dressingTextFont = pgtype.Text{Valid: false}
	}

	form.previewMessage = pgtype.Text{String: preview, Valid: preview != ""}
	form.googleSheetURL = pgtype.Text{String: request.GoogleSheetUrl, Valid: request.GoogleSheetUrl != ""}
	form.messageAfterSubmission = request.MessageAfterSubmission
	form.visibility = visibilityFromAPIFormat(request.Visibility)
	form.title = request.Title

	return form, nil
}

func (s *Service) Create(ctx context.Context, request Request, unitID uuid.UUID, userID uuid.UUID) (CreateRow, error) {
	ctx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	fields, err := buildFormFieldsFromRequest(request)
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
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create form")
		span.RecordError(err)
		return CreateRow{}, err
	}

	return newForm, nil
}

func (s *Service) Patch(ctx context.Context, id uuid.UUID, request PatchRequest, userID uuid.UUID) (PatchRow, error) {
	ctx, span := s.tracer.Start(ctx, "Patch")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	params := PatchParams{
		ID:         id,
		LastEditor: userID,
	}

	if request.Title != nil {
		params.Title = pgtype.Text{String: *request.Title, Valid: true}
	}

	if request.Description != nil {
		j, h, err := markdown.ProcessAPIInput([]byte(*request.Description))
		if err != nil {
			span.RecordError(err)
			return PatchRow{}, err
		}
		params.DescriptionJson = j
		params.DescriptionHtml = pgtype.Text{String: h, Valid: true}
	}

	if request.PreviewMessage != nil {
		params.PreviewMessage = pgtype.Text{String: *request.PreviewMessage, Valid: true}
	}

	if request.Deadline != nil {
		params.Deadline = pgtype.Timestamptz{Time: *request.Deadline, Valid: true}
	}

	if request.PublishTime != nil {
		params.PublishTime = pgtype.Timestamptz{Time: *request.PublishTime, Valid: true}
	}

	if request.MessageAfterSubmission != nil {
		params.MessageAfterSubmission = pgtype.Text{String: *request.MessageAfterSubmission, Valid: true}
	}

	if request.GoogleSheetUrl != nil {
		params.GoogleSheetUrl = pgtype.Text{String: *request.GoogleSheetUrl, Valid: true}
	}

	if request.Visibility != nil {
		params.Visibility = NullVisibility{
			Visibility: visibilityFromAPIFormat(*request.Visibility),
			Valid:      true,
		}
	}

	if request.Dressing != nil {
		if request.Dressing.Color != "" {
			params.DressingColor = pgtype.Text{String: request.Dressing.Color, Valid: true}
		}
		if request.Dressing.HeaderFont != "" {
			params.DressingHeaderFont = pgtype.Text{String: request.Dressing.HeaderFont, Valid: true}
		}
		if request.Dressing.QuestionFont != "" {
			params.DressingQuestionFont = pgtype.Text{String: request.Dressing.QuestionFont, Valid: true}
		}
		if request.Dressing.TextFont != "" {
			params.DressingTextFont = pgtype.Text{String: request.Dressing.TextFont, Valid: true}
		}
	}

	patchedForm, err := s.queries.Patch(ctx, params)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "patch form")
		span.RecordError(err)
		return PatchRow{}, err
	}

	return patchedForm, nil
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

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (GetByIDRow, error) {
	ctx, span := s.tracer.Start(ctx, "GetFormByID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	currentForm, err := s.queries.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.RecordError(internal.ErrFormNotFound)
			return GetByIDRow{}, internal.ErrFormNotFound
		}
		err = databaseutil.WrapDBError(err, logger, "get form by id")
		span.RecordError(err)
		return GetByIDRow{}, err
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

func (s *Service) GetUnitIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	ctx, span := s.tracer.Start(ctx, "GetUnitIDByFormID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	unitID, err := s.queries.GetUnitIDByID(ctx, id)
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

func (s *Service) GetCreatorByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	ctx, span := s.tracer.Start(ctx, "GetCreatorByFormID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	creatorID, err := s.queries.GetCreatorByID(ctx, id)
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

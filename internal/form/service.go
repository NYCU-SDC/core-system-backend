package form

import (
	"NYCU-SDC/core-system-backend/internal/form/response"
	"context"
	"errors"
	"slices"

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
	List(ctx context.Context, includeArchived pgtype.Bool) ([]ListRow, error)
	ListByUnit(ctx context.Context, arg ListByUnitParams) ([]ListByUnitRow, error)
	SetStatus(ctx context.Context, arg SetStatusParams) (Form, error)
	UploadCoverImage(ctx context.Context, arg UploadCoverImageParams) (uuid.UUID, error)
	GetCoverImage(ctx context.Context, id uuid.UUID) ([]byte, error)
}

type ResponseStore interface {
	ListBySubmittedBy(ctx context.Context, submittedBy uuid.UUID) ([]response.FormResponse, error)
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
	description            pgtype.Text
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
	logger        *zap.Logger
	queries       Querier
	tracer        trace.Tracer
	responseStore ResponseStore
}

func NewService(logger *zap.Logger, db DBTX, responseStore ResponseStore) *Service {
	return &Service{
		logger:        logger,
		queries:       New(db),
		tracer:        otel.Tracer("forms/service"),
		responseStore: responseStore,
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

func buildFormFieldsFromRequest(request Request) formFields {
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

	preview := request.PreviewMessage
	if preview == "" && request.Description != "" {
		runes := []rune(request.Description)
		if len(runes) > 25 {
			preview = string(runes[:25])
		} else {
			preview = request.Description
		}
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

	form.description = pgtype.Text{String: request.Description, Valid: true}
	form.previewMessage = pgtype.Text{String: preview, Valid: preview != ""}
	form.googleSheetURL = pgtype.Text{String: request.GoogleSheetUrl, Valid: request.GoogleSheetUrl != ""}
	form.messageAfterSubmission = request.MessageAfterSubmission
	form.visibility = visibilityFromAPIFormat(request.Visibility)
	form.title = request.Title

	return form
}

func (s *Service) Create(ctx context.Context, request Request, unitID uuid.UUID, userID uuid.UUID) (CreateRow, error) {
	ctx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	fields := buildFormFieldsFromRequest(request)

	newForm, err := s.queries.Create(ctx, CreateParams{
		Title:                  fields.title,
		Description:            fields.description,
		PreviewMessage:         fields.previewMessage,
		UnitID:                 pgtype.UUID{Bytes: unitID, Valid: true},
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
		params.Description = pgtype.Text{String: *request.Description, Valid: true}
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
		err = databaseutil.WrapDBError(err, logger, "get form by id")
		span.RecordError(err)
		return GetByIDRow{}, err
	}

	return currentForm, nil
}

func (s *Service) List(ctx context.Context) ([]ListRow, error) {
	ctx, span := s.tracer.Start(ctx, "ListForms")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	forms, err := s.queries.List(ctx, pgtype.Bool{Valid: false})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list forms")
		span.RecordError(err)
		return []ListRow{}, err
	}

	return forms, nil
}

func (s *Service) ListByUnit(ctx context.Context, unitID uuid.UUID) ([]ListByUnitRow, error) {
	ctx, span := s.tracer.Start(ctx, "ListByUnit")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	forms, err := s.queries.ListByUnit(ctx, ListByUnitParams{
		UnitID: pgtype.UUID{Bytes: unitID, Valid: true},
	})
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

func (s *Service) ListFormsOfUser(ctx context.Context, unitIDs []uuid.UUID, userID uuid.UUID) ([]UserForm, error) {
	ctx, span := s.tracer.Start(ctx, "ListFormsOfUser")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	responses, err := s.responseStore.ListBySubmittedBy(ctx, userID)
	if err != nil {
		span.RecordError(err)
		return []UserForm{}, err
	}

	formStatusMap := make(map[uuid.UUID]UserFormStatus)
	for _, response := range responses {
		status := UserFormStatusInProgress
		if response.SubmittedAt.Valid {
			status = UserFormStatusCompleted
		}
		formStatusMap[response.FormID] = status
	}

	allForms := make(map[uuid.UUID]ListByUnitRow)
	for _, unitID := range unitIDs {
		forms, err := s.queries.ListByUnit(ctx, ListByUnitParams{
			UnitID: pgtype.UUID{Bytes: unitID, Valid: true},
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "list forms by unit")
			span.RecordError(err)
			return []UserForm{}, err
		}

		for _, form := range forms {
			allForms[form.ID] = form
		}
	}

	userForms := make([]UserForm, 0, len(allForms))
	for formID, form := range allForms {
		status, exists := formStatusMap[formID]
		if !exists {
			status = UserFormStatusNotStarted
		}

		userForms = append(userForms, UserForm{
			FormID:   formID,
			Title:    form.Title,
			Deadline: form.Deadline,
			Status:   status,
		})
	}

	slices.SortFunc(userForms, func(a, b UserForm) int {

		if a.Deadline.Valid != b.Deadline.Valid {
			if a.Deadline.Valid {
				return -1
			}
			return 1
		}

		if a.Deadline.Valid {
			if n := a.Deadline.Time.Compare(b.Deadline.Time); n != 0 {
				return n
			}
		}

		if a.Title < b.Title {
			return -1
		}
		if a.Title > b.Title {
			return 1
		}

		return 0
	})

	return userForms, nil
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

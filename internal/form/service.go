package form

import (
	"context"
	"errors"
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
	Update(ctx context.Context, params UpdateParams) (UpdateRow, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (GetByIDRow, error)
	List(ctx context.Context) ([]ListRow, error)
	ListByUnit(ctx context.Context, unitID pgtype.UUID) ([]ListByUnitRow, error)
	SetStatus(ctx context.Context, arg SetStatusParams) (Form, error)
	UploadCoverImage(ctx context.Context, arg UploadCoverImageParams) (uuid.UUID, error)
	GetCoverImage(ctx context.Context, id uuid.UUID) ([]byte, error)
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

func (s *Service) Create(ctx context.Context, request Request, unitID uuid.UUID, userID uuid.UUID) (CreateRow, error) {
	ctx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	var deadline pgtype.Timestamptz
	if request.Deadline != nil {
		deadline = pgtype.Timestamptz{Time: *request.Deadline, Valid: true}
	} else {
		deadline = pgtype.Timestamptz{Valid: false}
	}

	var publishTime pgtype.Timestamptz
	if request.PublishTime != nil {
		publishTime = pgtype.Timestamptz{Time: *request.PublishTime, Valid: true}
	} else {
		publishTime = pgtype.Timestamptz{Valid: false}
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

	var dressingColor, dressingHeaderFont, dressingQuestionFont, dressingTextFont pgtype.Text
	if request.Dressing != nil {
		dressingColor = pgtype.Text{String: request.Dressing.Color, Valid: request.Dressing.Color != ""}
		dressingHeaderFont = pgtype.Text{String: request.Dressing.HeaderFont, Valid: request.Dressing.HeaderFont != ""}
		dressingQuestionFont = pgtype.Text{String: request.Dressing.QuestionFont, Valid: request.Dressing.QuestionFont != ""}
		dressingTextFont = pgtype.Text{String: request.Dressing.TextFont, Valid: request.Dressing.TextFont != ""}
	} else {
		dressingColor = pgtype.Text{Valid: false}
		dressingHeaderFont = pgtype.Text{Valid: false}
		dressingQuestionFont = pgtype.Text{Valid: false}
		dressingTextFont = pgtype.Text{Valid: false}
	}

	newForm, err := s.queries.Create(ctx, CreateParams{
		Title:                  request.Title,
		Description:            pgtype.Text{String: request.Description, Valid: true},
		PreviewMessage:         pgtype.Text{String: preview, Valid: preview != ""},
		UnitID:                 pgtype.UUID{Bytes: unitID, Valid: true},
		LastEditor:             userID,
		Deadline:               deadline,
		PublishTime:            publishTime,
		MessageAfterSubmission: request.MessageAfterSubmission,
		GoogleSheetUrl:         pgtype.Text{String: request.GoogleSheetUrl, Valid: request.GoogleSheetUrl != ""},
		Visibility:             request.Visibility,
		DressingColor:          dressingColor,
		DressingHeaderFont:     dressingHeaderFont,
		DressingQuestionFont:   dressingQuestionFont,
		DressingTextFont:       dressingTextFont,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create form")
		span.RecordError(err)
		return CreateRow{}, err
	}

	return newForm, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, request Request, userID uuid.UUID) (UpdateRow, error) {
	ctx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	var deadline pgtype.Timestamptz
	if request.Deadline != nil {
		deadline = pgtype.Timestamptz{Time: *request.Deadline, Valid: true}
	} else {
		deadline = pgtype.Timestamptz{Valid: false}
	}

	var publishTime pgtype.Timestamptz
	if request.PublishTime != nil {
		publishTime = pgtype.Timestamptz{Time: *request.PublishTime, Valid: true}
	} else {
		publishTime = pgtype.Timestamptz{Valid: false}
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

	var dressingColor, dressingHeaderFont, dressingQuestionFont, dressingTextFont pgtype.Text
	if request.Dressing != nil {
		dressingColor = pgtype.Text{String: request.Dressing.Color, Valid: request.Dressing.Color != ""}
		dressingHeaderFont = pgtype.Text{String: request.Dressing.HeaderFont, Valid: request.Dressing.HeaderFont != ""}
		dressingQuestionFont = pgtype.Text{String: request.Dressing.QuestionFont, Valid: request.Dressing.QuestionFont != ""}
		dressingTextFont = pgtype.Text{String: request.Dressing.TextFont, Valid: request.Dressing.TextFont != ""}
	} else {
		dressingColor = pgtype.Text{Valid: false}
		dressingHeaderFont = pgtype.Text{Valid: false}
		dressingQuestionFont = pgtype.Text{Valid: false}
		dressingTextFont = pgtype.Text{Valid: false}
	}

	updatedForm, err := s.queries.Update(ctx, UpdateParams{
		ID:                     id,
		Title:                  request.Title,
		Description:            pgtype.Text{String: request.Description, Valid: true},
		PreviewMessage:         pgtype.Text{String: preview, Valid: preview != ""},
		LastEditor:             userID,
		Deadline:               deadline,
		PublishTime:            publishTime,
		MessageAfterSubmission: request.MessageAfterSubmission,
		GoogleSheetUrl:         pgtype.Text{String: request.GoogleSheetUrl, Valid: request.GoogleSheetUrl != ""},
		Visibility:             request.Visibility,
		DressingColor:          dressingColor,
		DressingHeaderFont:     dressingHeaderFont,
		DressingQuestionFont:   dressingQuestionFont,
		DressingTextFont:       dressingTextFont,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update form")
		span.RecordError(err)
		return UpdateRow{}, err
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

	forms, err := s.queries.List(ctx)
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

	forms, err := s.queries.ListByUnit(ctx, pgtype.UUID{Bytes: unitID, Valid: true})
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
		err = databaseutil.WrapDBError(err, logger, "check form existence")
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

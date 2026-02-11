package form

import (
	"NYCU-SDC/core-system-backend/internal/form/response"
	"context"
	"slices"

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

func (s *Service) Create(ctx context.Context, req Request, unitID uuid.UUID, userID uuid.UUID) (CreateRow, error) {
	ctx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	var deadline pgtype.Timestamptz
	if req.Deadline != nil {
		deadline = pgtype.Timestamptz{Time: *req.Deadline, Valid: true}
	} else {
		deadline = pgtype.Timestamptz{Valid: false}
	}

	dbParams := map[string]interface{}{
		"title":           req.Title,
		"description":     req.Description,
		"preview_message": req.PreviewMessage,
		"unit_id":         unitID.String(),
		"last_editor":     userID.String(),
	}
	tracker := logutil.StartDBOperation(ctx, logger, "Create", dbParams)

	newForm, err := s.queries.Create(ctx, CreateParams{
		Title:          req.Title,
		Description:    pgtype.Text{String: req.Description, Valid: true},
		PreviewMessage: pgtype.Text{String: req.PreviewMessage, Valid: req.PreviewMessage != ""},
		UnitID:         pgtype.UUID{Bytes: unitID, Valid: true},
		LastEditor:     userID,
		Deadline:       deadline,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithTracker(err, tracker, "create form")
		span.RecordError(err)
		return CreateRow{}, err
	}

	tracker.SuccessWrite(newForm.ID.String())

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

	dbParams := map[string]interface{}{
		"id":              id.String(),
		"title":           request.Title,
		"description":     request.Description,
		"preview_message": request.PreviewMessage,
		"last_editor":     userID.String(),
	}
	tracker := logutil.StartDBOperation(ctx, logger, "Update", dbParams)

	updatedForm, err := s.queries.Update(ctx, UpdateParams{
		ID:             id,
		Title:          request.Title,
		Description:    pgtype.Text{String: request.Description, Valid: true},
		PreviewMessage: pgtype.Text{String: request.PreviewMessage, Valid: request.PreviewMessage != ""},
		LastEditor:     userID,
		Deadline:       deadline,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithTracker(err, tracker, "update form")
		span.RecordError(err)
		return UpdateRow{}, err
	}

	tracker.SuccessWrite(id.String())

	return updatedForm, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	dbParams := map[string]interface{}{
		"id": id.String(),
	}
	tracker := logutil.StartDBOperation(ctx, logger, "Delete", dbParams)

	err := s.queries.Delete(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithTracker(err, tracker, "delete form")
		span.RecordError(err)
		return err
	}
	tracker.SuccessWrite(id.String())

	return nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (GetByIDRow, error) {
	ctx, span := s.tracer.Start(ctx, "GetByID")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	dbParams := map[string]interface{}{
		"id": id.String(),
	}
	tracker := logutil.StartDBOperation(ctx, logger, "GetByID", dbParams)

	currentForm, err := s.queries.GetByID(ctx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithTracker(err, tracker, "get form by id")
		span.RecordError(err)
		return GetByIDRow{}, err
	}

	tracker.SuccessRead(1, id.String())

	return currentForm, nil
}

func (s *Service) List(ctx context.Context) ([]ListRow, error) {
	ctx, span := s.tracer.Start(ctx, "ListForms")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	tracker := logutil.StartDBOperation(ctx, logger, "List", nil)

	forms, err := s.queries.List(ctx)
	if err != nil {
		err = databaseutil.WrapDBErrorWithTracker(err, tracker, "list forms")
		span.RecordError(err)
		return []ListRow{}, err
	}

	tracker.SuccessRead(len(forms), "")

	return forms, nil
}

func (s *Service) ListByUnit(ctx context.Context, unitID uuid.UUID) ([]ListByUnitRow, error) {
	ctx, span := s.tracer.Start(ctx, "ListByUnit")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	dbParams := map[string]interface{}{
		"unit_id": unitID.String(),
	}
	tracker := logutil.StartDBOperation(ctx, logger, "ListByUnit", dbParams)

	forms, err := s.queries.ListByUnit(ctx, pgtype.UUID{Bytes: unitID, Valid: true})
	if err != nil {
		err = databaseutil.WrapDBErrorWithTracker(err, tracker, "list forms by unit")
		span.RecordError(err)
		return []ListByUnitRow{}, err
	}

	tracker.SuccessRead(len(forms), unitID.String())

	return forms, nil
}

func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status Status, userID uuid.UUID) (Form, error) {
	ctx, span := s.tracer.Start(ctx, "SetStatus")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	dbParams := map[string]interface{}{
		"id":          id.String(),
		"status":      string(status),
		"last_editor": userID.String(),
	}
	tracker := logutil.StartDBOperation(ctx, logger, "SetStatus", dbParams)

	updated, err := s.queries.SetStatus(ctx, SetStatusParams{
		ID:         id,
		Status:     status,
		LastEditor: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithTracker(err, tracker, "set form status")
		span.RecordError(err)
		return Form{}, err
	}

	tracker.SuccessWrite(id.String())

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
		forms, err := s.queries.ListByUnit(ctx, pgtype.UUID{Bytes: unitID, Valid: true})
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

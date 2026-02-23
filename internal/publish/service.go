package publish

import (
	"context"
	"errors"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/form/workflow"
	"NYCU-SDC/core-system-backend/internal/inbox"

	"github.com/jackc/pgx/v5"
	"github.com/google/uuid"
	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Distributor interface {
	GetOrgRecipients(ctx context.Context, orgID uuid.UUID) ([]uuid.UUID, error)
	GetRecipients(ctx context.Context, unitIDs []uuid.UUID) ([]uuid.UUID, error)
}

type FormStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (form.GetByIDRow, error)
	SetStatus(ctx context.Context, id uuid.UUID, status form.Status, userID uuid.UUID) (form.Form, error)
}

type InboxPort interface {
	Create(ctx context.Context, contentType inbox.ContentType, contentID uuid.UUID, userIDs []uuid.UUID, postByUnitID uuid.UUID) (uuid.UUID, error)
}

type WorkflowStore interface {
	Get(ctx context.Context, formID uuid.UUID) (workflow.GetRow, error)
	Activate(ctx context.Context, formID uuid.UUID, userID uuid.UUID, workflow []byte) (workflow.ActivateRow, error)
}

type Selection struct {
	OrgID   uuid.UUID
	UnitIDs []uuid.UUID
}

type Service struct {
	logger      *zap.Logger
	tracer      trace.Tracer
	distributor Distributor
	store       FormStore
	inbox       InboxPort
	workflow    WorkflowStore
}

func NewService(
	logger *zap.Logger,
	distributor Distributor,
	store FormStore,
	inbox InboxPort,
	workflow WorkflowStore,
) *Service {
	return &Service{
		logger:      logger,
		tracer:      otel.Tracer("publish/service"),
		distributor: distributor,
		store:       store,
		inbox:       inbox,
		workflow:    workflow,
	}
}

func (s *Service) GetRecipients(ctx context.Context, selection Selection) ([]uuid.UUID, error) {
	ctx, span := s.tracer.Start(ctx, "GetRecipients")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	var users []uuid.UUID
	if selection.OrgID != uuid.Nil {
		orgUsers, err := s.distributor.GetOrgRecipients(ctx, selection.OrgID)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "getting org recipients")
			span.RecordError(err)
			return nil, err
		}
		users = append(users, orgUsers...)
	} else if len(selection.UnitIDs) > 0 {
		unitUsers, err := s.distributor.GetRecipients(ctx, selection.UnitIDs)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "getting unit recipients")
			span.RecordError(err)
			return nil, err
		}
		users = append(users, unitUsers...)
	}

	// can add some verify method here

	return users, nil
}

// PublishForm not Publish is because maybe we will publish something else in future
// This method is responsible for:
//  1. Ensuring the form is in draft status
//  2. Ensuring there is a latest workflow stored for the form
//  3. Activating that latest workflow from DB
//  4. Publishing the form
func (s *Service) PublishForm(ctx context.Context, formID uuid.UUID, editor uuid.UUID) (form.Visibility, error) {
	ctx, span := s.tracer.Start(ctx, "PublishForm")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	// check form existence and status
	targetForm, err := s.store.GetByID(ctx, formID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, internal.ErrFormNotFound) {
			span.RecordError(internal.ErrFormNotFound)
			return "", internal.ErrFormNotFound
		}
		err = databaseutil.WrapDBError(err, logger, "getting form by id")
		span.RecordError(err)
		return "", err
	}

	if targetForm.Status != form.StatusDraft {
		err = internal.ErrFormNotDraft
		span.RecordError(err)
		return "", err
	}

	// Always activate the latest stored workflow before publishing.
	// This uses the workflow stored in DB instead of expecting the client
	// to provide workflow JSON.
	latestWorkflow, err := s.workflow.Get(ctx, formID)
	if err != nil {
		span.RecordError(err)
		return "", err
	}

	activatedVersion, err := s.workflow.Activate(ctx, formID, editor, latestWorkflow.Workflow)
	if err != nil {
		logger.Error("failed to activate workflow during publish", zap.Error(err), zap.String("formId", formID.String()))
		span.RecordError(err)
		return "", err
	}

	if !activatedVersion.IsActive {
		logger.Error("workflow activation returned inactive version",
			zap.String("formId", formID.String()),
			zap.String("versionId", activatedVersion.ID.String()),
			zap.Bool("isActive", activatedVersion.IsActive))
		span.RecordError(internal.ErrWorkflowNotActive)
		return "", internal.ErrWorkflowNotActive
	}

	updatedForm, err := s.store.SetStatus(ctx, formID, form.StatusPublished, editor)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "setting form status = published")
		span.RecordError(err)
		return "", err
	}

	logger.Info("Form published",
		zap.String("form_id", formID.String()),
		zap.String("editor", editor.String()),
	)
	return updatedForm.Visibility, nil
}

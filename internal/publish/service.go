package publish

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/inbox"
	"context"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"

	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/form/workflow"

	"github.com/google/uuid"
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
func (s *Service) PublishForm(ctx context.Context, formID uuid.UUID, unitIDs []uuid.UUID, editor uuid.UUID) (form.Visibility, error) {
	ctx, span := s.tracer.Start(ctx, "PublishForm")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	// check form existence and status
	targetForm, err := s.store.GetByID(ctx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "getting form by id")
		span.RecordError(err)
		return "", err
	}

	if targetForm.Status != form.StatusDraft {
		err = internal.ErrFormNotDraft
		span.RecordError(err)
		return "", err
	}

	// check workflow is active
	workflowVersion, err := s.workflow.Get(ctx, formID)
	if err != nil {
		span.RecordError(err)
		return "", err
	}

	if !workflowVersion.IsActive {
		err = internal.ErrWorkflowNotActive
		span.RecordError(err)
		return "", err
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

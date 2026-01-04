package publish

import (
	"context"
	"fmt"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/inbox"

	logutil "github.com/NYCU-SDC/summer/pkg/log"

	"NYCU-SDC/core-system-backend/internal/form"

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
}

func NewService(
	logger *zap.Logger,
	distributor Distributor,
	store FormStore,
	inbox InboxPort,
) *Service {
	return &Service{
		logger:      logger,
		tracer:      otel.Tracer("publish/service"),
		distributor: distributor,
		store:       store,
		inbox:       inbox,
	}
}

func (s *Service) GetRecipients(ctx context.Context, selection Selection) ([]uuid.UUID, error) {
	methodName := "GetRecipients"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	tracker := logutil.StartMethod(ctx, logger, methodName, map[string]interface{}{
		"selection": selection,
	},
	)

	var users []uuid.UUID
	if selection.OrgID != uuid.Nil {
		orgUsers, err := s.distributor.GetOrgRecipients(ctx, selection.OrgID)
		if err != nil {
			wrappedErr := fmt.Errorf("failed to resolve org recipients: %w", err)
			span.RecordError(wrappedErr)
			return nil, wrappedErr
		}

		logger.Debug("Retrieved org recipients",
			zap.String("org_id", selection.OrgID.String()),
			zap.Int("recipient_count", len(orgUsers)),
		)
		users = append(users, orgUsers...)
	} else if len(selection.UnitIDs) > 0 {
		unitUsers, err := s.distributor.GetRecipients(ctx, selection.UnitIDs)
		if err != nil {
			wrappedErr := fmt.Errorf("failed to resolve unit recipients: %w", err)
			span.RecordError(wrappedErr)
			return nil, wrappedErr
		}

		logger.Debug("Retrieved unit recipients",
			zap.Int("unit_count", len(selection.UnitIDs)),
			zap.Int("recipient_count", len(unitUsers)),
		)
		users = append(users, unitUsers...)
	}

	// can add some verify method here

	tracker.Complete(map[string]interface{}{
		"recipient_count": len(users),
	})

	return users, nil
}

// PublishForm not Publish is because maybe we will publish something else in future
func (s *Service) PublishForm(ctx context.Context, formID uuid.UUID, unitIDs []uuid.UUID, editor uuid.UUID) error {
	methodName := "PublishForm"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	params := map[string]interface{}{
		"form_id":        formID.String(),
		"editor":         editor.String(),
		"unit_ids_count": len(unitIDs),
	}
	tracker := logutil.StartMethod(ctx, logger, methodName, params)

	// check form existence and status
	targetForm, err := s.store.GetByID(ctx, formID)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to retrieve target form: %w", err)
		span.RecordError(wrappedErr)
		return wrappedErr
	}

	if targetForm.Status != form.StatusDraft {
		logger.Warn("Form is not in draft status",
			zap.String("form_id", formID.String()),
			zap.String("current_status", string(targetForm.Status)),
			zap.String("expected_status", string(form.StatusDraft)),
		)
		err = internal.ErrFormNotDraft
		span.RecordError(err)
		return err
	}

	_, err = s.store.SetStatus(ctx, formID, form.StatusPublished, editor)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to update form status: %w", err)
		span.RecordError(wrappedErr)
		return wrappedErr
	}

	unitID, err := uuid.Parse(targetForm.UnitID.String())
	if err != nil {
		logger.Error("failed to parse unit ID", zap.Error(err))
		span.RecordError(err)
		return err
	}

	recipientIDs, err := s.GetRecipients(ctx, Selection{
		UnitIDs: unitIDs,
	})
	if err != nil {
		wrappedErr := fmt.Errorf("failed to calculate recipient list: %w", err)
		span.RecordError(wrappedErr)
		return wrappedErr
	}

	logger.Debug("Recipients retrieved for form publish",
		zap.String("form_id", formID.String()),
		zap.Int("recipient_count", len(recipientIDs)),
		zap.Int("unit_count", len(unitIDs)),
	)

	_, err = s.inbox.Create(ctx, inbox.ContentTypeForm, formID, recipientIDs, unitID)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to dispatch inbox notifications: %w", err)
		span.RecordError(wrappedErr)
		return wrappedErr
	}

	tracker.Complete(map[string]interface{}{
		"recipient_count": len(recipientIDs),
	})
	return nil
}

package distribute

import (
	"context"
	"fmt"

	"NYCU-SDC/core-system-backend/internal/user"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type UnitStore interface {
	ListMembers(ctx context.Context, id uuid.UUID) ([]user.Profile, error)
	ListUnitsMembers(ctx context.Context, unitIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
}

type Service struct {
	logger *zap.Logger
	tracer trace.Tracer
	store  UnitStore
}

func NewService(logger *zap.Logger, store UnitStore) *Service {
	return &Service{
		logger: logger,
		store:  store,
		tracer: otel.Tracer("distribute/service"),
	}
}

// Todo: Need to use SimpleUser instead of uuid.UUID
func (s *Service) GetOrgRecipients(ctx context.Context, orgID uuid.UUID) ([]uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetOrgRecipients")
	defer span.End()

	recipients, err := s.store.ListMembers(traceCtx, orgID)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to fetch org members: %w", err)
		span.RecordError(wrappedErr)
		return nil, wrappedErr
	}

	ids := make([]uuid.UUID, 0, len(recipients))
	for _, r := range recipients {
		ids = append(ids, r.ID)
	}

	return ids, nil
}

func (s *Service) GetRecipients(ctx context.Context, unitIDs []uuid.UUID) ([]uuid.UUID, error) {
	methodName := "GetRecipients"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	tracker := logutil.StartMethod(ctx, logger, methodName, map[string]interface{}{
		"unitIDs": len(unitIDs),
	})

	all := make([]uuid.UUID, 0)

	memberMap, err := s.store.ListUnitsMembers(ctx, unitIDs)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to fetch unit members: %w", err)
		span.RecordError(wrappedErr)
		return nil, wrappedErr
	}

	for _, ms := range memberMap {
		all = append(all, ms...)
	}

	logger.Debug("Processing recipients calculation",
		zap.Int("raw_total_count", len(all)),
	)

	seen := make(map[uuid.UUID]struct{}, len(all))
	uniq := make([]uuid.UUID, 0, len(all))
	for _, id := range all {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}

	tracker.Complete(map[string]interface{}{
		"recipients_count": len(uniq),
	})

	return uniq, nil
}

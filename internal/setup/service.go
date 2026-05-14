package setup

import (
	"NYCU-SDC/core-system-backend/internal"
	config2 "NYCU-SDC/core-system-backend/internal/config"
	"NYCU-SDC/core-system-backend/internal/unit"
	"context"
	"errors"
	"fmt"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Service struct {
	logger      *zap.Logger
	tracer      trace.Tracer
	setupImpl   config2.SetupImpl
	unitService UnitService
	userService UserService
}

type UnitService interface {
	SlugExists(ctx context.Context, slug string) (bool, error)
	CreateOrganization(ctx context.Context, name string, description string, slug string) (unit.Unit, error)
	GetOrgIDBySlug(ctx context.Context, slug string) (uuid.UUID, error)
	AddMemberWithRole(ctx context.Context, unitID uuid.UUID, memberID uuid.UUID, role string) error
	GetMemberRole(ctx context.Context, unitID uuid.UUID, memberID uuid.UUID) (unit.UnitRole, error)
	UpdateUnitMemberRole(ctx context.Context, unitID uuid.UUID, memberID uuid.UUID, newRole unit.UnitRole) error
}

type UserService interface {
	FindOrCreateByEmail(ctx context.Context, email string, globalRole []string, userID *uuid.UUID) (uuid.UUID, error)
}

func NewService(logger *zap.Logger, setupImpl config2.SetupImpl, unitService UnitService, userService UserService) *Service {
	service := &Service{
		logger:      logger,
		tracer:      otel.Tracer("setup"),
		setupImpl:   setupImpl,
		unitService: unitService,
		userService: userService,
	}

	return service
}

func (s *Service) Setup(ctx context.Context) error {
	traceCtx, span := s.tracer.Start(ctx, "Setup")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	adminCount := make(map[string]int)
	for _, user := range s.setupImpl.Config.Users {
		for _, member := range user.OrgMember {
			if member.OrgRole == "admin" {
				adminCount[member.Slug]++
			}
		}
	}

	for _, org := range s.setupImpl.Config.Organizations {
		if adminCount[org.Slug] < 1 {
			logger.Error("The organization does not have the admin role", zap.String("org_name", org.Name))
			err := fmt.Errorf("the organization %s does not have the admin role", org.Name)
			span.RecordError(err)
			return err
		}
	}

	for _, org := range s.setupImpl.Config.Organizations {
		exist, err := s.unitService.SlugExists(traceCtx, org.Slug)
		if err != nil {
			logger.Error("Failed to check if the organization exists", zap.Error(err))
			span.RecordError(err)
			return err
		}
		if !exist {
			_, err := s.unitService.CreateOrganization(traceCtx, org.Name, org.Description, org.Slug)
			if err != nil {
				logger.Error("Failed to initialize organization", zap.String("org_name", org.Name), zap.Error(err))
				span.RecordError(err)
				return err
			}
		}
	}
	logger.Info("Successfully initialized organizations")

	userIDs := make(map[string]uuid.UUID, len(s.setupImpl.Config.Users))
	for _, user := range s.setupImpl.Config.Users {
		id, err := s.userService.FindOrCreateByEmail(traceCtx, user.Email, user.GlobalRole, user.UserID)
		if err != nil {
			logger.Error("Failed to find or create user", zap.String("email", user.Email), zap.Error(err))
			span.RecordError(err)
			return err
		}
		userIDs[user.Email] = id
	}
	logger.Info("Successfully initialized users")

	for _, user := range s.setupImpl.Config.Users {
		userID := userIDs[user.Email]
		for _, org := range user.OrgMember {
			orgID, err := s.unitService.GetOrgIDBySlug(traceCtx, org.Slug)
			if err != nil {
				logger.Error("Failed to get organization id by slug", zap.String("org_name", org.Slug), zap.Error(err))
				span.RecordError(err)
				return err
			}
			_, err = s.unitService.GetMemberRole(traceCtx, orgID, userID)
			if err != nil {
				if errors.Is(err, internal.ErrNotFound) {
					err = s.unitService.AddMemberWithRole(traceCtx, orgID, userID, org.OrgRole)
					if err != nil {
						logger.Error("Failed to add member with role", zap.String("org_name", org.Slug), zap.String("user_email", user.Email), zap.Error(err))
						span.RecordError(err)
						return err
					}
					continue
				}
				logger.Error("Failed to get current member role", zap.String("org_name", org.Slug), zap.String("user_email", user.Email), zap.Error(err))
				span.RecordError(err)
				return err
			}
		}
	}
	logger.Info("Successfully initialized user roles")

	return nil
}

func (s *Service) AllowedOnboarding(userEmail string) bool {
	return s.setupImpl.AllowedOnboarding(userEmail)
}

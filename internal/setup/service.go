package setup

import (
	config2 "NYCU-SDC/core-system-backend/internal/config"
	"NYCU-SDC/core-system-backend/internal/unit"
	"context"
	"fmt"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Service struct {
	logger      *zap.Logger
	tracer      trace.Tracer
	db          *pgxpool.Pool
	setupImpl   config2.SetupImpl
	unitService UnitService
	userService UserService
}

type UnitService interface {
	SlugExists(ctx context.Context, slug string) (bool, error)
	CreateOrganization(ctx context.Context, name string, description string, slug string) (unit.Unit, error)
}

type UserService interface {
	FindOrCreateByEmail(ctx context.Context, email string, globalRole []string) (uuid.UUID, error)
}

func NewService(logger *zap.Logger, db *pgxpool.Pool, setupImpl config2.SetupImpl, unitService UnitService, userService UserService) *Service {
	service := &Service{
		logger:      logger,
		tracer:      otel.Tracer("setup"),
		db:          db,
		setupImpl:   setupImpl,
		unitService: unitService,
		userService: userService,
	}

	logger.Info("NewService process done")

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

	for _, user := range s.setupImpl.Config.Users {
		_, err := s.userService.FindOrCreateByEmail(traceCtx, user.Email, user.GlobalRole)
		if err != nil {
			logger.Error("Failed to find or create user", zap.String("email", user.Email), zap.Error(err))
			span.RecordError(err)
			return err
		}
	}
	logger.Info("Successfully initialized users")

	return nil
}

func (s *Service) AllowedOnboarding(userEmail string) bool {
	return s.setupImpl.AllowedOnboarding(userEmail)
}

package setup

import (
	"NYCU-SDC/core-system-backend/internal/unit"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type Service struct {
	logger                *zap.Logger
	tracer                trace.Tracer
	db                    *pgxpool.Pool
	config                SetupConfig
	allowedOnboardingList AllowedOnboardingList
	unitService           UnitService
	userService           UserService
}

type UnitService interface {
	SlugExists(ctx context.Context, slug string) (bool, error)
	CreateOrganization(ctx context.Context, name string, description string, slug string) (unit.Unit, error)
}

type UserService interface {
	FindOrCreateByEmail(ctx context.Context, email string, globalRole []string) (uuid.UUID, error)
}

func NewService(logger *zap.Logger, db *pgxpool.Pool, setupPath string, setupData string, unitService UnitService, userService UserService) (*Service, error) {
	var config SetupConfig

	data, err := os.ReadFile(setupPath)
	if err != nil {
		if setupData != "" {
			logger.Info("Loading setup config from SETUP_YAML environment variable")
			decoded, decErr := base64.StdEncoding.DecodeString(setupData)
			if decErr != nil {
				logger.Error("Failed to base64 decode SETUP_YAML", zap.Error(decErr))
				return nil, fmt.Errorf("failed to base64 decode SETUP_YAML: %w", decErr)
			}
			data = decoded
		} else {
			// missing setup config is expected, so the process can go on without it
			logger.Warn("No setup config found (neither file nor SETUP_YAML env)", zap.String("path", setupPath))
		}
	}

	if data != nil {
		err = yaml.Unmarshal(data, &config)
		if err != nil {
			logger.Error("Failed to parse setup config", zap.Error(err))
			return nil, fmt.Errorf("failed to parse setup config: %w", err)
		}
	}

	allowedList := make(AllowedOnboardingList)
	for _, user := range config.Users {
		if user.AllowedOnboarding {
			allowedList[strings.ToLower(user.Email)] = struct{}{}
		}
	}

	service := &Service{
		logger:                logger,
		tracer:                otel.Tracer("setup"),
		db:                    db,
		config:                config,
		allowedOnboardingList: allowedList,
		unitService:           unitService,
		userService:           userService,
	}

	logger.Info("NewService process done", zap.Int("allowed_onboarding_count", len(allowedList)))

	return service, nil
}

func (s *Service) Setup(ctx context.Context) error {
	traceCtx, span := s.tracer.Start(ctx, "ExistsByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	adminCount := make(map[string]int)
	for _, user := range s.config.Users {
		for _, member := range user.OrgMember {
			if member.OrgRole == "admin" {
				adminCount[member.Slug]++
			}
		}
	}

	for _, org := range s.config.Organizations {
		if adminCount[org.Slug] < 1 {
			logger.Error("The organization does not have the admin role", zap.String("org_name", org.Name))
			return fmt.Errorf("the organization %s does not have the admin role", org.Name)
		}
	}

	for _, org := range s.config.Organizations {
		exist, err := s.unitService.SlugExists(ctx, org.Slug)
		if err != nil {
			logger.Error("Failed to check if the organization exists", zap.Error(err))
			return err
		}
		if !exist {
			_, err := s.unitService.CreateOrganization(ctx, org.Name, org.Description, org.Slug)
			if err != nil {
				logger.Error("Failed to initialize organization", zap.String("org_name", org.Name), zap.Error(err))
				return err
			}
		}
	}
	logger.Info("Successfully initialized organizations")

	for _, user := range s.config.Users {
		_, err := s.userService.FindOrCreateByEmail(ctx, user.Email, user.GlobalRole)
		if err != nil {
			logger.Error("Failed to find or create user", zap.String("email", user.Email), zap.Error(err))
			return err
		}
	}
	logger.Info("Successfully initialized users")

	return nil
}

func (s *Service) AllowedOnboarding(email string) bool {
	_, exist := s.allowedOnboardingList[strings.ToLower(email)]
	return exist
}

package setup

import (
	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type Service struct {
	logger                *zap.Logger
	db                    *pgxpool.Pool
	config                SetupConfig
	allowedOnboardingList AllowedOnboardingList
	unitService           *unit.Service
	userService           *user.Service
}

func NewService(logger *zap.Logger, db *pgxpool.Pool, setupPath string, unitService *unit.Service, userService *user.Service) (*Service, error) {
	var config SetupConfig
	data, err := os.ReadFile(setupPath)
	if err != nil {
		//missing setup files is expected, so the process can go on without setup files
		logger.Warn("Failed to read setup file", zap.String("path", setupPath), zap.Error(err))
	} else {
		err = yaml.Unmarshal(data, &config)
		if err != nil {
			logger.Error("Failed to parse setup file", zap.String("path", setupPath), zap.Error(err))
			return nil, fmt.Errorf("failed to parse setup file: %w", err)
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
		db:                    db,
		config:                config,
		allowedOnboardingList: allowedList,
		unitService:           unitService,
		userService:           userService,
	}

	logger.Info("NewService proccess done", zap.Int("allowed_onboarding_count", len(allowedList)))

	return service, nil
}

func (s *Service) Setup(ctx context.Context) error {
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
			s.logger.Error("The organization does not have the admin role", zap.String("org_name", org.Name))
			return fmt.Errorf("the organization %s does not have the admin role", org.Name)
		}
	}

	for _, org := range s.config.Organizations {
		exist, err := s.unitService.SlugExists(ctx, org.Slug)
		if err != nil {
			s.logger.Error("Failed to check if the organization exists", zap.Error(err))
			return err
		}
		if !exist {
			_, err := s.unitService.CreateOrganization(ctx, org.Name, org.Description, org.Slug)
			if err != nil {
				s.logger.Error("Failed to initialize organization", zap.String("org_name", org.Name), zap.Error(err))
				return err
			}
		}
	}
	s.logger.Info("Successfully initialized organizations")

	for _, user := range s.config.Users {
		_, err := s.userService.FindOrCreateByEmail(ctx, user.Email, user.GlobalRole)
		if err != nil {
			s.logger.Error("Failed to find or create user", zap.String("email", user.Email), zap.Error(err))
			return err
		}
	}
	s.logger.Info("Successfully initialized users")

	return nil
}

func (s *Service) AllowedOnboarding(email string) bool {
	_, exist := s.allowedOnboardingList[strings.ToLower(email)]
	return exist
}

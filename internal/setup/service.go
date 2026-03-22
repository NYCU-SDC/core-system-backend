package setup

import (
	"NYCU-SDC/core-system-backend/internal/unit"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type Service struct {
	logger                *zap.Logger
	db                    *pgxpool.Pool
	config                SetupConfig
	allowedOnboardingList AllowedOnboardingList
	unitService           unit.Service
}

func NewService(logger *zap.Logger, db *pgxpool.Pool, setupPath string) (*Service, error) {
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
	}

	logger.Info("NewService proccess done", zap.Int("allowed_onboarding_count", len(allowedList)))

	return service, nil
}

func (s *Service) Setup(ctx context.Context) error {
	metadata := []byte{}
	currentUserID := uuid.UUID{}
	for _, org := range s.config.Organizations {
		_, err := s.unitService.CreateOrganization(ctx, org.Name, org.Description, org.Slug, currentUserID, metadata)
		if err != nil {
			s.logger.Error("Failed to initialize organization", zap.String("org_name", org.Name), zap.Error(err))
			return err
		}
	}
	return nil
}

func (s *Service) AllowedOnboarding(email string) bool {
	_, exist := s.allowedOnboardingList[strings.ToLower(email)]
	return exist
}

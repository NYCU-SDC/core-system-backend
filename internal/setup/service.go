package setup

import (
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
}

func NewService(logger *zap.Logger, db *pgxpool.Pool, setupPath string) (*Service, error) {
	data, err := os.ReadFile(setupPath)
	if err != nil {
		logger.Error("Failed to read setup file", zap.String("path", setupPath), zap.Error(err))
		return nil, fmt.Errorf("failed to read setup file: %w", err)
	}

	var config SetupConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		logger.Error("Failed to parse setup file", zap.String("path", setupPath), zap.Error(err))
		return nil, fmt.Errorf("failed to parse setup file: %w", err)
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

func (s *Service) Setup() error {
	return nil
}

func (s *Service) AllowedOnboarding(email string) bool {
	_, exist := s.allowedOnboardingList[strings.ToLower(email)]
	return exist
}

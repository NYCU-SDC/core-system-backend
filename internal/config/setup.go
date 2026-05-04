package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type SetupConfig struct {
	Organizations []Organization `yaml:"organizations"`
	Users         []User         `yaml:"users"`
}

type Organization struct {
	Name        string `yaml:"name"`
	Slug        string `yaml:"slug"`
	Description string `yaml:"description"`
}

type User struct {
	Email             string      `yaml:"email"`
	GlobalRole        []string    `yaml:"global_role"`
	OrgMember         []OrgMember `yaml:"org_member"`
	AllowedOnboarding bool        `yaml:"allowed_onboarding"`
}

type OrgMember struct {
	Slug    string `yaml:"slug"`
	OrgRole string `yaml:"org_role"`
}
type AllowedOnboardingList map[string]struct{}

type SetupImpl struct {
	Config                SetupConfig
	AllowedOnboardingList AllowedOnboardingList
}

func (s *SetupImpl) LoadSetupConfig(logger *zap.Logger, setupPath string, setupData string) error {
	var cfg SetupConfig

	data, err := os.ReadFile(setupPath)
	if err != nil {
		if setupData != "" {
			logger.Info("Loading setup config from SETUP_YAML environment variable")
			decoded, decErr := base64.StdEncoding.DecodeString(setupData)
			if decErr != nil {
				logger.Error("Failed to base64 decode SETUP_YAML", zap.Error(decErr))
				return fmt.Errorf("failed to base64 decode SETUP_YAML: %w", decErr)
			}
			data = decoded
		} else {
			// missing setup config is expected, so the process can go on without it
			logger.Warn("No setup config found (neither file nor SETUP_YAML env)", zap.String("path", setupPath))
		}
	}

	if data != nil {
		err = yaml.Unmarshal(data, &cfg)
		if err != nil {
			logger.Error("Failed to parse setup config", zap.Error(err))
			return fmt.Errorf("failed to parse setup config: %w", err)
		}
	}

	allowedList := make(AllowedOnboardingList)
	for _, user := range cfg.Users {
		if user.AllowedOnboarding {
			allowedList[strings.ToLower(user.Email)] = struct{}{}
		}
	}

	s.Config = cfg
	s.AllowedOnboardingList = allowedList

	return nil
}

func (s *SetupImpl) AllowedOnboarding(email string) bool {
	_, exist := s.AllowedOnboardingList[strings.ToLower(email)]
	return exist
}

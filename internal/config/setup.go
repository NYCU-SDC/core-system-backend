package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type rawSetupConfig struct {
	Organizations []rawOrganization `yaml:"organizations"`
	Users         []rawUser         `yaml:"users"`
}

type rawOrganization struct {
	Name        string `yaml:"name"`
	Slug        string `yaml:"slug"`
	Description string `yaml:"description"`
}

type rawUser struct {
	Email             string         `yaml:"email"`
	UserID            string         `yaml:"user_id"`
	GlobalRole        []string       `yaml:"global_role"`
	OrgMember         []rawOrgMember `yaml:"org_member"`
	AllowedOnboarding bool           `yaml:"allowed_onboarding"`
}

type rawOrgMember struct {
	Slug    string `yaml:"slug"`
	OrgRole string `yaml:"org_role"`
}

type SetupConfig struct {
	Organizations []Organization
	Users         []User
}

type Organization struct {
	Name        string
	Slug        string
	Description string
}

type User struct {
	Email             string
	UserID            *uuid.UUID
	GlobalRole        []string
	OrgMember         []OrgMember
	AllowedOnboarding bool
}

type OrgMember struct {
	Slug    string
	OrgRole string
}
type AllowedOnboardingList map[string]struct{}

type SetupImpl struct {
	Config                SetupConfig
	AllowedOnboardingList AllowedOnboardingList
}

func (s *SetupImpl) LoadSetupConfig(logger *zap.Logger, setupPath string, setupData string) error {
	var rawCfg rawSetupConfig
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
		err = yaml.Unmarshal(data, &rawCfg)
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

	cfg, err = parseSetupConfig(rawCfg)
	if err != nil {
		return fmt.Errorf("invalid setup config: %w", err)
	}

	s.Config = cfg
	s.AllowedOnboardingList = allowedList

	return nil
}

func (s *SetupImpl) AllowedOnboarding(email string) bool {
	_, exist := s.AllowedOnboardingList[strings.ToLower(email)]
	return exist
}

func parseSetupConfig(raw rawSetupConfig) (SetupConfig, error) {
	orgs := make([]Organization, 0, len(raw.Organizations))
	users := make([]User, 0, len(raw.Users))

	for _, rawOrg := range raw.Organizations {
		org := Organization(rawOrg)

		if org.Slug == "" {
			return SetupConfig{}, fmt.Errorf("organization slug is required")
		}

		orgs = append(orgs, org)
	}

	for _, rawUser := range raw.Users {
		email := normalizeEmail(rawUser.Email)
		if email == "" {
			return SetupConfig{}, fmt.Errorf("user email is required")
		}

		userID, err := parseOptionalUUID(rawUser.UserID)
		if err != nil {
			return SetupConfig{}, fmt.Errorf("invalid user_id for email %q: %w", email, err)
		}

		orgMembers := make([]OrgMember, 0, len(rawUser.OrgMember))
		for _, rawMember := range rawUser.OrgMember {
			member := OrgMember(rawMember)

			if member.Slug == "" {
				return SetupConfig{}, fmt.Errorf("org member slug is required for email %q", email)
			}

			orgMembers = append(orgMembers, member)
		}

		users = append(users, User{
			Email:             email,
			UserID:            userID,
			GlobalRole:        rawUser.GlobalRole,
			OrgMember:         orgMembers,
			AllowedOnboarding: rawUser.AllowedOnboarding,
		})
	}

	return SetupConfig{
		Organizations: orgs,
		Users:         users,
	}, nil
}

func parseOptionalUUID(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	id, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}

	return &id, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

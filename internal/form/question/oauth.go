package question

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type OauthProvider string

const (
	GoogleOauthProvider OauthProvider = "google"
	GitHubOauthProvider OauthProvider = "github"
)

var validOauthProviders = map[OauthProvider]bool{
	GoogleOauthProvider: true,
	GitHubOauthProvider: true,
}

type OAuthConnect struct {
	question Question
	formID   uuid.UUID
	Provider OauthProvider
}

func (o OAuthConnect) Question() Question {
	return o.question
}

func (o OAuthConnect) FormID() uuid.UUID {
	return o.formID
}

func (o OAuthConnect) Validate(value string) error {
	if value == "" {
		return fmt.Errorf("oauth value cannot be empty")
	}

	var oauthData struct {
		AvatarURL string `json:"avatar_url"`
		Username  string `json:"username"`
	}

	if err := json.Unmarshal([]byte(value), &oauthData); err != nil {
		return fmt.Errorf("invalid oauth JSON structure: %w", err)
	}

	// Validate required fields
	if oauthData.Username == "" {
		return fmt.Errorf("oauth username cannot be empty")
	}

	if oauthData.AvatarURL == "" {
		return fmt.Errorf("oauth avatar_url cannot be empty")
	}

	return nil
}

func NewOAuthConnect(q Question, formID uuid.UUID) (OAuthConnect, error) {
	if q.Metadata == nil {
		return OAuthConnect{}, errors.New("metadata is nil")
	}

	var partial map[string]json.RawMessage
	if err := json.Unmarshal(q.Metadata, &partial); err != nil {
		return OAuthConnect{}, fmt.Errorf("could not parse partial json: %w", err)
	}

	provider, err := ExtractOauthConnect(q.Metadata)
	if err != nil {
		return OAuthConnect{}, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: q.Metadata, Message: "oauthConnect field missing"}
	}

	if provider == "" {
		return OAuthConnect{}, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: q.Metadata, Message: "oauthConnect provider is empty"}
	}

	if provider != GoogleOauthProvider && provider != GitHubOauthProvider {
		return OAuthConnect{}, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: q.Metadata, Message: "invalid oauthConnect provider"}
	}

	return OAuthConnect{
		question: q,
		formID:   formID,
		Provider: provider,
	}, nil
}

func GenerateOauthConnectMetadata(provider string) ([]byte, error) {
	if provider == "" {
		return nil, ErrMetadataValidate{
			QuestionID: "oauth_connect",
			RawData:    []byte(fmt.Sprintf("%v", provider)),
			Message:    "no provider provided for oauth_connect question",
		}
	}

	oauthProvider := OauthProvider(strings.ToLower(provider))
	if !validOauthProviders[oauthProvider] {
		return nil, fmt.Errorf("invalid OAuth provider: %s", provider)
	}

	metadata := map[string]any{
		"oauthConnect": provider,
	}

	return json.Marshal(metadata)
}

func ExtractOauthConnect(data []byte) (OauthProvider, error) {
	var partial map[string]json.RawMessage
	if err := json.Unmarshal(data, &partial); err != nil {
		return "", fmt.Errorf("could not parse partial json: %w", err)
	}

	var provider OauthProvider
	if raw, ok := partial["oauthConnect"]; ok {
		if err := json.Unmarshal(raw, &provider); err != nil {
			return "", fmt.Errorf("could not parse oauth provider: %w", err)
		}
	}

	return provider, nil
}

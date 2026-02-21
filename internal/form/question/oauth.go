package question

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"NYCU-SDC/core-system-backend/internal/form/shared"

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

func (o OAuthConnect) Validate(rawValue json.RawMessage) error {
	var v shared.OAuthConnectAnswer
	if err := json.Unmarshal(rawValue, &v); err != nil {
		return fmt.Errorf("invalid oauth_connect answer: %w", err)
	}
	if v.Provider == "" || v.ProviderID == "" {
		return errors.New("oauth_connect answer must have provider and providerId")
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

func (o OAuthConnect) DecodeRequest(rawValue json.RawMessage) (any, error) {
	var v shared.OAuthConnectAnswer
	if err := json.Unmarshal(rawValue, &v); err != nil {
		return nil, fmt.Errorf("failed to decode oauth_connect answer from request: %w", err)
	}
	return v, nil
}

func (o OAuthConnect) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var v shared.OAuthConnectAnswer
	if err := json.Unmarshal(rawValue, &v); err != nil {
		return nil, fmt.Errorf("failed to decode oauth_connect answer from storage: %w", err)
	}
	return v, nil
}

func (o OAuthConnect) EncodeRequest(answer any) (json.RawMessage, error) {
	v, ok := answer.(shared.OAuthConnectAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.OAuthConnectAnswer, got %T", answer)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to encode oauth_connect answer: %w", err)
	}
	return raw, nil
}

func (o OAuthConnect) DisplayValue(rawValue json.RawMessage) (string, error) {
	var v shared.OAuthConnectAnswer
	if err := json.Unmarshal(rawValue, &v); err != nil {
		return "", fmt.Errorf("failed to decode oauth_connect answer for display: %w", err)
	}
	if v.Username != "" && v.Email != "" {
		return fmt.Sprintf("%s(%s)", v.Username, v.Email), nil
	}
	if v.Username != "" {
		return v.Username, nil
	}
	return v.Email, nil
}

func (o OAuthConnect) MatchesPattern(rawValue json.RawMessage, pattern string) (bool, error) {
	return false, errors.New("MatchesPattern is not supported for oauth_connect question type")
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

package question

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
)

type ShortText struct {
	question Question
	formID   uuid.UUID
}

func NewShortText(q Question, formID uuid.UUID) ShortText {
	return ShortText{
		question: q,
		formID:   formID,
	}
}

func (s ShortText) Question() Question {
	return s.question
}

func (s ShortText) FormID() uuid.UUID {
	return s.formID
}

func (s ShortText) Validate(rawValue json.RawMessage) error {
	var value string
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return fmt.Errorf("invalid short text value format: %w", err)
	}

	if len(value) > 100 {
		return ErrInvalidAnswerLength{
			Expected: 100,
			Given:    len(value),
		}
	}

	return nil
}

func (s ShortText) DecodeRequest(rawValue json.RawMessage) (any, error) {
	var value string
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return nil, fmt.Errorf("invalid short text value format: %w", err)
	}

	return shared.ShortTextAnswer{Value: value}, nil
}

func (s ShortText) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.ShortTextAnswer
	if err := json.Unmarshal(rawValue, &answer); err != nil {
		return nil, fmt.Errorf("invalid short text answer in storage: %w", err)
	}

	return answer, nil
}

func (s ShortText) EncodeRequest(answer any) (json.RawMessage, error) {
	shortTextAnswer, ok := answer.(shared.ShortTextAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.ShortTextAnswer, got %T", answer)
	}

	// API expects a plain string value
	return json.Marshal(shortTextAnswer.Value)
}

func (s ShortText) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := s.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	shortTextAnswer, ok := answer.(shared.ShortTextAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.ShortTextAnswer, got %T", answer)
	}

	return shortTextAnswer.Value, nil
}

func (s ShortText) MatchesPattern(rawValue json.RawMessage, pattern string) (bool, error) {
	answer, err := s.DecodeStorage(rawValue)
	if err != nil {
		return false, fmt.Errorf("failed to decode short text answer: %w", err)
	}

	shortTextAnswer, ok := answer.(shared.ShortTextAnswer)
	if !ok {
		return false, fmt.Errorf("expected shared.ShortTextAnswer, got %T", answer)
	}

	return matchPattern(shortTextAnswer.Value, pattern)
}

type LongText struct {
	question Question
	formID   uuid.UUID
}

func NewLongText(q Question, formID uuid.UUID) LongText {
	return LongText{
		question: q,
		formID:   formID,
	}
}

func (l LongText) Question() Question {
	return l.question
}

func (l LongText) FormID() uuid.UUID {
	return l.formID
}

func (l LongText) Validate(rawValue json.RawMessage) error {
	var value string
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return fmt.Errorf("invalid long text value format: %w", err)
	}

	if len(value) > 1000 {
		return ErrInvalidAnswerLength{
			Expected: 1000,
			Given:    len(value),
		}
	}

	return nil
}

func (l LongText) DecodeRequest(rawValue json.RawMessage) (any, error) {
	var value string
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return nil, fmt.Errorf("invalid long text value format: %w", err)
	}

	return shared.LongTextAnswer{Value: value}, nil
}

func (l LongText) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.LongTextAnswer
	if err := json.Unmarshal(rawValue, &answer); err != nil {
		return nil, fmt.Errorf("invalid long text answer in storage: %w", err)
	}

	return answer, nil
}

func (l LongText) EncodeRequest(answer any) (json.RawMessage, error) {
	longTextAnswer, ok := answer.(shared.LongTextAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.LongTextAnswer, got %T", answer)
	}

	// API expects a plain string value
	return json.Marshal(longTextAnswer.Value)
}

func (l LongText) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := l.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	longTextAnswer, ok := answer.(shared.LongTextAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.LongTextAnswer, got %T", answer)
	}

	value := longTextAnswer.Value
	// Limit to 100 characters
	if len(value) > 100 {
		return value[:100] + "...", nil
	}
	return value, nil
}

func (l LongText) MatchesPattern(rawValue json.RawMessage, pattern string) (bool, error) {
	answer, err := l.DecodeStorage(rawValue)
	if err != nil {
		return false, fmt.Errorf("failed to decode long text answer: %w", err)
	}

	longTextAnswer, ok := answer.(shared.LongTextAnswer)
	if !ok {
		return false, fmt.Errorf("expected shared.LongTextAnswer, got %T", answer)
	}

	return matchPattern(longTextAnswer.Value, pattern)
}

type Hyperlink struct {
	question Question
	formID   uuid.UUID
}

func NewHyperlink(q Question, formID uuid.UUID) Hyperlink {
	return Hyperlink{
		question: q,
		formID:   formID,
	}
}

func (h Hyperlink) Question() Question {
	return h.question
}

func (h Hyperlink) FormID() uuid.UUID {
	return h.formID
}

func (h Hyperlink) Validate(rawValue json.RawMessage) error {
	var value string
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return fmt.Errorf("invalid hyperlink value format: %w", err)
	}

	if len(value) > 100 {
		return ErrInvalidAnswerLength{
			Expected: 100,
			Given:    len(value),
		}
	}

	if err := validateURL(value); err != nil {
		return err
	}

	return nil
}

func (h Hyperlink) DecodeRequest(rawValue json.RawMessage) (any, error) {
	var value string
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return nil, fmt.Errorf("invalid hyperlink value format: %w", err)
	}

	return shared.HyperlinkAnswer{Value: value}, nil
}

func (h Hyperlink) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.HyperlinkAnswer
	if err := json.Unmarshal(rawValue, &answer); err != nil {
		return nil, fmt.Errorf("invalid hyperlink answer in storage: %w", err)
	}

	return answer, nil
}

func (h Hyperlink) EncodeRequest(answer any) (json.RawMessage, error) {
	hyperlinkAnswer, ok := answer.(shared.HyperlinkAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.HyperlinkAnswer, got %T", answer)
	}

	// API expects a plain string value
	return json.Marshal(hyperlinkAnswer.Value)
}

func (h Hyperlink) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := h.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	hyperlinkAnswer, ok := answer.(shared.HyperlinkAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.HyperlinkAnswer, got %T", answer)
	}

	return hyperlinkAnswer.Value, nil
}

func (h Hyperlink) MatchesPattern(rawValue json.RawMessage, pattern string) (bool, error) {
	answer, err := h.DecodeStorage(rawValue)
	if err != nil {
		return false, fmt.Errorf("failed to decode hyperlink answer: %w", err)
	}

	hyperlinkAnswer, ok := answer.(shared.HyperlinkAnswer)
	if !ok {
		return false, fmt.Errorf("expected shared.HyperlinkAnswer, got %T", answer)
	}

	return matchPattern(hyperlinkAnswer.Value, pattern)
}

// validateURL checks if the value is a valid URL
func validateURL(value string) error {
	if value == "" {
		return nil
	}

	parsedURL, err := url.Parse(value)
	if err != nil {
		return ErrInvalidHyperlinkFormat{
			Value:   value,
			Message: "invalid URL format",
		}
	}

	if parsedURL.Scheme == "" {
		return ErrInvalidHyperlinkFormat{
			Value:   value,
			Message: "URL must include a scheme (http:// or https://)",
		}
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrInvalidHyperlinkFormat{
			Value:   value,
			Message: "URL scheme must be http or https",
		}
	}

	if parsedURL.Host == "" {
		return ErrInvalidHyperlinkFormat{
			Value:   value,
			Message: "URL must include a host",
		}
	}

	return nil
}

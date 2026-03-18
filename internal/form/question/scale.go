package question

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
)

//go:embed icons.json
var iconsJSON []byte

var validIcons map[string]bool

// Import valid icon list at init
func init() {
	var icons []string
	if err := json.Unmarshal(iconsJSON, &icons); err != nil {
		validIcons = map[string]bool{"star": true}
		return
	}

	validIcons = make(map[string]bool, len(icons))
	for _, icon := range icons {
		validIcons[icon] = true
	}
}

type ScaleOption struct {
	Icon          string `json:"icon"`
	MinVal        int    `json:"minVal" validate:"required"`
	MaxVal        int    `json:"maxVal" validate:"required"`
	MinValueLabel string `json:"minValueLabel,omitempty"`
	MaxValueLabel string `json:"maxValueLabel,omitempty"`
}

type LinearScaleMetadata struct {
	Icon          string `json:"icon"`
	MinVal        int    `json:"minVal" validate:"required"`
	MaxVal        int    `json:"maxVal" validate:"required"`
	MinValueLabel string `json:"minValueLabel"`
	MaxValueLabel string `json:"maxValueLabel"`
}

type RatingMetadata struct {
	Icon          string `json:"icon" validate:"required"`
	MinVal        int    `json:"minVal" validate:"required"`
	MaxVal        int    `json:"maxVal" validate:"required"`
	MinValueLabel string `json:"minValueLabel"`
	MaxValueLabel string `json:"maxValueLabel"`
}

type LinearScale struct {
	question      Question
	formID        uuid.UUID
	MinVal        int
	MaxVal        int
	MinValueLabel string
	MaxValueLabel string
}

func NewLinearScale(q Question, formID uuid.UUID) (LinearScale, error) {
	metadata := q.Metadata
	if metadata == nil {
		return LinearScale{}, errors.New("metadata is nil")
	}

	linearScale, err := ExtractLinearScale(metadata)
	if err != nil {
		return LinearScale{}, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: metadata, Message: "could not extract linear scale options from metadata"}
	}

	if linearScale.MinVal >= linearScale.MaxVal {
		return LinearScale{}, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: metadata, Message: "minVal must be less than maxVal"}
	}

	return LinearScale{
		question:      q,
		formID:        formID,
		MinVal:        linearScale.MinVal,
		MaxVal:        linearScale.MaxVal,
		MinValueLabel: linearScale.MinValueLabel,
		MaxValueLabel: linearScale.MaxValueLabel,
	}, nil
}

func (s LinearScale) Question() Question { return s.question }

func (s LinearScale) FormID() uuid.UUID { return s.formID }

func (s LinearScale) Validate(rawValue json.RawMessage) error {
	var value int
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return fmt.Errorf("invalid linear scale value format: %w", err)
	}

	if value < s.MinVal || value > s.MaxVal {
		return ErrInvalidScaleValue{
			QuestionID: s.question.ID.String(),
			RawValue:   value,
			Message:    "out of range",
		}
	}

	return nil
}

func (s LinearScale) DecodeRequest(rawValue json.RawMessage) (any, error) {
	// API sends int32 for linear scale
	var value int
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return nil, fmt.Errorf("invalid linear scale value format: %w", err)
	}

	// Validate range
	if value < s.MinVal || value > s.MaxVal {
		return nil, ErrInvalidScaleValue{
			QuestionID: s.question.ID.String(),
			RawValue:   value,
			Message:    fmt.Sprintf("value %d is out of range [%d, %d]", value, s.MinVal, s.MaxVal),
		}
	}

	return shared.LinearScaleAnswer{
		Value: value,
	}, nil
}

func (s LinearScale) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.LinearScaleAnswer
	if err := json.Unmarshal(rawValue, &answer); err != nil {
		return nil, fmt.Errorf("invalid linear scale answer in storage: %w", err)
	}

	return answer, nil
}

func (s LinearScale) EncodeRequest(answer any) (json.RawMessage, error) {
	linearScaleAnswer, ok := answer.(shared.LinearScaleAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.LinearScaleAnswer, got %T", answer)
	}

	// API expects int32 value
	return json.Marshal(linearScaleAnswer.Value)
}

func (s LinearScale) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := s.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	linearScaleAnswer, ok := answer.(shared.LinearScaleAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.LinearScaleAnswer, got %T", answer)
	}

	return fmt.Sprintf("%d (%d-%d)", linearScaleAnswer.Value, s.MinVal, s.MaxVal), nil
}

func (s LinearScale) MatchesPattern(rawValue json.RawMessage, pattern string) (bool, error) {
	answer, err := s.DecodeStorage(rawValue)
	if err != nil {
		return false, fmt.Errorf("failed to decode linear scale answer: %w", err)
	}

	linearScaleAnswer, ok := answer.(shared.LinearScaleAnswer)
	if !ok {
		return false, fmt.Errorf("expected shared.LinearScaleAnswer, got %T", answer)
	}

	// Match against the string representation of the value
	match, err := matchPattern(fmt.Sprintf("%d", linearScaleAnswer.Value), pattern)
	if err != nil {
		return false, fmt.Errorf("failed to match pattern for linear scale answer: %w", err)
	}
	return match, nil
}

type Rating struct {
	question      Question
	formID        uuid.UUID
	Icon          string
	MinVal        int
	MaxVal        int
	MinValueLabel string
	MaxValueLabel string
}

func NewRating(q Question, formID uuid.UUID) (Rating, error) {
	metadata := q.Metadata
	if metadata == nil {
		return Rating{}, errors.New("metadata is nil")
	}

	rating, err := ExtractRating(metadata)
	if err != nil {
		return Rating{}, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: metadata, Message: "could not extract rating options from metadata"}
	}

	if rating.MinVal >= rating.MaxVal {
		return Rating{}, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: metadata, Message: "minVal must be less than maxVal"}
	}

	if !validIcons[rating.Icon] {
		return Rating{}, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: metadata, Message: "invalid icon"}
	}

	return Rating{
		question:      q,
		formID:        formID,
		Icon:          rating.Icon,
		MinVal:        rating.MinVal,
		MaxVal:        rating.MaxVal,
		MinValueLabel: rating.MinValueLabel,
		MaxValueLabel: rating.MaxValueLabel,
	}, nil
}

func (s Rating) Question() Question { return s.question }

func (s Rating) FormID() uuid.UUID { return s.formID }

func (s Rating) Validate(rawValue json.RawMessage) error {
	var value int
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return fmt.Errorf("invalid rating value format: %w", err)
	}

	if value < s.MinVal || value > s.MaxVal {
		return ErrInvalidScaleValue{
			QuestionID: s.question.ID.String(),
			RawValue:   value,
			Message:    "out of range",
		}
	}

	return nil
}

func (s Rating) DecodeRequest(rawValue json.RawMessage) (any, error) {
	// API sends int32 for rating
	var value int
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return nil, fmt.Errorf("invalid rating value format: %w", err)
	}

	// Validate range
	if value < s.MinVal || value > s.MaxVal {
		return nil, ErrInvalidScaleValue{
			QuestionID: s.question.ID.String(),
			RawValue:   value,
			Message:    fmt.Sprintf("value %d is out of range [%d, %d]", value, s.MinVal, s.MaxVal),
		}
	}

	return shared.RatingAnswer{
		Value: value,
	}, nil
}

func (s Rating) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.RatingAnswer
	if err := json.Unmarshal(rawValue, &answer); err != nil {
		return nil, fmt.Errorf("invalid rating answer in storage: %w", err)
	}

	return answer, nil
}

func (s Rating) EncodeRequest(answer any) (json.RawMessage, error) {
	ratingAnswer, ok := answer.(shared.RatingAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.RatingAnswer, got %T", answer)
	}

	// API expects int32 value
	return json.Marshal(ratingAnswer.Value)
}

func (s Rating) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := s.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	ratingAnswer, ok := answer.(shared.RatingAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.RatingAnswer, got %T", answer)
	}

	return fmt.Sprintf("%d (%d-%d)", ratingAnswer.Value, s.MinVal, s.MaxVal), nil
}

func (s Rating) MatchesPattern(rawValue json.RawMessage, pattern string) (bool, error) {
	answer, err := s.DecodeStorage(rawValue)
	if err != nil {
		return false, fmt.Errorf("failed to decode rating answer: %w", err)
	}

	ratingAnswer, ok := answer.(shared.RatingAnswer)
	if !ok {
		return false, fmt.Errorf("expected shared.RatingAnswer, got %T", answer)
	}

	// Match against the string representation of the value
	match, err := matchPattern(fmt.Sprintf("%d", ratingAnswer.Value), pattern)
	if err != nil {
		return false, fmt.Errorf("failed to match pattern for rating answer: %w", err)
	}
	return match, nil
}

func GenerateLinearScaleMetadata(option ScaleOption) ([]byte, error) {
	if option.MinVal >= option.MaxVal {
		return nil, fmt.Errorf("%w: minVal (%d) must be less than maxVal (%d)", internal.ErrValidationFailed, option.MinVal, option.MaxVal)
	}

	if option.MinVal < 1 || option.MinVal > 10 {
		return nil, fmt.Errorf("%w: minVal must be between 1 and 10, got %d", internal.ErrValidationFailed, option.MinVal)
	}

	if option.MaxVal < 1 || option.MaxVal > 10 {
		return nil, fmt.Errorf("%w: maxVal must be between 1 and 10, got %d", internal.ErrValidationFailed, option.MaxVal)
	}

	metadata := map[string]any{
		"scale": LinearScaleMetadata(option),
	}

	return json.Marshal(metadata)
}

func GenerateRatingMetadata(option ScaleOption) ([]byte, error) {
	if option.Icon == "" {
		return nil, fmt.Errorf("%w: icon is required for rating questions", internal.ErrValidationFailed)
	}

	if option.MinVal >= option.MaxVal {
		return nil, fmt.Errorf("%w: minVal (%d) must be less than maxVal (%d)", internal.ErrValidationFailed, option.MinVal, option.MaxVal)
	}

	if option.MinVal < 1 {
		return nil, fmt.Errorf("%w: minVal must be at least 1 for rating, got %d", internal.ErrValidationFailed, option.MinVal)
	}

	if option.MaxVal > 10 {
		return nil, fmt.Errorf("%w: maxVal must be at most 10 for rating, got %d", internal.ErrValidationFailed, option.MaxVal)
	}

	if !validIcons[option.Icon] {
		return nil, fmt.Errorf("%w: invalid icon: %s", internal.ErrValidationFailed, option.Icon)
	}

	metadata := map[string]any{
		"scale": RatingMetadata(option),
	}

	return json.Marshal(metadata)
}

func ExtractLinearScale(data []byte) (LinearScaleMetadata, error) {
	var partial map[string]json.RawMessage
	if err := json.Unmarshal(data, &partial); err != nil {
		return LinearScaleMetadata{}, fmt.Errorf("could not parse partial json: %w", err)
	}

	var metadata LinearScaleMetadata
	if raw, ok := partial["scale"]; ok {
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return LinearScaleMetadata{}, fmt.Errorf("could not parse linear scale: %w", err)
		}
	}
	return metadata, nil
}

func ExtractRating(data []byte) (RatingMetadata, error) {
	var partial map[string]json.RawMessage
	if err := json.Unmarshal(data, &partial); err != nil {
		return RatingMetadata{}, fmt.Errorf("could not parse partial json: %w", err)
	}

	var metadata RatingMetadata
	if raw, ok := partial["scale"]; ok {
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return RatingMetadata{}, fmt.Errorf("could not parse rating scale: %w", err)
		}
	}
	return metadata, nil
}

package question

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
)

//go:embed icons.json
var iconsJSON []byte

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

func (s LinearScale) Question() Question { return s.question }

func (s LinearScale) FormID() uuid.UUID { return s.formID }

func (s LinearScale) Validate(value string) error {
	num, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return err
	}

	intValue := int(num)
	if intValue < s.MinVal || intValue > s.MaxVal {
		return ErrInvalidScaleValue{
			QuestionID: s.question.ID.String(),
			RawValue:   intValue,
			Message:    "out of range",
		}
	}

	return nil
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

	return LinearScale{
		question:      q,
		formID:        formID,
		MinVal:        linearScale.MinVal,
		MaxVal:        linearScale.MaxVal,
		MinValueLabel: linearScale.MinValueLabel,
		MaxValueLabel: linearScale.MaxValueLabel,
	}, nil
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

func (s Rating) Question() Question { return s.question }

func (s Rating) FormID() uuid.UUID { return s.formID }

func (s Rating) Validate(value string) error {
	num, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return err
	}

	intValue := int(num)
	if intValue < s.MinVal || intValue > s.MaxVal {
		return ErrInvalidScaleValue{
			QuestionID: s.question.ID.String(),
			RawValue:   intValue,
			Message:    "out of range",
		}
	}

	return nil
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

func GenerateLinearScaleMetadata(option ScaleOption) ([]byte, error) {
	if option.MinVal >= option.MaxVal {
		return nil, fmt.Errorf("minVal (%d) must be less than maxVal (%d)", option.MinVal, option.MaxVal)
	}

	if option.MinVal != 0 && option.MaxVal != 1 {
		return nil, ErrMetadataValidate{
			QuestionID: string(QuestionTypeLinearScale),
			RawData:    []byte(fmt.Sprintf("%v", option)),
			Message:    "minVal must be 0 or 1",
		}
	}

	if option.MaxVal < 2 || option.MaxVal > 10 {
		return nil, ErrMetadataValidate{
			QuestionID: string(QuestionTypeLinearScale),
			RawData:    []byte(fmt.Sprintf("%v", option)),
			Message:    "maxVal must be between 2 and 10",
		}
	}

	metadata := map[string]any{
		"scale": LinearScaleMetadata(option),
	}

	return json.Marshal(metadata)
}

func GenerateRatingMetadata(option ScaleOption) ([]byte, error) {
	if option.Icon == "" {
		return nil, errors.New("icon is required for rating questions")
	}

	if option.MinVal >= option.MaxVal {
		return nil, fmt.Errorf("minVal (%d) must be less than maxVal (%d)", option.MinVal, option.MaxVal)
	}

	if option.MinVal != 0 && option.MaxVal != 1 {
		return nil, ErrMetadataValidate{
			QuestionID: string(QuestionTypeRating),
			RawData:    []byte(fmt.Sprintf("%v", option)),
			Message:    "minVal must be 0 or 1",
		}
	}

	if option.MaxVal < 2 || option.MaxVal > 10 {
		return nil, ErrMetadataValidate{
			QuestionID: string(QuestionTypeRating),
			RawData:    []byte(fmt.Sprintf("%v", option)),
			Message:    "maxVal must be between 2 and 10",
		}
	}

	if !validIcons[option.Icon] {
		return nil, ErrMetadataValidate{
			QuestionID: string(QuestionTypeRating),
			RawData:    []byte(fmt.Sprintf("%v", option)),
			Message:    "invalid icon",
		}
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

package question

import (
	"encoding/json"
	"fmt"
	"time"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
)

// DateField wraps an optional date bound for DateOption.MinDate / DateOption.MaxDate
// from JSON and returns a validation error on invalid format
type DateField struct {
	Time *time.Time
}

func (o *DateField) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		o.Time = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		o.Time = nil
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse("2006-01-02", s)
	}
	if err != nil {
		return fmt.Errorf("%w: invalid date format for minDate/maxDate: %v", internal.ErrValidationFailed, err)
	}
	o.Time = &t
	return nil
}

func (o DateField) MarshalJSON() ([]byte, error) {
	if o.Time == nil {
		return []byte("null"), nil
	}
	return o.Time.MarshalJSON()
}

// DateOption is the API wire shape for date-question config.
// Used on Request.date (create/update) and Response.date (ToResponse; DATE questions only).
//
// MinDate and MaxDate use DateField (not *time.Time) for lenient client input and explicit
// JSON null on output. GenerateDateMetadata copies DateField.Time into DateMetadata for DB.
//
// Empty bounds (Time nil): accepted on input; stored without that key; marshaled as JSON null
// on output. Only one side may be set. min <= max is checked only when both are set.
type DateOption struct {
	HasYear  bool      `json:"hasYear"`
	HasMonth bool      `json:"hasMonth"`
	HasDay   bool      `json:"hasDay"`
	MinDate  DateField `json:"minDate"` // optional lower bound; nil Time = no minimum
	MaxDate  DateField `json:"maxDate"` // optional upper bound; nil Time = no maximum
}

// DateMetadata is the persisted shape inside question.Metadata, nested as {"date": ...}.
// Written by GenerateDateMetadata from DateOption; read by ExtractDateMetadata and NewDate.
//
// Uses plain *time.Time (not DateField): internal storage only, no custom JSON rules.
// MinDate and MaxDate nil = no bound; omitempty drops unset keys from stored JSON.
// ToResponse maps these back to DateOption DateField values for API output.
type DateMetadata struct {
	HasYear  bool       `json:"hasYear"`
	HasMonth bool       `json:"hasMonth"`
	HasDay   bool       `json:"hasDay"`
	MinDate  *time.Time `json:"minDate,omitempty"` // nil = no minimum date constraint
	MaxDate  *time.Time `json:"maxDate,omitempty"` // nil = no maximum date constraint
}

type Date struct {
	question Question
	formID   uuid.UUID
	HasYear  bool
	HasMonth bool
	HasDay   bool
	MinDate  *time.Time
	MaxDate  *time.Time
}

func NewDate(q Question, formID uuid.UUID) (Answerable, error) {
	metadata := q.Metadata

	// If metadata is nil, use default values (all components enabled)
	if metadata == nil {
		return &Date{
			question: q,
			formID:   formID,
			HasYear:  true,
			HasMonth: true,
			HasDay:   true,
		}, nil
	}

	dateMetadata, err := ExtractDateMetadata(metadata)
	if err != nil {
		return nil, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: metadata, Message: "could not extract date metadata"}
	}

	// Validate that at least one component is enabled
	if !dateMetadata.HasYear && !dateMetadata.HasMonth && !dateMetadata.HasDay {
		return nil, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: metadata, Message: "at least one of hasYear, hasMonth, or hasDay must be true"}
	}

	// Validate date range
	if dateMetadata.MinDate != nil && dateMetadata.MaxDate != nil {
		if dateMetadata.MinDate.After(*dateMetadata.MaxDate) {
			return nil, ErrMetadataBroken{QuestionID: q.ID.String(), RawData: metadata, Message: "minDate must be before or equal to maxDate"}
		}
	}

	return &Date{
		question: q,
		formID:   formID,
		HasYear:  dateMetadata.HasYear,
		HasMonth: dateMetadata.HasMonth,
		HasDay:   dateMetadata.HasDay,
		MinDate:  dateMetadata.MinDate,
		MaxDate:  dateMetadata.MaxDate,
	}, nil
}

func (d Date) Question() Question {
	return d.question
}

func (d Date) FormID() uuid.UUID {
	return d.formID
}

func (d Date) Validate(rawValue json.RawMessage) error {
	var value string
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return fmt.Errorf("invalid date value format: %w", err)
	}

	// Parse the date string to get a time.Time object for validation
	var parsedTime time.Time
	var err error

	// Try parsing as ISO 8601 datetime (RFC3339) first
	parsedTime, err = time.Parse(time.RFC3339, value)
	if err != nil {
		// Fall back to date-only format (YYYY-MM-DD)
		parsedTime, err = time.Parse("2006-01-02", value)
		if err != nil {
			return ErrInvalidDateFormat{
				QuestionID: d.question.ID.String(),
				RawValue:   value,
				Message:    "invalid date format, expected ISO 8601 format (YYYY-MM-DD or RFC3339)",
			}
		}
	}

	// Validate against date range constraints
	if d.MinDate != nil {
		// Truncate to day for comparison
		minDay := time.Date(d.MinDate.Year(), d.MinDate.Month(), d.MinDate.Day(), 0, 0, 0, 0, time.UTC)
		parsedDay := time.Date(parsedTime.Year(), parsedTime.Month(), parsedTime.Day(), 0, 0, 0, 0, time.UTC)

		if parsedDay.Before(minDay) {
			return ErrInvalidDateValue{
				QuestionID: d.question.ID.String(),
				Message:    fmt.Sprintf("date %s is before minimum allowed date %s", parsedDay.Format("2006-01-02"), minDay.Format("2006-01-02")),
			}
		}
	}

	if d.MaxDate != nil {
		// Truncate to day for comparison
		maxDay := time.Date(d.MaxDate.Year(), d.MaxDate.Month(), d.MaxDate.Day(), 0, 0, 0, 0, time.UTC)
		parsedDay := time.Date(parsedTime.Year(), parsedTime.Month(), parsedTime.Day(), 0, 0, 0, 0, time.UTC)

		if parsedDay.After(maxDay) {
			return ErrInvalidDateValue{
				QuestionID: d.question.ID.String(),
				Message:    fmt.Sprintf("date %s is after maximum allowed date %s", parsedDay.Format("2006-01-02"), maxDay.Format("2006-01-02")),
			}
		}
	}

	return nil
}

func (d Date) DecodeRequest(param shared.AnswerParam) (any, error) {
	// API sends ISO 8601 date string (e.g., "2024-12-31T00:00:00Z" or "2024-12-31")
	var value string
	if err := json.Unmarshal(param.Value, &value); err != nil {
		return nil, fmt.Errorf("invalid date value format: %w", err)
	}

	// Parse the date string - support both date-only and datetime formats
	var parsedTime time.Time
	var err error

	// Try parsing as ISO 8601 datetime first
	parsedTime, err = time.Parse(time.RFC3339, value)
	if err != nil {
		// Fall back to date-only format
		parsedTime, err = time.Parse("2006-01-02", value)
		if err != nil {
			return nil, ErrInvalidDateFormat{
				QuestionID: d.question.ID.String(),
				RawValue:   value,
				Message:    "invalid date format, expected ISO 8601 format (YYYY-MM-DD or RFC3339)",
			}
		}
	}

	// Validate against date range constraints
	if d.MinDate != nil {
		minDay := time.Date(d.MinDate.Year(), d.MinDate.Month(), d.MinDate.Day(), 0, 0, 0, 0, time.UTC)
		parsedDay := time.Date(parsedTime.Year(), parsedTime.Month(), parsedTime.Day(), 0, 0, 0, 0, time.UTC)

		if parsedDay.Before(minDay) {
			return nil, ErrInvalidDateValue{
				QuestionID: d.question.ID.String(),
				Message:    fmt.Sprintf("date %s is before minimum allowed date %s", parsedDay.Format("2006-01-02"), minDay.Format("2006-01-02")),
			}
		}
	}

	if d.MaxDate != nil {
		maxDay := time.Date(d.MaxDate.Year(), d.MaxDate.Month(), d.MaxDate.Day(), 0, 0, 0, 0, time.UTC)
		parsedDay := time.Date(parsedTime.Year(), parsedTime.Month(), parsedTime.Day(), 0, 0, 0, 0, time.UTC)

		if parsedDay.After(maxDay) {
			return nil, ErrInvalidDateValue{
				QuestionID: d.question.ID.String(),
				Message:    fmt.Sprintf("date %s is after maximum allowed date %s", parsedDay.Format("2006-01-02"), maxDay.Format("2006-01-02")),
			}
		}
	}

	// Build DateAnswer based on which components are enabled in metadata
	answer := shared.DateAnswer{}

	if d.HasYear {
		year := parsedTime.Year()
		answer.Year = &year
	}

	if d.HasMonth {
		month := int(parsedTime.Month())
		answer.Month = &month
	}

	if d.HasDay {
		day := parsedTime.Day()
		answer.Day = &day
	}

	return answer, nil
}

func (d Date) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.DateAnswer
	if err := json.Unmarshal(rawValue, &answer); err != nil {
		return nil, fmt.Errorf("invalid date answer in storage: %w", err)
	}

	// Validate that the stored answer matches the metadata requirements
	if d.HasYear && answer.Year == nil {
		return nil, ErrInvalidDateValue{
			QuestionID: d.question.ID.String(),
			Message:    "year is required but not found in stored answer",
		}
	}

	if d.HasMonth && answer.Month == nil {
		return nil, ErrInvalidDateValue{
			QuestionID: d.question.ID.String(),
			Message:    "month is required but not found in stored answer",
		}
	}

	if d.HasDay && answer.Day == nil {
		return nil, ErrInvalidDateValue{
			QuestionID: d.question.ID.String(),
			Message:    "day is required but not found in stored answer",
		}
	}

	return answer, nil
}

func (d Date) EncodeRequest(answer any) (json.RawMessage, error) {
	dateAnswer, ok := answer.(shared.DateAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.DateAnswer, got %T", answer)
	}

	// Validate that required components are present based on metadata
	if d.HasYear && dateAnswer.Year == nil {
		return nil, fmt.Errorf("year is required for this date question")
	}

	if d.HasMonth && dateAnswer.Month == nil {
		return nil, fmt.Errorf("month is required for this date question")
	}

	if d.HasDay && dateAnswer.Day == nil {
		return nil, fmt.Errorf("day is required for this date question")
	}

	// Build a time.Time object with the provided components
	// Use defaults for missing non-required components
	year := 1970
	month := 1
	day := 1

	if dateAnswer.Year != nil {
		year = *dateAnswer.Year
	}
	if dateAnswer.Month != nil {
		month = *dateAnswer.Month
	}
	if dateAnswer.Day != nil {
		day = *dateAnswer.Day
	}

	// Create time in UTC and format as RFC3339 (ISO 8601)
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

	// Return as ISO 8601 string for API
	return json.Marshal(t.Format(time.RFC3339))
}

func (d Date) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := d.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	dateAnswer, ok := answer.(shared.DateAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.DateAnswer, got %T", answer)
	}

	// Use the String() method from shared.DateAnswer which formats based on available components
	return dateAnswer.String(), nil
}

func (d Date) MatchesPattern(rawValue json.RawMessage, pattern string) (bool, error) {
	display, err := d.DisplayValue(rawValue)
	if err != nil {
		return false, err
	}

	match, err := matchPattern(display, pattern)
	if err != nil {
		return false, fmt.Errorf("failed to match pattern for date answer: %w", err)
	}
	return match, nil
}

// GenerateDateMetadata creates and validates metadata JSON for date questions
func GenerateDateMetadata(option DateOption) ([]byte, error) {
	// Validate that at least one component is required
	if !option.HasYear && !option.HasMonth && !option.HasDay {
		return nil, fmt.Errorf("%w: at least one of hasYear, hasMonth, or hasDay must be true", internal.ErrValidationFailed)
	}

	// Validate date range if both min and max are provided
	if option.MinDate.Time != nil && option.MaxDate.Time != nil {
		if option.MinDate.Time.After(*option.MaxDate.Time) {
			return nil, fmt.Errorf("%w: minDate (%s) must be before or equal to maxDate (%s)",
				internal.ErrValidationFailed, option.MinDate.Time.Format(time.RFC3339), option.MaxDate.Time.Format(time.RFC3339))
		}
	}

	metadata := map[string]any{
		"date": DateMetadata{
			HasYear:  option.HasYear,
			HasMonth: option.HasMonth,
			HasDay:   option.HasDay,
			MinDate:  option.MinDate.Time,
			MaxDate:  option.MaxDate.Time,
		},
	}

	return json.Marshal(metadata)
}

// ExtractDateMetadata extracts date metadata from raw JSON bytes
func ExtractDateMetadata(data []byte) (DateMetadata, error) {
	var partial map[string]json.RawMessage
	if err := json.Unmarshal(data, &partial); err != nil {
		return DateMetadata{}, fmt.Errorf("could not parse partial json: %w", err)
	}

	var metadata DateMetadata
	if raw, ok := partial["date"]; ok {
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return DateMetadata{}, fmt.Errorf("could not parse date metadata: %w", err)
		}
	}

	return metadata, nil
}

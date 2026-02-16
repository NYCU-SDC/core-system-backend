package question

import (
	"encoding/json"
	"fmt"
	"time"

	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
)

type Date struct {
	question Question
	formID   uuid.UUID
}

func NewDate(q Question, formID uuid.UUID) (Answerable, error) {
	return &Date{question: q, formID: formID}, nil
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

	// Try parsing as ISO 8601 datetime (RFC3339) first
	_, err := time.Parse(time.RFC3339, value)
	if err != nil {
		// Fall back to date-only format (YYYY-MM-DD)
		_, err = time.Parse("2006-01-02", value)
		if err != nil {
			return ErrInvalidDateFormat{
				QuestionID: d.question.ID.String(),
				RawValue:   value,
				Message:    "invalid date format, expected ISO 8601 format (YYYY-MM-DD or RFC3339)",
			}
		}
	}

	return nil
}

func (d Date) DecodeRequest(rawValue json.RawMessage) (any, error) {
	// API sends ISO 8601 date string (e.g., "2024-12-31T00:00:00Z" or "2024-12-31")
	var value string
	if err := json.Unmarshal(rawValue, &value); err != nil {
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

	// Convert to shared.DateAnswer format for storage
	year := parsedTime.Year()
	month := int(parsedTime.Month())
	day := parsedTime.Day()

	return shared.DateAnswer{
		Year:  &year,
		Month: &month,
		Day:   &day,
	}, nil
}

func (d Date) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.DateAnswer
	if err := json.Unmarshal(rawValue, &answer); err != nil {
		return nil, fmt.Errorf("invalid date answer in storage: %w", err)
	}

	return answer, nil
}

func (d Date) EncodeRequest(answer any) (json.RawMessage, error) {
	dateAnswer, ok := answer.(shared.DateAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.DateAnswer, got %T", answer)
	}

	// API expects ISO 8601 datetime format (utcDateTime)
	// Convert DateAnswer back to ISO 8601 string
	if dateAnswer.Year == nil {
		return nil, fmt.Errorf("year is required for date answer")
	}

	// Set default values for missing components
	year := *dateAnswer.Year
	month := 1
	day := 1

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
	// TODO: Implement DisplayValue for Date
	return "", fmt.Errorf("DisplayValue not implemented for Date question type")
}

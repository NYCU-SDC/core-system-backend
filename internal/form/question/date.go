package question

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type DateOption struct {
	HasYear  bool      `json:"hasYear"`
	HasMonth bool      `json:"hasMonth"`
	HasDay   bool      `json:"hasDay"`
	MinDate  time.Time `json:"minDate,omitempty"`
	MaxDate  time.Time `json:"maxDate,omitempty"`
}
type Date struct {
	question Question
	formID   uuid.UUID
	HasYear  bool      `json:"hasYear"`
	HasMonth bool      `json:"hasMonth"`
	HasDay   bool      `json:"hasDay"`
	MinDate  time.Time `json:"minDate,omitempty"`
	MaxDate  time.Time `json:"maxDate,omitempty"`
}

func NewDate(q Question, formID uuid.UUID) (Date, error) {
	metadata := q.Metadata
	if metadata == nil || len(metadata) == 0 {
		return Date{}, fmt.Errorf("metadata is nil or empty for date question %s", q.ID.String())
	}

	date, err := ExtractDate(metadata)
	if err != nil {
		return Date{}, fmt.Errorf("could not extract date: %w", err)
	}

	return Date{
		question: q,
		formID:   formID,
		HasYear:  date.HasYear,
		HasMonth: date.HasMonth,
		HasDay:   date.HasDay,
		MinDate:  date.MinDate,
		MaxDate:  date.MaxDate,
	}, nil
}

func (d Date) Question() Question {
	return d.question
}

func (d Date) FormID() uuid.UUID {
	return d.formID
}

func (d Date) Validate(value string) error {
	// Determine expected format based on hasYear/hasMonth/hasDay
	var format string
	var expectedFormat string

	if d.HasYear && d.HasMonth && d.HasDay {
		format = "2006-01-02"
		expectedFormat = "YYYY-MM-DD"
	} else if d.HasYear && d.HasMonth && !d.HasDay {
		format = "2006-01"
		expectedFormat = "YYYY-MM"
	} else if d.HasYear && !d.HasMonth && !d.HasDay {
		format = "2006"
		expectedFormat = "YYYY"
	} else {
		return ErrInvalidDateFormat{
			QuestionID: d.question.ID.String(),
			RawValue:   value,
			Message:    "invalid date configuration: at least hasYear must be true",
		}
	}

	// Parse the date with the expected format
	parsedDate, err := time.Parse(format, value)
	if err != nil {
		return ErrInvalidDateFormat{
			QuestionID: d.question.ID.String(),
			RawValue:   value,
			Message:    fmt.Sprintf("invalid date format, expected %s", expectedFormat),
		}
	}

	// Validate against minDate and maxDate if they are set
	if !d.MinDate.IsZero() {
		// Normalize minDate to the first day based on the format
		minDate := d.normalizeMinDate(d.MinDate)
		if parsedDate.Before(minDate) {
			return ErrInvalidDateFormat{
				QuestionID: d.question.ID.String(),
				RawValue:   value,
				Message:    fmt.Sprintf("date is before minimum allowed date %s", minDate.Format(format)),
			}
		}
	}

	if !d.MaxDate.IsZero() {
		// Normalize maxDate to the last day based on the format
		maxDate := d.normalizeMaxDate(d.MaxDate)
		if parsedDate.After(maxDate) {
			return ErrInvalidDateFormat{
				QuestionID: d.question.ID.String(),
				RawValue:   value,
				Message:    fmt.Sprintf("date is after maximum allowed date %s", maxDate.Format(format)),
			}
		}
	}

	return nil
}

// normalizeMinDate returns the first day of the period for partial dates
func (d Date) normalizeMinDate(t time.Time) time.Time {
	if d.HasYear && d.HasMonth && d.HasDay {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	} else if d.HasYear && d.HasMonth && !d.HasDay {
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	} else if d.HasYear && !d.HasMonth && !d.HasDay {
		return time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return t
}

// normalizeMaxDate returns the last day of the period for partial dates
func (d Date) normalizeMaxDate(t time.Time) time.Time {
	if d.HasYear && d.HasMonth && d.HasDay {
		return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
	} else if d.HasYear && d.HasMonth && !d.HasDay {
		nextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		return nextMonth.Add(-24 * time.Hour)
	} else if d.HasYear && !d.HasMonth && !d.HasDay {
		return time.Date(t.Year(), time.December, 31, 23, 59, 59, 999999999, time.UTC)
	}
	return t
}

func GenerateDateMetadata(option DateOption) ([]byte, error) {
	metadata := map[string]any{
		"date": option,
	}

	return json.Marshal(metadata)
}

func ExtractDate(data []byte) (DateOption, error) {
	var partial map[string]json.RawMessage
	if err := json.Unmarshal(data, &partial); err != nil {
		return DateOption{}, fmt.Errorf("could not parse partial json: %w", err)
	}

	var metadata DateOption
	if raw, ok := partial["date"]; ok {
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return DateOption{}, fmt.Errorf("could not parse date: %w", err)
		}
	}
	return metadata, nil
}

package question

import (
	"encoding/json"
	"testing"
	"time"

	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
)

func TestDate_Validate(t *testing.T) {
	d, _ := NewDate(Question{ID: uuid.New()}, uuid.New())

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
	}{
		{
			name:        "Should accept ISO 8601 date format (YYYY-MM-DD)",
			rawValue:    `"2024-12-31"`,
			shouldError: false,
		},
		{
			name:        "Should accept RFC3339 datetime format",
			rawValue:    `"2024-12-31T00:00:00Z"`,
			shouldError: false,
		},
		{
			name:        "Should accept RFC3339 with timezone",
			rawValue:    `"2024-12-31T15:30:00+08:00"`,
			shouldError: false,
		},
		{
			name:        "Should reject invalid date format",
			rawValue:    `"12/31/2024"`,
			shouldError: true,
		},
		{
			name:        "Should reject invalid JSON",
			rawValue:    `not a string`,
			shouldError: true,
		},
		{
			name:        "Should reject non-string value",
			rawValue:    `123`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := d.Validate(json.RawMessage(tt.rawValue))

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestDate_DecodeRequest(t *testing.T) {
	d, _ := NewDate(Question{ID: uuid.New()}, uuid.New())

	tests := []struct {
		name        string
		rawValue    string
		expected    shared.DateAnswer
		shouldError bool
	}{
		{
			name:     "Should decode ISO 8601 date format",
			rawValue: `"2024-12-31"`,
			expected: shared.DateAnswer{
				Year:  intPtr(2024),
				Month: intPtr(12),
				Day:   intPtr(31),
			},
			shouldError: false,
		},
		{
			name:     "Should decode RFC3339 datetime format",
			rawValue: `"2024-12-31T00:00:00Z"`,
			expected: shared.DateAnswer{
				Year:  intPtr(2024),
				Month: intPtr(12),
				Day:   intPtr(31),
			},
			shouldError: false,
		},
		{
			name:     "Should decode RFC3339 with timezone",
			rawValue: `"2024-05-06T12:20:00-12:00"`,
			expected: shared.DateAnswer{
				Year:  intPtr(2024),
				Month: intPtr(5),
				Day:   intPtr(6),
			},
			shouldError: false,
		},
		{
			name:        "Should return error for invalid date format",
			rawValue:    `"12/31/2024"`,
			shouldError: true,
		},
		{
			name:        "Should return error for invalid JSON",
			rawValue:    `not a string`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := d.DecodeRequest(json.RawMessage(tt.rawValue))

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			answer, ok := result.(shared.DateAnswer)
			if !ok {
				t.Errorf("Expected shared.DateAnswer, got %T", result)
				return
			}

			if !compareDateAnswers(answer, tt.expected) {
				t.Errorf("Expected %+v, got %+v", tt.expected, answer)
			}
		})
	}
}

func TestDate_DecodeStorage(t *testing.T) {
	tests := []struct {
		name        string
		metadata    DateMetadata
		rawValue    string
		expected    shared.DateAnswer
		shouldError bool
	}{
		{
			name: "Should decode stored date answer with all components",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			rawValue: `{"year":2024,"month":12,"day":31}`,
			expected: shared.DateAnswer{
				Year:  intPtr(2024),
				Month: intPtr(12),
				Day:   intPtr(31),
			},
			shouldError: false,
		},
		{
			name: "Should decode stored date answer with only year",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: false,
				HasDay:   false,
			},
			rawValue: `{"year":2024}`,
			expected: shared.DateAnswer{
				Year: intPtr(2024),
			},
			shouldError: false,
		},
		{
			name: "Should decode stored date answer with year and month",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   false,
			},
			rawValue: `{"year":2024,"month":5}`,
			expected: shared.DateAnswer{
				Year:  intPtr(2024),
				Month: intPtr(5),
			},
			shouldError: false,
		},
		{
			name: "Should return error when required year is missing",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			rawValue:    `{"month":5,"day":10}`,
			shouldError: true,
		},
		{
			name: "Should return error when required month is missing",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			rawValue:    `{"year":2024,"day":10}`,
			shouldError: true,
		},
		{
			name: "Should return error when required day is missing",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			rawValue:    `{"year":2024,"month":5}`,
			shouldError: true,
		},
		{
			name: "Should return error for invalid JSON structure",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			rawValue:    `{invalid}`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create metadata
			metadataBytes, _ := json.Marshal(map[string]any{"date": tt.metadata})

			d, err := NewDate(Question{
				ID:       uuid.New(),
				Metadata: metadataBytes,
			}, uuid.New())
			if err != nil {
				t.Fatalf("Failed to create Date: %v", err)
			}

			result, err := d.DecodeStorage(json.RawMessage(tt.rawValue))

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			answer, ok := result.(shared.DateAnswer)
			if !ok {
				t.Errorf("Expected shared.DateAnswer, got %T", result)
				return
			}

			if !compareDateAnswers(answer, tt.expected) {
				t.Errorf("Expected %+v, got %+v", tt.expected, answer)
			}
		})
	}
}

func TestDate_EncodeRequest(t *testing.T) {
	tests := []struct {
		name        string
		metadata    DateMetadata
		answer      any
		expected    string // Expected RFC3339 date string
		shouldError bool
	}{
		{
			name: "Should encode complete date answer",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			answer: shared.DateAnswer{
				Year:  intPtr(2024),
				Month: intPtr(12),
				Day:   intPtr(31),
			},
			expected:    `"2024-12-31T00:00:00Z"`,
			shouldError: false,
		},
		{
			name: "Should encode date answer with only year",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: false,
				HasDay:   false,
			},
			answer: shared.DateAnswer{
				Year: intPtr(2024),
			},
			expected:    `"2024-01-01T00:00:00Z"`, // Defaults to Jan 1
			shouldError: false,
		},
		{
			name: "Should encode date answer with year and month",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   false,
			},
			answer: shared.DateAnswer{
				Year:  intPtr(2024),
				Month: intPtr(5),
			},
			expected:    `"2024-05-01T00:00:00Z"`, // Defaults to day 1
			shouldError: false,
		},
		{
			name: "Should return error when required year is missing",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			answer: shared.DateAnswer{
				Month: intPtr(12),
				Day:   intPtr(31),
			},
			shouldError: true,
		},
		{
			name: "Should return error when required month is missing",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			answer: shared.DateAnswer{
				Year: intPtr(2024),
				Day:  intPtr(31),
			},
			shouldError: true,
		},
		{
			name: "Should return error when required day is missing",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			answer: shared.DateAnswer{
				Year:  intPtr(2024),
				Month: intPtr(12),
			},
			shouldError: true,
		},
		{
			name: "Should return error for wrong type",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			answer:      "not a date answer",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create metadata
			metadataBytes, _ := json.Marshal(map[string]any{"date": tt.metadata})

			d, err := NewDate(Question{
				ID:       uuid.New(),
				Metadata: metadataBytes,
			}, uuid.New())
			if err != nil {
				t.Fatalf("Failed to create Date: %v", err)
			}

			result, err := d.EncodeRequest(tt.answer)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			resultStr := string(result)
			if resultStr != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, resultStr)
			}
		})
	}
}

// Helper functions

func intPtr(i int) *int {
	return &i
}

func compareDateAnswers(a, b shared.DateAnswer) bool {
	if (a.Year == nil) != (b.Year == nil) {
		return false
	}
	if a.Year != nil && *a.Year != *b.Year {
		return false
	}

	if (a.Month == nil) != (b.Month == nil) {
		return false
	}
	if a.Month != nil && *a.Month != *b.Month {
		return false
	}

	if (a.Day == nil) != (b.Day == nil) {
		return false
	}
	if a.Day != nil && *a.Day != *b.Day {
		return false
	}

	return true
}

func TestGenerateDateMetadata(t *testing.T) {
	tests := []struct {
		name        string
		option      DateOption
		shouldError bool
	}{
		{
			name: "Should generate metadata with all components enabled",
			option: DateOption{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			shouldError: false,
		},
		{
			name: "Should generate metadata with only year",
			option: DateOption{
				HasYear:  true,
				HasMonth: false,
				HasDay:   false,
			},
			shouldError: false,
		},
		{
			name: "Should generate metadata with year and month",
			option: DateOption{
				HasYear:  true,
				HasMonth: true,
				HasDay:   false,
			},
			shouldError: false,
		},
		{
			name: "Should generate metadata with min and max dates",
			option: DateOption{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
				MinDate:  timePtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
				MaxDate:  timePtr(time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)),
			},
			shouldError: false,
		},
		{
			name: "Should return error when no components are enabled",
			option: DateOption{
				HasYear:  false,
				HasMonth: false,
				HasDay:   false,
			},
			shouldError: true,
		},
		{
			name: "Should return error when minDate is after maxDate",
			option: DateOption{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
				MinDate:  timePtr(time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)),
				MaxDate:  timePtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GenerateDateMetadata(tt.option)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("Expected non-nil result")
				return
			}

			// Verify the generated metadata can be extracted
			extracted, err := ExtractDateMetadata(result)
			if err != nil {
				t.Errorf("Failed to extract metadata: %v", err)
				return
			}

			if extracted.HasYear != tt.option.HasYear {
				t.Errorf("HasYear: expected %v, got %v", tt.option.HasYear, extracted.HasYear)
			}
			if extracted.HasMonth != tt.option.HasMonth {
				t.Errorf("HasMonth: expected %v, got %v", tt.option.HasMonth, extracted.HasMonth)
			}
			if extracted.HasDay != tt.option.HasDay {
				t.Errorf("HasDay: expected %v, got %v", tt.option.HasDay, extracted.HasDay)
			}
		})
	}
}

func TestExtractDateMetadata(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		expected    DateMetadata
		shouldError bool
	}{
		{
			name: "Should extract date metadata with all components",
			data: `{"date":{"hasYear":true,"hasMonth":true,"hasDay":true}}`,
			expected: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			shouldError: false,
		},
		{
			name: "Should extract date metadata with only year",
			data: `{"date":{"hasYear":true,"hasMonth":false,"hasDay":false}}`,
			expected: DateMetadata{
				HasYear:  true,
				HasMonth: false,
				HasDay:   false,
			},
			shouldError: false,
		},
		{
			name:        "Should return error for invalid JSON",
			data:        `{invalid}`,
			shouldError: true,
		},
		{
			name:        "Should return error for invalid date structure",
			data:        `{"date":"not an object"}`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractDateMetadata([]byte(tt.data))

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result.HasYear != tt.expected.HasYear {
				t.Errorf("HasYear: expected %v, got %v", tt.expected.HasYear, result.HasYear)
			}
			if result.HasMonth != tt.expected.HasMonth {
				t.Errorf("HasMonth: expected %v, got %v", tt.expected.HasMonth, result.HasMonth)
			}
			if result.HasDay != tt.expected.HasDay {
				t.Errorf("HasDay: expected %v, got %v", tt.expected.HasDay, result.HasDay)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestDate_ValidateWithDateRange(t *testing.T) {
	minDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	maxDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	metadata := DateMetadata{
		HasYear:  true,
		HasMonth: true,
		HasDay:   true,
		MinDate:  &minDate,
		MaxDate:  &maxDate,
	}
	metadataBytes, _ := json.Marshal(map[string]any{"date": metadata})

	d, err := NewDate(Question{
		ID:       uuid.New(),
		Metadata: metadataBytes,
	}, uuid.New())
	if err != nil {
		t.Fatalf("Failed to create Date: %v", err)
	}

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
	}{
		{
			name:        "Should accept date within range",
			rawValue:    `"2024-06-15"`,
			shouldError: false,
		},
		{
			name:        "Should accept minimum date",
			rawValue:    `"2024-01-01"`,
			shouldError: false,
		},
		{
			name:        "Should accept maximum date",
			rawValue:    `"2024-12-31"`,
			shouldError: false,
		},
		{
			name:        "Should reject date before minimum",
			rawValue:    `"2023-12-31"`,
			shouldError: true,
		},
		{
			name:        "Should reject date after maximum",
			rawValue:    `"2025-01-01"`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := d.Validate(json.RawMessage(tt.rawValue))

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestDate_DecodeRequestWithDateRange(t *testing.T) {
	minDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	maxDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	metadata := DateMetadata{
		HasYear:  true,
		HasMonth: true,
		HasDay:   true,
		MinDate:  &minDate,
		MaxDate:  &maxDate,
	}
	metadataBytes, _ := json.Marshal(map[string]any{"date": metadata})

	d, err := NewDate(Question{
		ID:       uuid.New(),
		Metadata: metadataBytes,
	}, uuid.New())
	if err != nil {
		t.Fatalf("Failed to create Date: %v", err)
	}

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
	}{
		{
			name:        "Should decode date within range",
			rawValue:    `"2024-06-15"`,
			shouldError: false,
		},
		{
			name:        "Should reject date before minimum",
			rawValue:    `"2023-12-31"`,
			shouldError: true,
		},
		{
			name:        "Should reject date after maximum",
			rawValue:    `"2025-01-01"`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := d.DecodeRequest(json.RawMessage(tt.rawValue))

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestDate_DecodeRequestWithPartialComponents(t *testing.T) {
	tests := []struct {
		name        string
		metadata    DateMetadata
		rawValue    string
		expected    shared.DateAnswer
		shouldError bool
	}{
		{
			name: "Should decode with only year component enabled",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: false,
				HasDay:   false,
			},
			rawValue: `"2024-06-15"`,
			expected: shared.DateAnswer{
				Year: intPtr(2024),
			},
			shouldError: false,
		},
		{
			name: "Should decode with year and month components enabled",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   false,
			},
			rawValue: `"2024-06-15"`,
			expected: shared.DateAnswer{
				Year:  intPtr(2024),
				Month: intPtr(6),
			},
			shouldError: false,
		},
		{
			name: "Should decode with all components enabled",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			rawValue: `"2024-06-15"`,
			expected: shared.DateAnswer{
				Year:  intPtr(2024),
				Month: intPtr(6),
				Day:   intPtr(15),
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadataBytes, _ := json.Marshal(map[string]any{"date": tt.metadata})

			d, err := NewDate(Question{
				ID:       uuid.New(),
				Metadata: metadataBytes,
			}, uuid.New())
			if err != nil {
				t.Fatalf("Failed to create Date: %v", err)
			}

			result, err := d.DecodeRequest(json.RawMessage(tt.rawValue))

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			answer, ok := result.(shared.DateAnswer)
			if !ok {
				t.Errorf("Expected shared.DateAnswer, got %T", result)
				return
			}

			if !compareDateAnswers(answer, tt.expected) {
				t.Errorf("Expected %+v, got %+v", tt.expected, answer)
			}
		})
	}
}

func TestDate_DisplayValue(t *testing.T) {
	tests := []struct {
		name        string
		metadata    DateMetadata
		rawValue    string
		expected    string
		shouldError bool
	}{
		{
			name: "Should display full date",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			},
			rawValue:    `{"year":2024,"month":12,"day":31}`,
			expected:    "2024-12-31",
			shouldError: false,
		},
		{
			name: "Should display year and month",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: true,
				HasDay:   false,
			},
			rawValue:    `{"year":2024,"month":5}`,
			expected:    "2024-05",
			shouldError: false,
		},
		{
			name: "Should display only year",
			metadata: DateMetadata{
				HasYear:  true,
				HasMonth: false,
				HasDay:   false,
			},
			rawValue:    `{"year":2024}`,
			expected:    "2024",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadataBytes, _ := json.Marshal(map[string]any{"date": tt.metadata})

			d, err := NewDate(Question{
				ID:       uuid.New(),
				Metadata: metadataBytes,
			}, uuid.New())
			if err != nil {
				t.Fatalf("Failed to create Date: %v", err)
			}

			result, err := d.DisplayValue(json.RawMessage(tt.rawValue))

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

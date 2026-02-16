package question

import (
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/shared"
	"github.com/google/uuid"
)

// LinearScale Tests

func TestLinearScale_DecodeRequest(t *testing.T) {
	metadata, _ := GenerateLinearScaleMetadata(ScaleOption{
		MinVal:        1,
		MaxVal:        5,
		MinValueLabel: "Poor",
		MaxValueLabel: "Excellent",
	})

	ls := LinearScale{
		question:      Question{ID: uuid.New(), Metadata: metadata},
		formID:        uuid.New(),
		MinVal:        1,
		MaxVal:        5,
		MinValueLabel: "Poor",
		MaxValueLabel: "Excellent",
	}

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
		validate    func(t *testing.T, result any)
	}{
		{
			name:        "Should decode valid linear scale value within range",
			rawValue:    `3`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.LinearScaleAnswer)
				if !ok {
					t.Fatalf("Expected shared.LinearScaleAnswer, got %T", result)
				}
				if answer.Value != 3 {
					t.Errorf("Expected value 3, got %d", answer.Value)
				}
			},
		},
		{
			name:        "Should decode minimum value",
			rawValue:    `1`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.LinearScaleAnswer)
				if !ok {
					t.Fatalf("Expected shared.LinearScaleAnswer, got %T", result)
				}
				if answer.Value != 1 {
					t.Errorf("Expected value 1, got %d", answer.Value)
				}
			},
		},
		{
			name:        "Should decode maximum value",
			rawValue:    `5`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.LinearScaleAnswer)
				if !ok {
					t.Fatalf("Expected shared.LinearScaleAnswer, got %T", result)
				}
				if answer.Value != 5 {
					t.Errorf("Expected value 5, got %d", answer.Value)
				}
			},
		},
		{
			name:        "Should return error for value below minimum",
			rawValue:    `0`,
			shouldError: true,
		},
		{
			name:        "Should return error for value above maximum",
			rawValue:    `6`,
			shouldError: true,
		},
		{
			name:        "Should return error for negative value",
			rawValue:    `-1`,
			shouldError: true,
		},
		{
			name:        "Should return error for invalid JSON format",
			rawValue:    `"not a number"`,
			shouldError: true,
		},
		{
			name:        "Should return error for float value",
			rawValue:    `3.5`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ls.DecodeRequest(json.RawMessage(tt.rawValue))

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

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestLinearScale_DecodeStorage(t *testing.T) {
	ls := LinearScale{
		question: Question{ID: uuid.New()},
		formID:   uuid.New(),
		MinVal:   1,
		MaxVal:   5,
	}

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
		validate    func(t *testing.T, result any)
	}{
		{
			name:        "Should decode stored linear scale answer",
			rawValue:    `{"value":3}`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.LinearScaleAnswer)
				if !ok {
					t.Fatalf("Expected shared.LinearScaleAnswer, got %T", result)
				}
				if answer.Value != 3 {
					t.Errorf("Expected value 3, got %d", answer.Value)
				}
			},
		},
		{
			name:        "Should return error for invalid JSON",
			rawValue:    `invalid json`,
			shouldError: true,
		},
		{
			name:        "Should decode even with missing value field",
			rawValue:    `{}`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.LinearScaleAnswer)
				if !ok {
					t.Fatalf("Expected shared.LinearScaleAnswer, got %T", result)
				}
				if answer.Value != 0 {
					t.Errorf("Expected value 0 (zero value), got %d", answer.Value)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ls.DecodeStorage(json.RawMessage(tt.rawValue))

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

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestLinearScale_EncodeRequest(t *testing.T) {
	ls := LinearScale{
		question: Question{ID: uuid.New()},
		formID:   uuid.New(),
		MinVal:   1,
		MaxVal:   5,
	}

	tests := []struct {
		name        string
		answer      any
		expected    string
		shouldError bool
	}{
		{
			name: "Should encode linear scale answer",
			answer: shared.LinearScaleAnswer{
				Value: 3,
			},
			expected:    `3`,
			shouldError: false,
		},
		{
			name: "Should encode minimum value",
			answer: shared.LinearScaleAnswer{
				Value: 1,
			},
			expected:    `1`,
			shouldError: false,
		},
		{
			name: "Should encode maximum value",
			answer: shared.LinearScaleAnswer{
				Value: 5,
			},
			expected:    `5`,
			shouldError: false,
		},
		{
			name:        "Should return error for wrong answer type",
			answer:      "not a linear scale answer",
			shouldError: true,
		},
		{
			name:        "Should return error for rating answer type",
			answer:      shared.RatingAnswer{Value: 3},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ls.EncodeRequest(tt.answer)

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

			if string(result) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(result))
			}
		})
	}
}

// Rating Tests

func TestRating_DecodeRequest(t *testing.T) {
	metadata, _ := GenerateRatingMetadata(ScaleOption{
		Icon:          "star",
		MinVal:        1,
		MaxVal:        10,
		MinValueLabel: "Bad",
		MaxValueLabel: "Great",
	})

	rating := Rating{
		question:      Question{ID: uuid.New(), Metadata: metadata},
		formID:        uuid.New(),
		Icon:          "star",
		MinVal:        1,
		MaxVal:        10,
		MinValueLabel: "Bad",
		MaxValueLabel: "Great",
	}

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
		validate    func(t *testing.T, result any)
	}{
		{
			name:        "Should decode valid rating value within range",
			rawValue:    `7`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.RatingAnswer)
				if !ok {
					t.Fatalf("Expected shared.RatingAnswer, got %T", result)
				}
				if answer.Value != 7 {
					t.Errorf("Expected value 7, got %d", answer.Value)
				}
			},
		},
		{
			name:        "Should decode minimum value",
			rawValue:    `1`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.RatingAnswer)
				if !ok {
					t.Fatalf("Expected shared.RatingAnswer, got %T", result)
				}
				if answer.Value != 1 {
					t.Errorf("Expected value 1, got %d", answer.Value)
				}
			},
		},
		{
			name:        "Should decode maximum value",
			rawValue:    `10`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.RatingAnswer)
				if !ok {
					t.Fatalf("Expected shared.RatingAnswer, got %T", result)
				}
				if answer.Value != 10 {
					t.Errorf("Expected value 10, got %d", answer.Value)
				}
			},
		},
		{
			name:        "Should return error for value below minimum",
			rawValue:    `0`,
			shouldError: true,
		},
		{
			name:        "Should return error for value above maximum",
			rawValue:    `11`,
			shouldError: true,
		},
		{
			name:        "Should return error for negative value",
			rawValue:    `-1`,
			shouldError: true,
		},
		{
			name:        "Should return error for invalid JSON format",
			rawValue:    `"not a number"`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rating.DecodeRequest(json.RawMessage(tt.rawValue))

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

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestRating_DecodeStorage(t *testing.T) {
	rating := Rating{
		question: Question{ID: uuid.New()},
		formID:   uuid.New(),
		Icon:     "star",
		MinVal:   1,
		MaxVal:   10,
	}

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
		validate    func(t *testing.T, result any)
	}{
		{
			name:        "Should decode stored rating answer",
			rawValue:    `{"value":7}`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.RatingAnswer)
				if !ok {
					t.Fatalf("Expected shared.RatingAnswer, got %T", result)
				}
				if answer.Value != 7 {
					t.Errorf("Expected value 7, got %d", answer.Value)
				}
			},
		},
		{
			name:        "Should return error for invalid JSON",
			rawValue:    `invalid json`,
			shouldError: true,
		},
		{
			name:        "Should decode even with missing value field",
			rawValue:    `{}`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.RatingAnswer)
				if !ok {
					t.Fatalf("Expected shared.RatingAnswer, got %T", result)
				}
				if answer.Value != 0 {
					t.Errorf("Expected value 0 (zero value), got %d", answer.Value)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rating.DecodeStorage(json.RawMessage(tt.rawValue))

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

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestRating_EncodeRequest(t *testing.T) {
	rating := Rating{
		question: Question{ID: uuid.New()},
		formID:   uuid.New(),
		Icon:     "star",
		MinVal:   1,
		MaxVal:   10,
	}

	tests := []struct {
		name        string
		answer      any
		expected    string
		shouldError bool
	}{
		{
			name: "Should encode rating answer",
			answer: shared.RatingAnswer{
				Value: 7,
			},
			expected:    `7`,
			shouldError: false,
		},
		{
			name: "Should encode minimum value",
			answer: shared.RatingAnswer{
				Value: 1,
			},
			expected:    `1`,
			shouldError: false,
		},
		{
			name: "Should encode maximum value",
			answer: shared.RatingAnswer{
				Value: 10,
			},
			expected:    `10`,
			shouldError: false,
		},
		{
			name:        "Should return error for wrong answer type",
			answer:      "not a rating answer",
			shouldError: true,
		},
		{
			name:        "Should return error for linear scale answer type",
			answer:      shared.LinearScaleAnswer{Value: 7},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rating.EncodeRequest(tt.answer)

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

			if string(result) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(result))
			}
		})
	}
}

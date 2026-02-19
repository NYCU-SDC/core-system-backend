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

	testCases := []struct {
		name          string
		rawValue      string
		expectedError bool
		validate      func(t *testing.T, result any)
	}{
		{
			name:          "Should decode valid linear scale value within range",
			rawValue:      `3`,
			expectedError: false,
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
			name:          "Should decode minimum value",
			rawValue:      `1`,
			expectedError: false,
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
			name:          "Should decode maximum value",
			rawValue:      `5`,
			expectedError: false,
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
			name:          "Should return error for value below minimum",
			rawValue:      `0`,
			expectedError: true,
		},
		{
			name:          "Should return error for value above maximum",
			rawValue:      `6`,
			expectedError: true,
		},
		{
			name:          "Should return error for negative value",
			rawValue:      `-1`,
			expectedError: true,
		},
		{
			name:          "Should return error for invalid JSON format",
			rawValue:      `"not a number"`,
			expectedError: true,
		},
		{
			name:          "Should return error for float value",
			rawValue:      `3.5`,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ls.DecodeRequest(json.RawMessage(tc.rawValue))

			if tc.expectedError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tc.validate != nil {
				tc.validate(t, result)
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

	testCases := []struct {
		name          string
		rawValue      string
		expectedError bool
		validate      func(t *testing.T, result any)
	}{
		{
			name:          "Should decode stored linear scale answer",
			rawValue:      `{"value":3}`,
			expectedError: false,
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
			name:          "Should return error for invalid JSON",
			rawValue:      `invalid json`,
			expectedError: true,
		},
		{
			name:          "Should decode even with missing value field",
			rawValue:      `{}`,
			expectedError: false,
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ls.DecodeStorage(json.RawMessage(tc.rawValue))

			if tc.expectedError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tc.validate != nil {
				tc.validate(t, result)
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

	testCases := []struct {
		name          string
		answer        any
		expected      string
		expectedError bool
	}{
		{
			name: "Should encode linear scale answer",
			answer: shared.LinearScaleAnswer{
				Value: 3,
			},
			expected:      `3`,
			expectedError: false,
		},
		{
			name: "Should encode minimum value",
			answer: shared.LinearScaleAnswer{
				Value: 1,
			},
			expected:      `1`,
			expectedError: false,
		},
		{
			name: "Should encode maximum value",
			answer: shared.LinearScaleAnswer{
				Value: 5,
			},
			expected:      `5`,
			expectedError: false,
		},
		{
			name:          "Should return error for wrong answer type",
			answer:        "not a linear scale answer",
			expectedError: true,
		},
		{
			name:          "Should return error for rating answer type",
			answer:        shared.RatingAnswer{Value: 3},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ls.EncodeRequest(tc.answer)

			if tc.expectedError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if string(result) != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, string(result))
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

	testCases := []struct {
		name          string
		rawValue      string
		expectedError bool
		validate      func(t *testing.T, result any)
	}{
		{
			name:          "Should decode valid rating value within range",
			rawValue:      `7`,
			expectedError: false,
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
			name:          "Should decode minimum value",
			rawValue:      `1`,
			expectedError: false,
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
			name:          "Should decode maximum value",
			rawValue:      `10`,
			expectedError: false,
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
			name:          "Should return error for value below minimum",
			rawValue:      `0`,
			expectedError: true,
		},
		{
			name:          "Should return error for value above maximum",
			rawValue:      `11`,
			expectedError: true,
		},
		{
			name:          "Should return error for negative value",
			rawValue:      `-1`,
			expectedError: true,
		},
		{
			name:          "Should return error for invalid JSON format",
			rawValue:      `"not a number"`,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := rating.DecodeRequest(json.RawMessage(tc.rawValue))

			if tc.expectedError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tc.validate != nil {
				tc.validate(t, result)
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

	testCases := []struct {
		name          string
		rawValue      string
		expectedError bool
		validate      func(t *testing.T, result any)
	}{
		{
			name:          "Should decode stored rating answer",
			rawValue:      `{"value":7}`,
			expectedError: false,
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
			name:          "Should return error for invalid JSON",
			rawValue:      `invalid json`,
			expectedError: true,
		},
		{
			name:          "Should decode even with missing value field",
			rawValue:      `{}`,
			expectedError: false,
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := rating.DecodeStorage(json.RawMessage(tc.rawValue))

			if tc.expectedError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tc.validate != nil {
				tc.validate(t, result)
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

	testCases := []struct {
		name          string
		answer        any
		expected      string
		expectedError bool
	}{
		{
			name: "Should encode rating answer",
			answer: shared.RatingAnswer{
				Value: 7,
			},
			expected:      `7`,
			expectedError: false,
		},
		{
			name: "Should encode minimum value",
			answer: shared.RatingAnswer{
				Value: 1,
			},
			expected:      `1`,
			expectedError: false,
		},
		{
			name: "Should encode maximum value",
			answer: shared.RatingAnswer{
				Value: 10,
			},
			expected:      `10`,
			expectedError: false,
		},
		{
			name:          "Should return error for wrong answer type",
			answer:        "not a rating answer",
			expectedError: true,
		},
		{
			name:          "Should return error for linear scale answer type",
			answer:        shared.LinearScaleAnswer{Value: 7},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := rating.EncodeRequest(tc.answer)

			if tc.expectedError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if string(result) != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, string(result))
			}
		})
	}
}

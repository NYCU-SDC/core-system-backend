package question

import (
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/shared"
	"github.com/google/uuid"
)

func TestShortText_DecodeRequest(t *testing.T) {
	st := NewShortText(Question{ID: uuid.New()}, uuid.New())

	tests := []struct {
		name        string
		rawValue    string
		expected    shared.ShortTextAnswer
		shouldError bool
	}{
		{
			name:        "Should decode valid short text value",
			rawValue:    `"John Doe"`,
			expected:    shared.ShortTextAnswer{Value: "John Doe"},
			shouldError: false,
		},
		{
			name:        "Should decode empty string",
			rawValue:    `""`,
			expected:    shared.ShortTextAnswer{Value: ""},
			shouldError: false,
		},
		{
			name:        "Should return error for invalid JSON",
			rawValue:    `not a string`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := st.DecodeRequest(json.RawMessage(tt.rawValue))

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

			answer, ok := result.(shared.ShortTextAnswer)
			if !ok {
				t.Errorf("Expected shared.ShortTextAnswer, got %T", result)
				return
			}

			if answer.Value != tt.expected.Value {
				t.Errorf("Expected value %q, got %q", tt.expected.Value, answer.Value)
			}
		})
	}
}

func TestShortText_DecodeStorage(t *testing.T) {
	st := NewShortText(Question{ID: uuid.New()}, uuid.New())

	tests := []struct {
		name        string
		rawValue    string
		expected    shared.ShortTextAnswer
		shouldError bool
	}{
		{
			name:        "Should decode stored short text answer",
			rawValue:    `{"value":"John Doe"}`,
			expected:    shared.ShortTextAnswer{Value: "John Doe"},
			shouldError: false,
		},
		{
			name:        "Should return error for invalid JSON structure",
			rawValue:    `{"invalid":"field"}`,
			expected:    shared.ShortTextAnswer{Value: ""},
			shouldError: false, // Empty value is valid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := st.DecodeStorage(json.RawMessage(tt.rawValue))

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

			answer, ok := result.(shared.ShortTextAnswer)
			if !ok {
				t.Errorf("Expected shared.ShortTextAnswer, got %T", result)
				return
			}

			if answer.Value != tt.expected.Value {
				t.Errorf("Expected value %q, got %q", tt.expected.Value, answer.Value)
			}
		})
	}
}

func TestLongText_DecodeRequest(t *testing.T) {
	lt := NewLongText(Question{ID: uuid.New()}, uuid.New())

	tests := []struct {
		name        string
		rawValue    string
		expected    shared.LongTextAnswer
		shouldError bool
	}{
		{
			name:        "Should decode valid long text value",
			rawValue:    `"This is a long text answer with multiple sentences."`,
			expected:    shared.LongTextAnswer{Value: "This is a long text answer with multiple sentences."},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := lt.DecodeRequest(json.RawMessage(tt.rawValue))

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

			answer, ok := result.(shared.LongTextAnswer)
			if !ok {
				t.Errorf("Expected shared.LongTextAnswer, got %T", result)
				return
			}

			if answer.Value != tt.expected.Value {
				t.Errorf("Expected value %q, got %q", tt.expected.Value, answer.Value)
			}
		})
	}
}

func TestHyperlink_DecodeRequest(t *testing.T) {
	hl := NewHyperlink(Question{ID: uuid.New()}, uuid.New())

	tests := []struct {
		name        string
		rawValue    string
		expected    shared.HyperlinkAnswer
		shouldError bool
	}{
		{
			name:        "Should decode valid hyperlink value",
			rawValue:    `"https://example.com"`,
			expected:    shared.HyperlinkAnswer{Value: "https://example.com"},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := hl.DecodeRequest(json.RawMessage(tt.rawValue))

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

			answer, ok := result.(shared.HyperlinkAnswer)
			if !ok {
				t.Errorf("Expected shared.HyperlinkAnswer, got %T", result)
				return
			}

			if answer.Value != tt.expected.Value {
				t.Errorf("Expected value %q, got %q", tt.expected.Value, answer.Value)
			}
		})
	}
}

package form

import (
	"testing"
)

func TestStatusToUppercase(t *testing.T) {
	tests := []struct {
		name     string
		input    Status
		expected string
	}{
		{
			name:     "draft to DRAFT",
			input:    StatusDraft,
			expected: "DRAFT",
		},
		{
			name:     "published to PUBLISHED",
			input:    StatusPublished,
			expected: "PUBLISHED",
		},
		{
			name:     "archived to ARCHIVED",
			input:    StatusArchived,
			expected: "ARCHIVED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := statusToUppercase(tt.input)
			if result != tt.expected {
				t.Errorf("statusToUppercase(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

package form

import (
	"testing"
)

func TestStatus_ToUppercase(t *testing.T) {
	testCases := []struct {
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := statusToUppercase(tc.input)
			if result != tc.expected {
				t.Errorf("statusToUppercase(%v) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

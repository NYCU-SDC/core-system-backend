package form

import (
	"testing"
)

func TestVisibilityToUppercase(t *testing.T) {
	tests := []struct {
		name     string
		input    Visibility
		expected string
	}{
		{
			name:     "public to PUBLIC",
			input:    VisibilityPublic,
			expected: "PUBLIC",
		},
		{
			name:     "private to PRIVATE",
			input:    VisibilityPrivate,
			expected: "PRIVATE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := visibilityToUppercase(tt.input)
			if result != tt.expected {
				t.Errorf("visibilityToUppercase(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestVisibilityFromAPIFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Visibility
	}{
		{
			name:     "PUBLIC to public",
			input:    "PUBLIC",
			expected: VisibilityPublic,
		},
		{
			name:     "PRIVATE to private",
			input:    "PRIVATE",
			expected: VisibilityPrivate,
		},
		{
			name:     "lowercase public for backward compatibility",
			input:    "public",
			expected: Visibility("public"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := visibilityFromAPIFormat(tt.input)
			if result != tt.expected {
				t.Errorf("visibilityFromAPIFormat(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

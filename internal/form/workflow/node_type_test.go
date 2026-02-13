package workflow

import (
	"testing"
)

func TestNodeTypeToUppercase(t *testing.T) {
	tests := []struct {
		name     string
		input    NodeType
		expected string
	}{
		{
			name:     "section to SECTION",
			input:    NodeTypeSection,
			expected: "SECTION",
		},
		{
			name:     "condition to CONDITION",
			input:    NodeTypeCondition,
			expected: "CONDITION",
		},
		{
			name:     "start to START",
			input:    NodeTypeStart,
			expected: "START",
		},
		{
			name:     "end to END",
			input:    NodeTypeEnd,
			expected: "END",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nodeTypeToUppercase(tt.input)
			if result != tt.expected {
				t.Errorf("nodeTypeToUppercase(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

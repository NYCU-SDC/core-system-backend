package workflow

import (
	"encoding/json"
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

func TestNodeTypeToLowercase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "SECTION to section",
			input:    "SECTION",
			expected: "section",
		},
		{
			name:     "CONDITION to condition",
			input:    "CONDITION",
			expected: "condition",
		},
		{
			name:     "START to start",
			input:    "START",
			expected: "start",
		},
		{
			name:     "END to end",
			input:    "END",
			expected: "end",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nodeTypeToLowercase(tt.input)
			if result != tt.expected {
				t.Errorf("nodeTypeToLowercase(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestWorkflowToAPIFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "convert workflow types to uppercase",
			input: `[
				{"id":"1","type":"start","label":"Start"},
				{"id":"2","type":"section","label":"Section 1"},
				{"id":"3","type":"condition","label":"Condition"},
				{"id":"4","type":"end","label":"End"}
			]`,
			expected: `[
				{"id":"1","type":"START","label":"Start"},
				{"id":"2","type":"SECTION","label":"Section 1"},
				{"id":"3","type":"CONDITION","label":"Condition"},
				{"id":"4","type":"END","label":"End"}
			]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := workflowToAPIFormat([]byte(tt.input))
			if err != nil {
				t.Fatalf("workflowToAPIFormat() error = %v", err)
			}

			// Unmarshal both to compare structure
			var resultNodes, expectedNodes []map[string]interface{}
			if err := json.Unmarshal(result, &resultNodes); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.expected), &expectedNodes); err != nil {
				t.Fatalf("failed to unmarshal expected: %v", err)
			}

			if len(resultNodes) != len(expectedNodes) {
				t.Fatalf("length mismatch: got %d, want %d", len(resultNodes), len(expectedNodes))
			}

			for i := range resultNodes {
				if resultNodes[i]["type"] != expectedNodes[i]["type"] {
					t.Errorf("node %d type mismatch: got %v, want %v", i, resultNodes[i]["type"], expectedNodes[i]["type"])
				}
			}
		})
	}
}

func TestWorkflowFromAPIFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "convert workflow types to lowercase",
			input: `[
				{"id":"1","type":"START","label":"Start"},
				{"id":"2","type":"SECTION","label":"Section 1"},
				{"id":"3","type":"CONDITION","label":"Condition"},
				{"id":"4","type":"END","label":"End"}
			]`,
			expected: `[
				{"id":"1","type":"start","label":"Start"},
				{"id":"2","type":"section","label":"Section 1"},
				{"id":"3","type":"condition","label":"Condition"},
				{"id":"4","type":"end","label":"End"}
			]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := workflowFromAPIFormat([]byte(tt.input))
			if err != nil {
				t.Fatalf("workflowFromAPIFormat() error = %v", err)
			}

			// Unmarshal both to compare structure
			var resultNodes, expectedNodes []map[string]interface{}
			if err := json.Unmarshal(result, &resultNodes); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.expected), &expectedNodes); err != nil {
				t.Fatalf("failed to unmarshal expected: %v", err)
			}

			if len(resultNodes) != len(expectedNodes) {
				t.Fatalf("length mismatch: got %d, want %d", len(resultNodes), len(expectedNodes))
			}

			for i := range resultNodes {
				if resultNodes[i]["type"] != expectedNodes[i]["type"] {
					t.Errorf("node %d type mismatch: got %v, want %v", i, resultNodes[i]["type"], expectedNodes[i]["type"])
				}
			}
		})
	}
}

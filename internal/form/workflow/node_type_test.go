package workflow

import (
	"encoding/json"
	"testing"
)

func TestNodeType_ToUppercase(t *testing.T) {
	testCases := []struct {
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := nodeTypeToUppercase(tc.input)
			if result != tc.expected {
				t.Errorf("nodeTypeToUppercase(%v) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestNodeType_ToLowercase(t *testing.T) {
	testCases := []struct {
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := nodeTypeToLowercase(tc.input)
			if result != tc.expected {
				t.Errorf("nodeTypeToLowercase(%v) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestWorkflow_ToAPIFormat(t *testing.T) {
	testCases := []struct {
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := workflowToAPIFormat([]byte(tc.input))
			if err != nil {
				t.Fatalf("workflowToAPIFormat() error = %v", err)
			}

			// Unmarshal both to compare structure
			var resultNodes, expectedNodes []map[string]interface{}
			err = json.Unmarshal(result, &resultNodes)
			if err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			err = json.Unmarshal([]byte(tc.expected), &expectedNodes)
			if err != nil {
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

func TestWorkflow_FromAPIFormat(t *testing.T) {
	testCases := []struct {
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
		{
			name: "preserve conditionRule (source accepted case-insensitive in validation)",
			input: `[
				{"id":"1","type":"start","label":"Start"},
				{"id":"2","type":"section","label":"Section"},
				{"id":"3","type":"condition","label":"Cond","conditionRule":{"source":"CHOICE","question":"125ff21a-d828-4a55-b58d-9d60c409eba7","pattern":"^opt$"},"nextTrue":"4","nextFalse":"5"},
				{"id":"4","type":"section","label":"A"},
				{"id":"5","type":"end","label":"End"}
			]`,
			expected: "", // structure checked below
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := workflowFromAPIFormat([]byte(tc.input))
			if err != nil {
				t.Fatalf("workflowFromAPIFormat() error = %v", err)
			}

			var resultNodes, expectedNodes []map[string]interface{}
			err = json.Unmarshal(result, &resultNodes)
			if err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			if tc.expected == "" {
				// conditionRule preserved as-is (validation accepts source case-insensitive)
				var nodes []map[string]interface{}
				if err := json.Unmarshal(result, &nodes); err != nil {
					t.Fatalf("unmarshal result: %v", err)
				}
				for _, n := range nodes {
					rule, _ := n["conditionRule"].(map[string]interface{})
					if rule == nil {
						continue
					}
					if q, _ := rule["question"].(string); q != "125ff21a-d828-4a55-b58d-9d60c409eba7" {
						t.Errorf("conditionRule.question = %q, want question ID", q)
					}
					if s, _ := rule["source"].(string); s != "CHOICE" {
						t.Errorf("conditionRule.source = %q, want preserved \"CHOICE\"", s)
					}
				}
				return
			}

			err = json.Unmarshal([]byte(tc.expected), &expectedNodes)
			if err != nil {
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

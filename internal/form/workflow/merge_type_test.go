package workflow

import (
	"encoding/json"
	"testing"
)

func TestWorkflow_MergeTypeFromDB(t *testing.T) {
	testCases := []struct {
		name        string
		apiWorkflow string
		dbWorkflow  string
		expectError bool
	}{
		{
			name: "successful merge - all nodes exist in database",
			apiWorkflow: `[
				{"id":"1","label":"Start Node","next":"2"},
				{"id":"2","label":"Section 1","next":"3"},
				{"id":"3","label":"End Node"}
			]`,
			dbWorkflow: `[
				{"id":"1","type":"start","label":"Start Node"},
				{"id":"2","type":"section","label":"Section 1"},
				{"id":"3","type":"end","label":"End Node"}
			]`,
			expectError: false,
		},
		{
			name: "error - node not found in database",
			apiWorkflow: `[
				{"id":"1","label":"Start Node"},
				{"id":"999","label":"Unknown Node"}
			]`,
			dbWorkflow: `[
				{"id":"1","type":"start","label":"Start Node"}
			]`,
			expectError: true,
		},
		{
			name: "successful merge - with condition node",
			apiWorkflow: `[
				{"id":"1","label":"Start","next":"2"},
				{"id":"2","label":"Condition","conditionRule":{},"nextTrue":"3","nextFalse":"4"},
				{"id":"3","label":"End True"},
				{"id":"4","label":"End False"}
			]`,
			dbWorkflow: `[
				{"id":"1","type":"start","label":"Start"},
				{"id":"2","type":"condition","label":"Condition"},
				{"id":"3","type":"end","label":"End True"},
				{"id":"4","type":"end","label":"End False"}
			]`,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := mergeTypeFromDB([]byte(tc.apiWorkflow), []byte(tc.dbWorkflow))

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Parse result to verify type field was added
			var resultNodes []map[string]interface{}
			err = json.Unmarshal(result, &resultNodes)
			if err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			// Parse database workflow to get expected types
			var dbNodes []map[string]interface{}
			err = json.Unmarshal([]byte(tc.dbWorkflow), &dbNodes)
			if err != nil {
				t.Fatalf("failed to unmarshal database workflow: %v", err)
			}

			// Build expected type map
			expectedTypes := make(map[string]string)
			for _, dbNode := range dbNodes {
				id := dbNode["id"].(string)
				nodeType := dbNode["type"].(string)
				expectedTypes[id] = nodeType
			}

			// Verify each result node has the correct type
			for _, resultNode := range resultNodes {
				id, ok := resultNode["id"].(string)
				if !ok {
					t.Errorf("node missing id field")
					continue
				}

				gotType, ok := resultNode["type"].(string)
				if !ok {
					t.Errorf("node %s missing type field", id)
					continue
				}

				expectedType, exists := expectedTypes[id]
				if !exists {
					t.Errorf("node %s not found in expected types", id)
					continue
				}

				if gotType != expectedType {
					t.Errorf("node %s type mismatch: got %v, want %v", id, gotType, expectedType)
				}
			}
		})
	}
}

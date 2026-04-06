package workflow

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowStructurallyEqual(t *testing.T) {
	startID := uuid.New().String()
	endID := uuid.New().String()
	sectionID := uuid.New().String()
	conditionID := uuid.New().String()
	questionID := uuid.New().String()

	testCases := []struct {
		name        string
		current     []byte
		incoming    []byte
		expected    bool
		expectedErr bool
	}{
		{
			name: "same structure different labels",
			current: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Start", "next": endID},
				{"id": endID, "type": "end", "label": "End"},
			}),
			incoming: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Different Label", "next": endID},
				{"id": endID, "type": "end", "label": "Another End"},
			}),
			expected: true,
		},
		{
			name: "same structure different key order",
			current: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Start", "next": endID},
				{"id": endID, "type": "end", "label": "End"},
			}),
			incoming: mustMarshal(t, []map[string]interface{}{
				{"label": "Start", "next": endID, "id": startID, "type": "start"},
				{"label": "End", "id": endID, "type": "end"},
			}),
			expected: true,
		},
		{
			name: "different next",
			current: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Start", "next": endID},
				{"id": endID, "type": "end", "label": "End"},
			}),
			incoming: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Start", "next": sectionID},
				{"id": sectionID, "type": "section", "label": "S", "next": endID},
				{"id": endID, "type": "end", "label": "End"},
			}),
			expected: false,
		},
		{
			name: "different node set extra node",
			current: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Start", "next": endID},
				{"id": endID, "type": "end", "label": "End"},
			}),
			incoming: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Start", "next": sectionID},
				{"id": sectionID, "type": "section", "label": "Section", "next": endID},
				{"id": endID, "type": "end", "label": "End"},
			}),
			expected: false,
		},
		{
			name: "condition same structure different labels",
			current: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Start", "next": conditionID},
				{"id": conditionID, "type": "condition", "label": "Old condition label",
					"nextTrue": endID, "nextFalse": endID,
					"conditionRule": map[string]interface{}{"source": "CHOICE", "question": questionID, "pattern": "^x$"}},
				{"id": endID, "type": "end", "label": "End"},
			}),
			incoming: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Start", "next": conditionID},
				{"id": conditionID, "type": "condition", "label": "New condition label",
					"nextTrue": endID, "nextFalse": endID,
					"conditionRule": map[string]interface{}{"source": "CHOICE", "question": questionID, "pattern": "^x$"}},
				{"id": endID, "type": "end", "label": "End"},
			}),
			expected: true,
		},
		{
			name: "condition different rule",
			current: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Start", "next": conditionID},
				{"id": conditionID, "type": "condition", "label": "Cond",
					"nextTrue": endID, "nextFalse": endID,
					"conditionRule": map[string]interface{}{"source": "CHOICE", "question": questionID, "pattern": "^a$"}},
				{"id": endID, "type": "end", "label": "End"},
			}),
			incoming: mustMarshal(t, []map[string]interface{}{
				{"id": startID, "type": "start", "label": "Start", "next": conditionID},
				{"id": conditionID, "type": "condition", "label": "Cond",
					"nextTrue": endID, "nextFalse": endID,
					"conditionRule": map[string]interface{}{"source": "CHOICE", "question": questionID, "pattern": "^b$"}},
				{"id": endID, "type": "end", "label": "End"},
			}),
			expected: false,
		},
		{
			name:        "invalid current JSON",
			current:     []byte(`not json`),
			incoming:    mustMarshal(t, []map[string]interface{}{{"id": "1", "type": "start"}}),
			expectedErr: true,
		},
		{
			name:        "invalid incoming JSON",
			current:     mustMarshal(t, []map[string]interface{}{{"id": "1", "type": "start"}}),
			incoming:    []byte(`[`),
			expectedErr: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := structurallyEqual(tc.current, tc.incoming)
			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

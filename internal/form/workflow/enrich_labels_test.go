package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/question"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errQuestionNotFound = errors.New("question not found")

func TestEnrichWorkflow_Labels(t *testing.T) {
	ctx := context.Background()
	startID := uuid.New().String()
	sectionID := uuid.New().String()
	conditionID := uuid.New().String()
	endID := uuid.New().String()
	startIDNilDeps := uuid.New().String()
	sectionIDMissing := uuid.New().String()
	endIDMissing := uuid.New().String()
	questionID := uuid.New()
	formID := uuid.New()

	testCases := []struct {
		name            string
		setup           func(t *testing.T) (workflow []byte, sectionTitles map[string]string, questionStore QuestionStore)
		expectUnchanged bool
		expectedLabels  map[string]string
	}{
		{
			name: "section and condition labels enriched",
			setup: func(t *testing.T) ([]byte, map[string]string, QuestionStore) {
				t.Helper()
				workflow := marshalWorkflow(t, []map[string]interface{}{
					{
						"id":    startID,
						"type":  nodeTypeToUppercase(NodeTypeStart),
						"label": "Start",
						"next":  sectionID,
					},
					{
						"id":    sectionID,
						"type":  nodeTypeToUppercase(NodeTypeSection),
						"label": "Old Section Label",
						"next":  conditionID,
					},
					{
						"id":        conditionID,
						"type":      nodeTypeToUppercase(NodeTypeCondition),
						"label":     "Old Condition Label",
						"nextTrue":  endID,
						"nextFalse": endID,
						"conditionRule": map[string]interface{}{
							"source":   "CHOICE",
							"question": questionID.String(),
							"pattern":  "^yes$",
						},
					},
					{
						"id":    endID,
						"type":  nodeTypeToUppercase(NodeTypeEnd),
						"label": "End",
					},
				})
				q := question.Question{ID: questionID, Title: pgtype.Text{String: "Question One", Valid: true}}
				answerable := question.NewShortText(q, formID)
				store := &mockQuestionStoreForEnrich{
					questions: map[string]question.Answerable{
						questionID.String(): answerable,
					},
				}
				sectionTitles := map[string]string{
					sectionID: "My Section Title",
				}
				return workflow, sectionTitles, store
			},
			expectUnchanged: false,
			expectedLabels: map[string]string{
				sectionID:   "My Section Title",
				conditionID: "When Question One matches ^yes$",
			},
		},
		{
			name: "nil deps leaves workflow unchanged",
			setup: func(t *testing.T) ([]byte, map[string]string, QuestionStore) {
				t.Helper()
				workflow := marshalWorkflow(t, []map[string]interface{}{{
					"id":    startIDNilDeps,
					"type":  nodeTypeToUppercase(NodeTypeStart),
					"label": "Start",
				}})
				return workflow, nil, nil
			},
			expectUnchanged: true,
			expectedLabels:  nil,
		},
		{
			name: "section missing from map keeps original label",
			setup: func(t *testing.T) ([]byte, map[string]string, QuestionStore) {
				t.Helper()
				workflow := marshalWorkflow(t, []map[string]interface{}{
					{
						"id":    sectionIDMissing,
						"type":  nodeTypeToUppercase(NodeTypeSection),
						"label": "Original",
						"next":  endIDMissing,
					},
				})
				return workflow, map[string]string{}, nil
			},
			expectUnchanged: false,
			expectedLabels:  map[string]string{sectionIDMissing: "Original"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workflow, sectionTitles, questionStore := tc.setup(t)

			enriched, err := enrichWorkflowLabels(ctx, workflow, formID, sectionTitles, questionStore)
			require.NoError(t, err)

			if tc.expectUnchanged {
				assert.Equal(t, workflow, enriched)
				return
			}

			var nodes []map[string]interface{}
			require.NoError(t, json.Unmarshal(enriched, &nodes))
			assertNodeLabels(t, nodesByID(nodes), tc.expectedLabels)
		})
	}
}

func TestConditionLabel_Rule(t *testing.T) {
	testCases := []struct {
		name     string
		node     map[string]interface{}
		store    QuestionStore
		expected string
	}{
		{
			name: "no question in rule keeps fallback",
			node: map[string]interface{}{
				"conditionRule": map[string]interface{}{
					"pattern": "^x$",
				},
			},
			store:    nil,
			expected: "No label",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			label := conditionLabelFromRule(context.Background(), tc.node, tc.store)
			assert.Equal(t, tc.expected, label)
		})
	}
}

// marshalWorkflow marshals workflow nodes to JSON and fails the test on error.
func marshalWorkflow(t *testing.T, nodes []map[string]interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(nodes)
	require.NoError(t, err)
	return b
}

// nodesByID returns a map of node ID to node for quick lookup.
func nodesByID(nodes []map[string]interface{}) map[string]map[string]interface{} {
	byID := make(map[string]map[string]interface{}, len(nodes))
	for _, n := range nodes {
		id, ok := n["id"].(string)
		if ok {
			byID[id] = n
		}
	}
	return byID
}

// assertNodeLabels asserts that each node in byID has the expected label.
func assertNodeLabels(t *testing.T, byID map[string]map[string]interface{}, expectedLabels map[string]string) {
	t.Helper()
	for nodeID, expectedLabel := range expectedLabels {
		n, ok := byID[nodeID]
		require.True(t, ok, "node %q not found", nodeID)
		assert.Equal(t, expectedLabel, n["label"], "node %q label", nodeID)
	}
}

// mockQuestionStoreForEnrich implements QuestionStore for enrichment tests.
type mockQuestionStoreForEnrich struct {
	questions map[string]question.Answerable
}

func (m *mockQuestionStoreForEnrich) GetByID(ctx context.Context, id uuid.UUID) (question.Answerable, error) {
	if a, ok := m.questions[id.String()]; ok {
		return a, nil
	}
	return nil, errQuestionNotFound
}

func (m *mockQuestionStoreForEnrich) ListSections(ctx context.Context, formID uuid.UUID) (map[string]question.Section, error) {
	return nil, nil
}

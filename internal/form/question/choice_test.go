package question

import (
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/shared"
	"github.com/google/uuid"
)

// Helper function to create test choices
func createTestChoices() []Choice {
	return []Choice{
		{
			ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			Name:        "Option A",
			Description: "First option",
		},
		{
			ID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Name:        "Option B",
			Description: "Second option",
		},
		{
			ID:          uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			Name:        "Option C",
			Description: "",
		},
	}
}

// SingleChoice Tests

func TestSingleChoice_DecodeRequest(t *testing.T) {
	choices := createTestChoices()
	metadata, _ := json.Marshal(map[string]any{"choice": choices})

	sc := SingleChoice{
		question: Question{ID: uuid.New(), Metadata: metadata},
		formID:   uuid.New(),
		Choices:  choices,
	}

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
		validate    func(t *testing.T, result any)
	}{
		{
			name:        "Should decode valid single choice",
			rawValue:    `["11111111-1111-1111-1111-111111111111"]`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.SingleChoiceAnswer)
				if !ok {
					t.Fatalf("Expected shared.SingleChoiceAnswer, got %T", result)
				}
				if answer.ChoiceID.String() != "11111111-1111-1111-1111-111111111111" {
					t.Errorf("Expected choice ID 11111111-1111-1111-1111-111111111111, got %s", answer.ChoiceID)
				}
				if answer.Snapshot.Name != "Option A" {
					t.Errorf("Expected snapshot name 'Option A', got %q", answer.Snapshot.Name)
				}
				if answer.Snapshot.Description != "First option" {
					t.Errorf("Expected snapshot description 'First option', got %q", answer.Snapshot.Description)
				}
			},
		},
		{
			name:        "Should return error for empty array",
			rawValue:    `[]`,
			shouldError: true,
		},
		{
			name:        "Should return error for multiple selections",
			rawValue:    `["11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"]`,
			shouldError: true,
		},
		{
			name:        "Should return error for invalid choice ID",
			rawValue:    `["99999999-9999-9999-9999-999999999999"]`,
			shouldError: true,
		},
		{
			name:        "Should return error for malformed UUID",
			rawValue:    `["not-a-uuid"]`,
			shouldError: true,
		},
		{
			name:        "Should return error for invalid JSON format",
			rawValue:    `"single-string"`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sc.DecodeRequest(json.RawMessage(tt.rawValue))

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

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestSingleChoice_DecodeStorage(t *testing.T) {
	choices := createTestChoices()
	sc := SingleChoice{
		question: Question{ID: uuid.New()},
		formID:   uuid.New(),
		Choices:  choices,
	}

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
		validate    func(t *testing.T, result any)
	}{
		{
			name:        "Should decode stored single choice answer",
			rawValue:    `{"choiceId":"11111111-1111-1111-1111-111111111111","snapshot":{"name":"Option A","description":"First option"}}`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.SingleChoiceAnswer)
				if !ok {
					t.Fatalf("Expected shared.SingleChoiceAnswer, got %T", result)
				}
				if answer.ChoiceID.String() != "11111111-1111-1111-1111-111111111111" {
					t.Errorf("Expected choice ID 11111111-1111-1111-1111-111111111111, got %s", answer.ChoiceID)
				}
				if answer.Snapshot.Name != "Option A" {
					t.Errorf("Expected snapshot name 'Option A', got %q", answer.Snapshot.Name)
				}
			},
		},
		{
			name:        "Should return error for invalid JSON",
			rawValue:    `invalid json`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sc.DecodeStorage(json.RawMessage(tt.rawValue))

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

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestSingleChoice_EncodeRequest(t *testing.T) {
	choices := createTestChoices()
	sc := SingleChoice{
		question: Question{ID: uuid.New()},
		formID:   uuid.New(),
		Choices:  choices,
	}

	tests := []struct {
		name        string
		answer      any
		expected    string
		shouldError bool
	}{
		{
			name: "Should encode single choice answer",
			answer: shared.SingleChoiceAnswer{
				ChoiceID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				Snapshot: shared.ChoiceSnapshot{
					Name:        "Option A",
					Description: "First option",
				},
			},
			expected:    `["11111111-1111-1111-1111-111111111111"]`,
			shouldError: false,
		},
		{
			name:        "Should return error for wrong answer type",
			answer:      "not a single choice answer",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sc.EncodeRequest(tt.answer)

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

			if string(result) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(result))
			}
		})
	}
}

// MultiChoice Tests

func TestMultiChoice_DecodeRequest(t *testing.T) {
	choices := createTestChoices()
	metadata, _ := json.Marshal(map[string]any{"choice": choices})

	mc := MultiChoice{
		question: Question{ID: uuid.New(), Metadata: metadata},
		formID:   uuid.New(),
		Choices:  choices,
	}

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
		validate    func(t *testing.T, result any)
	}{
		{
			name:        "Should decode multiple choices",
			rawValue:    `["11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"]`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.MultipleChoiceAnswer)
				if !ok {
					t.Fatalf("Expected shared.MultipleChoiceAnswer, got %T", result)
				}
				if len(answer.Choices) != 2 {
					t.Fatalf("Expected 2 choices, got %d", len(answer.Choices))
				}
				if answer.Choices[0].ChoiceID.String() != "11111111-1111-1111-1111-111111111111" {
					t.Errorf("Expected first choice ID 11111111-1111-1111-1111-111111111111, got %s", answer.Choices[0].ChoiceID)
				}
				if answer.Choices[0].Snapshot.Name != "Option A" {
					t.Errorf("Expected first snapshot name 'Option A', got %q", answer.Choices[0].Snapshot.Name)
				}
				if answer.Choices[1].ChoiceID.String() != "22222222-2222-2222-2222-222222222222" {
					t.Errorf("Expected second choice ID 22222222-2222-2222-2222-222222222222, got %s", answer.Choices[1].ChoiceID)
				}
			},
		},
		{
			name:        "Should decode single choice",
			rawValue:    `["11111111-1111-1111-1111-111111111111"]`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.MultipleChoiceAnswer)
				if !ok {
					t.Fatalf("Expected shared.MultipleChoiceAnswer, got %T", result)
				}
				if len(answer.Choices) != 1 {
					t.Fatalf("Expected 1 choice, got %d", len(answer.Choices))
				}
			},
		},
		{
			name:        "Should return error for empty array",
			rawValue:    `[]`,
			shouldError: true,
		},
		{
			name:        "Should return error for invalid choice ID",
			rawValue:    `["99999999-9999-9999-9999-999999999999"]`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := mc.DecodeRequest(json.RawMessage(tt.rawValue))

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

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestMultiChoice_EncodeRequest(t *testing.T) {
	choices := createTestChoices()
	mc := MultiChoice{
		question: Question{ID: uuid.New()},
		formID:   uuid.New(),
		Choices:  choices,
	}

	tests := []struct {
		name        string
		answer      any
		shouldError bool
		validate    func(t *testing.T, result json.RawMessage)
	}{
		{
			name: "Should encode multiple choice answer",
			answer: shared.MultipleChoiceAnswer{
				Choices: []struct {
					ChoiceID uuid.UUID             `json:"choiceId"`
					Snapshot shared.ChoiceSnapshot `json:"snapshot"`
				}{
					{
						ChoiceID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
						Snapshot: shared.ChoiceSnapshot{Name: "Option A", Description: "First option"},
					},
					{
						ChoiceID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
						Snapshot: shared.ChoiceSnapshot{Name: "Option B", Description: "Second option"},
					},
				},
			},
			shouldError: false,
			validate: func(t *testing.T, result json.RawMessage) {
				var ids []string
				if err := json.Unmarshal(result, &ids); err != nil {
					t.Errorf("Failed to unmarshal result: %v", err)
					return
				}
				if len(ids) != 2 {
					t.Errorf("Expected 2 choice IDs, got %d", len(ids))
				}
				if ids[0] != "11111111-1111-1111-1111-111111111111" {
					t.Errorf("Expected first ID 11111111-1111-1111-1111-111111111111, got %s", ids[0])
				}
				if ids[1] != "22222222-2222-2222-2222-222222222222" {
					t.Errorf("Expected second ID 22222222-2222-2222-2222-222222222222, got %s", ids[1])
				}
			},
		},
		{
			name:        "Should return error for wrong answer type",
			answer:      "not a multiple choice answer",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := mc.EncodeRequest(tt.answer)

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

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

// Ranking Tests

func TestRanking_DecodeRequest(t *testing.T) {
	choices := createTestChoices()
	metadata, _ := json.Marshal(map[string]any{"choice": choices})

	ranking := Ranking{
		question: Question{ID: uuid.New(), Metadata: metadata},
		formID:   uuid.New(),
		Rank:     choices,
	}

	tests := []struct {
		name        string
		rawValue    string
		shouldError bool
		validate    func(t *testing.T, result any)
	}{
		{
			name:        "Should decode ranking with correct order",
			rawValue:    `["33333333-3333-3333-3333-333333333333", "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"]`,
			shouldError: false,
			validate: func(t *testing.T, result any) {
				answer, ok := result.(shared.RankingAnswer)
				if !ok {
					t.Fatalf("Expected shared.RankingAnswer, got %T", result)
				}
				if len(answer.RankedChoices) != 3 {
					t.Fatalf("Expected 3 ranked choices, got %d", len(answer.RankedChoices))
				}
				// Check first ranked choice
				if answer.RankedChoices[0].ChoiceID.String() != "33333333-3333-3333-3333-333333333333" {
					t.Errorf("Expected first choice ID 33333333-3333-3333-3333-333333333333, got %s", answer.RankedChoices[0].ChoiceID)
				}
				if answer.RankedChoices[0].Rank != 1 {
					t.Errorf("Expected first rank to be 1, got %d", answer.RankedChoices[0].Rank)
				}
				if answer.RankedChoices[0].Snapshot.Name != "Option C" {
					t.Errorf("Expected first snapshot name 'Option C', got %q", answer.RankedChoices[0].Snapshot.Name)
				}
				// Check third ranked choice
				if answer.RankedChoices[2].Rank != 3 {
					t.Errorf("Expected third rank to be 3, got %d", answer.RankedChoices[2].Rank)
				}
			},
		},
		{
			name:        "Should return error for empty array",
			rawValue:    `[]`,
			shouldError: true,
		},
		{
			name:        "Should return error for invalid choice ID",
			rawValue:    `["99999999-9999-9999-9999-999999999999"]`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ranking.DecodeRequest(json.RawMessage(tt.rawValue))

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

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestRanking_EncodeRequest(t *testing.T) {
	choices := createTestChoices()
	ranking := Ranking{
		question: Question{ID: uuid.New()},
		formID:   uuid.New(),
		Rank:     choices,
	}

	tests := []struct {
		name        string
		answer      any
		shouldError bool
		validate    func(t *testing.T, result json.RawMessage)
	}{
		{
			name: "Should encode ranking answer in correct order",
			answer: shared.RankingAnswer{
				RankedChoices: []struct {
					ChoiceID uuid.UUID             `json:"choiceId"`
					Snapshot shared.ChoiceSnapshot `json:"snapshot"`
					Rank     int                   `json:"rank"`
				}{
					{
						ChoiceID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
						Snapshot: shared.ChoiceSnapshot{Name: "Option B", Description: "Second option"},
						Rank:     2,
					},
					{
						ChoiceID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
						Snapshot: shared.ChoiceSnapshot{Name: "Option A", Description: "First option"},
						Rank:     1,
					},
					{
						ChoiceID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
						Snapshot: shared.ChoiceSnapshot{Name: "Option C", Description: ""},
						Rank:     3,
					},
				},
			},
			shouldError: false,
			validate: func(t *testing.T, result json.RawMessage) {
				var ids []string
				if err := json.Unmarshal(result, &ids); err != nil {
					t.Errorf("Failed to unmarshal result: %v", err)
					return
				}
				if len(ids) != 3 {
					t.Errorf("Expected 3 choice IDs, got %d", len(ids))
				}
				// Should be sorted by rank
				if ids[0] != "11111111-1111-1111-1111-111111111111" {
					t.Errorf("Expected first ID (rank 1) 11111111-1111-1111-1111-111111111111, got %s", ids[0])
				}
				if ids[1] != "22222222-2222-2222-2222-222222222222" {
					t.Errorf("Expected second ID (rank 2) 22222222-2222-2222-2222-222222222222, got %s", ids[1])
				}
				if ids[2] != "33333333-3333-3333-3333-333333333333" {
					t.Errorf("Expected third ID (rank 3) 33333333-3333-3333-3333-333333333333, got %s", ids[2])
				}
			},
		},
		{
			name:        "Should return error for wrong answer type",
			answer:      "not a ranking answer",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ranking.EncodeRequest(tt.answer)

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

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

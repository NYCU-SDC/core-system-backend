package highlight

import (
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"github.com/google/uuid"
)

func TestCountChoicesSingleChoice(t *testing.T) {
	t.Parallel()

	choiceA := uuid.New()
	choiceB := uuid.New()
	answers := [][]byte{
		mustMarshal(t, shared.SingleChoiceAnswer{ChoiceID: choiceA}),
		mustMarshal(t, shared.SingleChoiceAnswer{ChoiceID: choiceA}),
		mustMarshal(t, shared.SingleChoiceAnswer{ChoiceID: choiceB}),
	}

	stats, err := countChoices(question.QuestionTypeSingleChoice, []question.Choice{
		{ID: choiceA, Name: "A"},
		{ID: choiceB, Name: "B"},
	}, answers)
	if err != nil {
		t.Fatalf("countChoices returned error: %v", err)
	}

	if got, want := stats[0].Count, int32(2); got != want {
		t.Fatalf("choice A count = %d, want %d", got, want)
	}
	if got, want := stats[1].Count, int32(1); got != want {
		t.Fatalf("choice B count = %d, want %d", got, want)
	}
}

func TestCountChoicesMultipleChoice(t *testing.T) {
	t.Parallel()

	choiceA := uuid.New()
	choiceB := uuid.New()
	choiceC := uuid.New()
	answers := [][]byte{
		mustMarshal(t, shared.MultipleChoiceAnswer{Choices: []struct {
			ChoiceID uuid.UUID             `json:"choiceId"`
			Snapshot shared.ChoiceSnapshot `json:"snapshot"`
		}{{ChoiceID: choiceA}, {ChoiceID: choiceB}}}),
		mustMarshal(t, shared.MultipleChoiceAnswer{Choices: []struct {
			ChoiceID uuid.UUID             `json:"choiceId"`
			Snapshot shared.ChoiceSnapshot `json:"snapshot"`
		}{{ChoiceID: choiceB}, {ChoiceID: choiceC}}}),
	}

	stats, err := countChoices(question.QuestionTypeMultipleChoice, []question.Choice{
		{ID: choiceA, Name: "A"},
		{ID: choiceB, Name: "B"},
		{ID: choiceC, Name: "C"},
	}, answers)
	if err != nil {
		t.Fatalf("countChoices returned error: %v", err)
	}

	if got, want := stats[0].Count, int32(1); got != want {
		t.Fatalf("choice A count = %d, want %d", got, want)
	}
	if got, want := stats[1].Count, int32(2); got != want {
		t.Fatalf("choice B count = %d, want %d", got, want)
	}
	if got, want := stats[2].Count, int32(1); got != want {
		t.Fatalf("choice C count = %d, want %d", got, want)
	}
}

func TestNormalizeDisplayTitle(t *testing.T) {
	t.Parallel()

	if got := normalizeDisplayTitle(nil); got.Valid {
		t.Fatalf("nil display title should be invalid")
	}

	blank := "   "
	if got := normalizeDisplayTitle(&blank); got.Valid {
		t.Fatalf("blank display title should be invalid")
	}

	title := "  Grade Summary  "
	got := normalizeDisplayTitle(&title)
	if !got.Valid || got.String != "Grade Summary" {
		t.Fatalf("normalized title = %#v, want valid trimmed text", got)
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return data
}

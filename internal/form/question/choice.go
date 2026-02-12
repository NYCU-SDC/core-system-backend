package question

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type ChoiceOption struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type Choice struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

type SingleChoice struct {
	question Question
	formID   uuid.UUID
	Choices  []Choice
}

func (s SingleChoice) Question() Question {
	return s.question
}

func (s SingleChoice) FormID() uuid.UUID {
	return s.formID
}

func (s SingleChoice) Validate(value string) error {
	if value == "" {
		return nil // No value means no selection
	}

	for _, choice := range s.Choices {
		if choice.ID.String() == value {
			return nil
		}
	}

	return ErrInvalidChoiceID{
		QuestionID: s.question.ID.String(),
		ChoiceID:   value,
	}
}

func NewSingleChoice(q Question, formID uuid.UUID) (SingleChoice, error) {
	metadata := q.Metadata

	// If sourceID is provided, metadata should be empty (no choices)
	if q.SourceID.Valid {
		return SingleChoice{question: q, formID: formID, Choices: []Choice{}}, nil
	}

	if metadata == nil || len(metadata) == 0 {
		return SingleChoice{}, fmt.Errorf("metadata is nil or empty for single choice question %s", q.ID.String())
	}

	choices, err := ExtractChoices(metadata)
	if err != nil {
		return SingleChoice{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    metadata,
			Message:    "could not extract choices from metadata",
		}
	}

	if len(choices) == 0 {
		return SingleChoice{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    metadata,
			Message:    "no choices found in metadata",
		}
	}

	for _, choice := range choices {
		if choice.ID == uuid.Nil {
			return SingleChoice{}, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice ID cannot be nil",
			}
		}

		if strings.TrimSpace(choice.Name) == "" {
			return SingleChoice{}, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice name cannot be empty",
			}
		}
	}

	return SingleChoice{
		question: q,
		formID:   formID,
		Choices:  choices,
	}, nil
}

type MultiChoice struct {
	question Question
	formID   uuid.UUID
	Choices  []Choice
}

func (m MultiChoice) Question() Question {
	return m.question
}

func (m MultiChoice) FormID() uuid.UUID {
	return m.formID
}

func (m MultiChoice) Validate(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil // No value means no selection
	}

	ids := strings.Split(value, ";")
	for _, v := range ids {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}

		valid := false
		for _, choice := range m.Choices {
			if choice.ID.String() == v {
				valid = true
				break
			}
		}

		if !valid {
			return ErrInvalidChoiceID{
				QuestionID: m.question.ID.String(),
				ChoiceID:   v,
			}
		}
	}

	return nil
}

func NewMultiChoice(q Question, formID uuid.UUID) (MultiChoice, error) {
	metadata := q.Metadata

	// If sourceID is provided, metadata should be empty (no choices)
	if q.SourceID.Valid {
		return MultiChoice{question: q, formID: formID, Choices: []Choice{}}, nil
	}

	if metadata == nil || len(metadata) == 0 {
		return MultiChoice{}, fmt.Errorf("metadata is nil or empty for multiple choice question %s", q.ID.String())
	}

	choices, err := ExtractChoices(metadata)
	if err != nil {
		return MultiChoice{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    metadata,
			Message:    "could not extract choices from metadata",
		}
	}

	if len(choices) == 0 {
		return MultiChoice{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    metadata,
			Message:    "no choices found in metadata",
		}
	}

	for _, choice := range choices {
		if choice.ID == uuid.Nil {
			return MultiChoice{}, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice ID cannot be nil",
			}
		}

		if strings.TrimSpace(choice.Name) == "" {
			return MultiChoice{}, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice name cannot be empty",
			}
		}
	}

	return MultiChoice{
		question: q,
		formID:   formID,
		Choices:  choices,
	}, nil
}

type DetailedMultiChoice struct {
	question Question
	formID   uuid.UUID
	Choices  []Choice
}

func (m DetailedMultiChoice) Question() Question {
	return m.question
}

func (m DetailedMultiChoice) FormID() uuid.UUID {
	return m.formID
}

func (m DetailedMultiChoice) Validate(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil // No value means no selection
	}

	ids := strings.Split(value, ";")
	for _, v := range ids {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}

		valid := false
		for _, choice := range m.Choices {
			if choice.ID.String() == v {
				valid = true
				break
			}
		}

		if !valid {
			return ErrInvalidChoiceID{
				QuestionID: m.question.ID.String(),
				ChoiceID:   v,
			}
		}
	}

	return nil
}

func NewDetailedMultiChoice(q Question, formID uuid.UUID) (DetailedMultiChoice, error) {
	metadata := q.Metadata
	if metadata == nil || len(metadata) == 0 {
		return DetailedMultiChoice{}, fmt.Errorf("metadata is nil or empty for detailed multiple choice question %s", q.ID.String())
	}

	choices, err := ExtractChoices(metadata)
	if err != nil {
		return DetailedMultiChoice{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    metadata,
			Message:    "could not extract choices from metadata",
		}
	}

	if len(choices) == 0 {
		return DetailedMultiChoice{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    metadata,
			Message:    "no choices found in metadata",
		}
	}

	for _, choice := range choices {
		if choice.ID == uuid.Nil {
			return DetailedMultiChoice{}, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice ID cannot be nil",
			}
		}

		if strings.TrimSpace(choice.Name) == "" {
			return DetailedMultiChoice{}, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice name cannot be empty",
			}
		}
	}

	return DetailedMultiChoice{
		question: q,
		formID:   formID,
		Choices:  choices,
	}, nil
}

type Ranking struct {
	question Question
	formID   uuid.UUID
	Rank     []Choice
}

func (r Ranking) Question() Question {
	return r.question
}

func (r Ranking) FormID() uuid.UUID {
	return r.formID
}

func (r Ranking) Validate(value string) error {
	ids := strings.Split(value, ";")
	for _, v := range ids {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}

		valid := false
		for _, choice := range r.Rank {
			if choice.ID.String() == v {
				valid = true
				break
			}
		}

		if !valid {
			return ErrInvalidChoiceID{
				QuestionID: r.question.ID.String(),
				ChoiceID:   v,
			}
		}
	}

	return nil
}

func NewRanking(q Question, formID uuid.UUID) (Ranking, error) {
	metadata := q.Metadata

	// If sourceID is provided, metadata should be empty (no choices)
	if q.SourceID.Valid {
		return Ranking{question: q, formID: formID, Rank: []Choice{}}, nil
	}

	if metadata == nil || len(metadata) == 0 {
		return Ranking{}, fmt.Errorf("metadata is nil or empty for ranking question %s", q.ID.String())
	}

	choices, err := ExtractChoices(metadata)
	if err != nil {
		return Ranking{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    metadata,
			Message:    "could not extract rank from metadata",
		}
	}

	if len(choices) == 0 {
		return Ranking{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    metadata,
			Message:    "no choices found in metadata",
		}
	}

	rank := make([]Choice, len(choices))
	for _, choice := range choices {
		if choice.ID == uuid.Nil {
			return Ranking{}, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice ID cannot be nil",
			}
		}
		rank = append(rank, choice)
	}

	return Ranking{
		question: q,
		formID:   formID,
		Rank:     rank,
	}, nil
}

// Creates and validates metadata JSON for choice-based questions
func GenerateChoiceMetadata(questionType string, choiceOptions []ChoiceOption) ([]byte, error) {
	// For choice questions, require at least one choice
	if len(choiceOptions) == 0 {
		return nil, ErrMetadataValidate{
			QuestionID: questionType,
			RawData:    []byte(fmt.Sprintf("%v", choiceOptions)),
			Message:    "no choices provided for choice question",
		}
	}

	// Generate choices with UUIDs
	choices := make([]Choice, len(choiceOptions))
	for i, option := range choiceOptions {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			return nil, ErrMetadataValidate{
				QuestionID: questionType,
				RawData:    []byte(fmt.Sprintf("%v", choiceOptions)),
				Message:    "choice name cannot be empty",
			}
		}
		choices[i] = Choice{
			ID:          uuid.New(),
			Name:        name,
			Description: strings.TrimSpace(option.Description),
		}
	}

	if questionType == "detailed_multiple_choice" {
		hasDescription := false
		for _, choice := range choices {
			if strings.TrimSpace(choice.Description) != "" {
				hasDescription = true
				break
			}
		}
		if !hasDescription {
			return nil, ErrMetadataValidate{
				QuestionID: questionType,
				RawData:    []byte(fmt.Sprintf("%v", choiceOptions)),
				Message:    "detailed multiple choice requires at least one choice with description",
			}
		}
	}

	metadata := map[string]any{
		"choice": choices,
	}

	return json.Marshal(metadata)
}

func ExtractChoices(data []byte) ([]Choice, error) {
	var partial map[string]json.RawMessage
	if err := json.Unmarshal(data, &partial); err != nil {
		return nil, fmt.Errorf("could not parse partial json: %w", err)
	}

	var choices []Choice
	if raw, ok := partial["choice"]; ok {
		if err := json.Unmarshal(raw, &choices); err != nil {
			return nil, fmt.Errorf("could not parse choices: %w", err)
		}
	}

	return choices, nil
}

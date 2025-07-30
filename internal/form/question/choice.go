package question

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
)

type Choice struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type SingleChoice struct {
	question Question
	Choices  []Choice
}

func (s SingleChoice) Question() Question {
	return s.question
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

func NewSingleChoice(q Question) (SingleChoice, error) {
	metadata := q.Metadata
	if metadata == nil {
		return SingleChoice{}, errors.New("metadata is nil")
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

		if choice.Name == "" {
			return SingleChoice{}, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice name cannot be empty",
			}
		}
	}

	return SingleChoice{
		question: q,
		Choices:  choices,
	}, nil
}

type MultiChoice struct {
	question Question
	Choices  []Choice
}

func (m MultiChoice) Question() Question {
	return m.question
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

func NewMultiChoice(q Question) (MultiChoice, error) {
	metadata := q.Metadata
	if metadata == nil {
		return MultiChoice{}, errors.New("metadata is nil")
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

		if choice.Name == "" {
			return MultiChoice{}, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice name cannot be empty",
			}
		}
	}

	return MultiChoice{
		question: q,
		Choices:  choices,
	}, nil
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

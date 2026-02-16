package question

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Date struct {
	question Question
	formID   uuid.UUID
}

func NewDate(q Question, formID uuid.UUID) (Answerable, error) {
	return &Date{question: q, formID: formID}, nil
}

func (d Date) Question() Question {
	return d.question
}

func (d Date) FormID() uuid.UUID {
	return d.formID
}

func (d Date) Validate(value string) error {
	_, err := time.Parse("2006-01-02", value)
	if err != nil {
		return ErrInvalidDateFormat{
			QuestionID: d.question.ID.String(),
			RawValue:   value,
			Message:    "invalid date format, expected YYYY-MM-DD",
		}
	}
	return nil
}

func (d Date) DecodeRequest(rawValue json.RawMessage) (any, error) {
	// TODO: Implement date decoding from API request
	return nil, errors.New("not implemented yet")
}

func (d Date) DecodeStorage(rawValue json.RawMessage) (any, error) {
	// TODO: Implement date decoding from storage
	return nil, errors.New("not implemented yet")
}

func (d Date) EncodeRequest(answer any) (json.RawMessage, error) {
	// TODO: Implement date encoding to API request format
	return nil, errors.New("not implemented yet")
}

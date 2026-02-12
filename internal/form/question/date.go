package question

import (
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

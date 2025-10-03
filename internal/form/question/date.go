package question

import (
	"time"
)

type Date struct {
	question Question
}

func NewDate(q Question) (Answerable, error) {
	return &Date{question: q}, nil
}

func (d Date) Question() Question {
	return d.question
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

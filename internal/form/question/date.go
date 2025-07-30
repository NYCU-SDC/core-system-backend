package question

import (
	"time"
)

type Date struct {
	questionID string
}

func (d Date) QuestionID() string {
	return d.questionID
}

func (d Date) Validate(value string) error {
	_, err := time.Parse("2006-01-02", value)
	if err != nil {
		return ErrInvalidDateFormat{
			QuestionID: d.questionID,
			RawValue:   value,
			Message:    "invalid date format, expected YYYY-MM-DD",
		}
	}
	return nil
}

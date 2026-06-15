package highlight

import (
	"NYCU-SDC/core-system-backend/internal"
	"fmt"
)

type ErrQuestionNotInForm struct {
	FormID     string
	QuestionID string
}

func (e ErrQuestionNotInForm) Error() string {
	return fmt.Sprintf("question %s does not belong to form %s", e.QuestionID, e.FormID)
}

func (e ErrQuestionNotInForm) Unwrap() error {
	return internal.ErrHighlightQuestionNotInForm
}

type ErrUnsupportedHighlightQuestionType struct {
	QuestionID   string
	QuestionType string
}

func (e ErrUnsupportedHighlightQuestionType) Error() string {
	return fmt.Sprintf("question %s with type %s cannot be used as highlight", e.QuestionID, e.QuestionType)
}

func (e ErrUnsupportedHighlightQuestionType) Unwrap() error {
	return internal.ErrHighlightQuestionType
}

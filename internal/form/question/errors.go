package question

import "fmt"

type ErrInvalidAnswerLength struct {
	Expected int
	Given    int
}

func (e ErrInvalidAnswerLength) Error() string {
	return fmt.Sprintf("invalid answer length, expected %d, got %d", e.Expected, e.Given)
}

type ErrInvalidChoiceID struct {
	QuestionID string
	ChoiceID   string
}

func (e ErrInvalidChoiceID) Error() string {
	return fmt.Sprintf("choice ID %s not found for question %s", e.ChoiceID, e.QuestionID)
}

type ErrInvalidDateFormat struct {
	QuestionID string
	RawValue   string
	Message    string
}

func (e ErrInvalidDateFormat) Error() string {
	return fmt.Sprintf("invalid date format for question %s: %s, raw value: %s", e.QuestionID, e.Message, e.RawValue)
}

// ErrMetadataBroken is returned when stored metadata is corrupted and cannot be recovered.
type ErrMetadataBroken struct {
	QuestionID string
	RawData    []byte
	Message    string
}

func (e ErrMetadataBroken) Error() string {
	return fmt.Sprintf("metadata broken for question %s: %s, raw data: %s", e.QuestionID, e.Message, e.RawData)
}

type ErrMetadataValidate struct {
	QuestionID string
	RawData    []byte
	Message    string
}

func (e ErrMetadataValidate) Error() string {
	return fmt.Sprintf("metadata validation failed for question %s: %s, raw data: %s", e.QuestionID, e.Message, e.RawData)
}

type ErrUnsupportedQuestionType struct {
	QuestionType string
}

func (e ErrUnsupportedQuestionType) Error() string {
	return fmt.Sprintf("unsupported question type: %s", e.QuestionType)
}

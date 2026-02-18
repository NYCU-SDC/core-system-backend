package question

import (
	"NYCU-SDC/core-system-backend/internal"
	"fmt"
)

type ErrInvalidHyperlinkFormat struct {
	Value   string
	Message string
}

func (e ErrInvalidHyperlinkFormat) Error() string {
	return fmt.Sprintf("invalid hyperlink format: %s, value: %s", e.Message, e.Value)
}

func (e ErrInvalidHyperlinkFormat) Unwrap() error {
	return internal.ErrValidationFailed
}

type ErrInvalidScaleValue struct {
	QuestionID string
	RawValue   int
	Message    string
}

func (e ErrInvalidScaleValue) Error() string {
	return fmt.Sprintf("invalid value for question %s: %s, raw value: %d", e.QuestionID, e.Message, e.RawValue)
}

func (e ErrInvalidScaleValue) Unwrap() error {
	return internal.ErrValidationFailed
}

type ErrInvalidAnswerLength struct {
	Expected int
	Given    int
}

func (e ErrInvalidAnswerLength) Error() string {
	return fmt.Sprintf("invalid answer length, expected %d, got %d", e.Expected, e.Given)
}

func (e ErrInvalidAnswerLength) Unwrap() error {
	return internal.ErrValidationFailed
}

type ErrInvalidChoiceID struct {
	QuestionID string
	ChoiceID   string
}

func (e ErrInvalidChoiceID) Error() string {
	return fmt.Sprintf("choice ID %s not found for question %s", e.ChoiceID, e.QuestionID)
}

func (e ErrInvalidChoiceID) Unwrap() error {
	return internal.ErrValidationFailed
}

type ErrInvalidDateFormat struct {
	QuestionID string
	RawValue   string
	Message    string
}

func (e ErrInvalidDateFormat) Error() string {
	return fmt.Sprintf("invalid date format for question %s: %s, raw value: %s", e.QuestionID, e.Message, e.RawValue)
}

func (e ErrInvalidDateFormat) Unwrap() error {
	return internal.ErrValidationFailed
}

type ErrInvalidDateValue struct {
	QuestionID string
	Message    string
}

func (e ErrInvalidDateValue) Error() string {
	return fmt.Sprintf("invalid date value for question %s: %s", e.QuestionID, e.Message)
}

func (e ErrInvalidDateValue) Unwrap() error {
	return internal.ErrValidationFailed
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

func (e ErrMetadataBroken) Unwrap() error {
	return internal.ErrInternalServerError
}

type ErrMetadataValidate struct {
	QuestionID string
	RawData    []byte
	Message    string
}

func (e ErrMetadataValidate) Error() string {
	return fmt.Sprintf("metadata validation failed for question %s: %s, raw data: %s", e.QuestionID, e.Message, e.RawData)
}

func (e ErrMetadataValidate) Unwrap() error {
	return internal.ErrValidationFailed
}

type ErrUnsupportedQuestionType struct {
	QuestionType string
}

func (e ErrUnsupportedQuestionType) Error() string {
	return fmt.Sprintf("unsupported question type: %s", e.QuestionType)
}

func (e ErrUnsupportedQuestionType) Unwrap() error {
	return internal.ErrInvalidRequestBody
}

type ErrInvalidMetadata struct {
	QuestionID string
	RawData    []byte
	Message    string
}

func (e ErrInvalidMetadata) Error() string {
	return fmt.Sprintf("invalid metadata for question %s: %s, raw data: %s", e.QuestionID, e.Message, e.RawData)
}

func (e ErrInvalidMetadata) Unwrap() error {
	return internal.ErrInvalidRequestBody
}

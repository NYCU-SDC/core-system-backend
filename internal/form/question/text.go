package question

import (
	"net/url"
	"strings"

	"github.com/google/uuid"
)

type ShortText struct {
	question Question
	formID   uuid.UUID
}

func (s ShortText) Question() Question {
	return s.question
}

func (s ShortText) FormID() uuid.UUID {
	return s.formID
}

func (s ShortText) Validate(value string) error {
	if len(value) > 100 {
		return ErrInvalidAnswerLength{
			Expected: 100,
			Given:    len(value),
		}
	}

	return nil
}

func NewShortText(q Question, formID uuid.UUID) ShortText {
	return ShortText{
		question: q,
		formID:   formID,
	}
}

type LongText struct {
	question Question
	formID   uuid.UUID
}

func (l LongText) Question() Question {
	return l.question
}

func (l LongText) FormID() uuid.UUID {
	return l.formID
}

func (l LongText) Validate(value string) error {
	if len(value) > 1000 {
		return ErrInvalidAnswerLength{
			Expected: 1000,
			Given:    len(value),
		}
	}

	return nil
}

func NewLongText(q Question, formID uuid.UUID) LongText {
	return LongText{
		question: q,
		formID:   formID,
	}
}

type Hyperlink struct {
	question Question
	formID   uuid.UUID
}

func (h Hyperlink) Question() Question {
	return h.question
}

func (h Hyperlink) FormID() uuid.UUID {
	return h.formID
}

func (h Hyperlink) Validate(value string) error {
	if len(value) > 100 {
		return ErrInvalidAnswerLength{
			Expected: 100,
			Given:    len(value),
		}
	}

	if err := validateURL(value); err != nil {
		return err
	}

	return nil
}

func NewHyperlink(q Question, formID uuid.UUID) Hyperlink {
	return Hyperlink{
		question: q,
		formID:   formID,
	}
}

// validateURL checks if the value is a valid URL
func validateURL(value string) error {
	if value == "" {
		return nil
	}

	parsedURL, err := url.Parse(value)
	if err != nil {
		return ErrInvalidHyperlinkFormat{
			Value:   value,
			Message: "invalid URL format",
		}
	}

	if parsedURL.Scheme == "" {
		return ErrInvalidHyperlinkFormat{
			Value:   value,
			Message: "URL must include a scheme (http:// or https://)",
		}
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrInvalidHyperlinkFormat{
			Value:   value,
			Message: "URL scheme must be http or https",
		}
	}

	if parsedURL.Host == "" {
		return ErrInvalidHyperlinkFormat{
			Value:   value,
			Message: "URL must include a host",
		}
	}

	return nil
}

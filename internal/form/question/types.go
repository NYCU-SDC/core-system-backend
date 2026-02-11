package question

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func NewAnswerable(q Question, formID uuid.UUID) (Answerable, error) {
	switch q.Type {
	case QuestionTypeShortText:
		return NewShortText(q, formID), nil
	case QuestionTypeLongText:
		return NewLongText(q, formID), nil
	case QuestionTypeSingleChoice:
		return NewSingleChoice(q, formID)
	case QuestionTypeMultipleChoice:
		return NewMultiChoice(q, formID)
	case QuestionTypeDate:
		return NewDate(q, formID)
	case QuestionTypeDropdown:
		return NewSingleChoice(q, formID)
	case QuestionTypeDetailedMultipleChoice:
		return NewDetailedMultiChoice(q, formID)
	case QuestionTypeLinearScale:
		return NewLinearScale(q, formID)
	case QuestionTypeRating:
		return NewRating(q, formID)
	case QuestionTypeRanking:
		return NewRanking(q, formID)
	case QuestionTypeOauthConnect:
		return NewOAuthConnect(q, formID)
	case QuestionTypeUploadFile:
		return NewUploadFile(q, formID)
	case QuestionTypeHyperlink:
		return NewHyperlink(q, formID), nil
	}

	return nil, ErrUnsupportedQuestionType{
		QuestionType: string(q.Type),
	}
}

// ChoiceTypes lists question types with selectable options
// Used for workflow condition nodes with source "choice".
var ChoiceTypes = []QuestionType{
	QuestionTypeSingleChoice,
	QuestionTypeMultipleChoice,
	QuestionTypeDropdown,
	QuestionTypeDetailedMultipleChoice,
	QuestionTypeRanking,
}

// NonChoiceTypes lists question types with text/date/URL values for pattern matching
// Used for workflow condition nodes with source "nonChoice".
var NonChoiceTypes = []QuestionType{
	QuestionTypeShortText,
	QuestionTypeLongText,
	QuestionTypeDate,
	QuestionTypeHyperlink,
	QuestionTypeUploadFile,
	QuestionTypeOauthConnect,
	QuestionTypeLinearScale,
	QuestionTypeRating,
}

// ContainsType returns true if typeToCheck is in the types slice.
func ContainsType(types []QuestionType, typeToCheck QuestionType) bool {
	for _, t := range types {
		if t == typeToCheck {
			return true
		}
	}
	return false
}

// FormatAllowedTypes formats a slice of question types for use in error messages.
func FormatAllowedTypes(types []QuestionType) string {
	switch len(types) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("'%s'", string(types[0]))
	default:
		names := make([]string, len(types))
		for i, t := range types {
			names[i] = fmt.Sprintf("'%s'", string(t))
		}
		return fmt.Sprintf("%s, or %s", strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
	}
}

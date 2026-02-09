package question

import "github.com/google/uuid"

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

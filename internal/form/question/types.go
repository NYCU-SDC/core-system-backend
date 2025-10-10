package question

func NewAnswerable(q Question) (Answerable, error) {
	switch q.Type {
	case QuestionTypeShortText:
		return NewShortText(q), nil
	case QuestionTypeLongText:
		return NewLongText(q), nil
	case QuestionTypeSingleChoice:
		return NewSingleChoice(q)
	case QuestionTypeMultipleChoice:
		return NewMultiChoice(q)
	case QuestionTypeDate:
		return NewDate(q)
	}

	return nil, ErrUnsupportedQuestionType{
		QuestionType: string(q.Type),
	}
}

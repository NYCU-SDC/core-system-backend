package question

type ShortText struct {
	question Question
}

func (s ShortText) Question() Question {
	return s.question
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

func NewShortText(q Question) ShortText {
	return ShortText{
		question: q,
	}
}

type LongText struct {
	question Question
}

func (l LongText) Question() Question {
	return l.question
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

func NewLongText(q Question) LongText {
	return LongText{
		question: q,
	}
}

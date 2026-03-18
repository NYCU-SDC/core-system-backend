package shared

import "encoding/json"

type AnswerParam struct {
	QuestionID string
	Value      json.RawMessage
}

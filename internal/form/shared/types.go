package shared

import "encoding/json"

type AnswerParam struct {
	QuestionID string
	Value      json.RawMessage
}

type AnswerJSON struct {
	QuestionID   string   `json:"questionId" validate:"required,uuid"`
	QuestionType string   `json:"questionType" validate:"required,oneof=SHORT_TEXT LONG_TEXT SINGLE_CHOICE MULTIPLE_CHOICE DATE DROPDOWN DETAILED_MULTIPLE_CHOICE RANKING HYPERLINK"`
	Value        []string `json:"value" validate:"required"`
}

type ScaleAnswerJSON struct {
	QuestionID   string `json:"questionId" validate:"required,uuid"`
	QuestionType string `json:"questionType" validate:"required,oneof=LINEAR_SCALE RATING"`
	Value        int32  `json:"value" validate:"required"`
}

// OauthAnswerJSON is stored in DB and returned after OAuth completion
type OauthAnswerJSON struct {
	QuestionID string `json:"questionId" validate:"required,uuid"`
	AvatarURL  string `json:"avatarUrl" validate:"required"`
	Username   string `json:"username" validate:"required"`
}

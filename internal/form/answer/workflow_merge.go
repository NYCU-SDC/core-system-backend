package answer

import (
	"encoding/json"
	"fmt"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/shared"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	"github.com/google/uuid"
)

// MergeAnswersForWorkflowResolution returns a copy of current answers with each payload
// applied by question ID (payload wins). Request values are normalized with the same
// DecodeRequest + JSON marshal path as Upsert so workflow resolution (DecodeStorage /
// MatchesPattern) sees storage-shaped bytes, not raw API wire JSON.
func MergeAnswersForWorkflowResolution(
	currentAnswers []Answer,
	payloads []Payload,
	answerableMap map[string]question.Answerable,
) ([]Answer, error) {
	if len(payloads) == 0 {
		return currentAnswers, nil
	}

	answerMap := make(map[uuid.UUID]Answer, len(currentAnswers)+len(payloads))
	for _, ans := range currentAnswers {
		answerMap[ans.QuestionID] = ans
	}

	for _, payload := range payloads {
		qid, err := handlerutil.ParseUUID(payload.QuestionID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid questionId %q: %w", internal.ErrWorkflowMergeInvalidQuestionID, payload.QuestionID, err)
		}

		answerable, ok := answerableMap[payload.QuestionID]
		if !ok {
			return nil, fmt.Errorf("%w: question %s not found in form", internal.ErrWorkflowMergeQuestionNotInForm, payload.QuestionID)
		}

		decoded, err := answerable.DecodeRequest(shared.AnswerParam{
			QuestionID: payload.QuestionID,
			Value:      payload.Value,
			OtherText:  payload.OtherText,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: answer value for question %s: %w", internal.ErrWorkflowMergeAnswerValueInvalid, payload.QuestionID, err)
		}

		storageBytes, err := json.Marshal(decoded)
		if err != nil {
			return nil, fmt.Errorf("%w: encode answer for question %s: %w", internal.ErrWorkflowMergeAnswerEncodeFailed, payload.QuestionID, err)
		}

		prev := answerMap[qid]
		prev.QuestionID = qid
		prev.Value = storageBytes
		answerMap[qid] = prev
	}

	out := make([]Answer, 0, len(answerMap))
	for _, ans := range answerMap {
		out = append(out, ans)
	}
	return out, nil
}

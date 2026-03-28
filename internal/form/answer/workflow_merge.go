package answer

import (
	"slices"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	"github.com/google/uuid"
)

// MergeAnswersForWorkflowResolution returns a copy of current answers with each payload
// applied by question ID (payload wins). Used so ResolveSections matches the effective
// state after this PATCH, including multiple answers submitted in one request.
func MergeAnswersForWorkflowResolution(currentAnswers []Answer, payloads []Payload) []Answer {
	if len(payloads) == 0 {
		return currentAnswers
	}

	answerMap := make(map[uuid.UUID]Answer, len(currentAnswers)+len(payloads))
	for _, ans := range currentAnswers {
		answerMap[ans.QuestionID] = ans
	}

	for _, payload := range payloads {
		qid, err := handlerutil.ParseUUID(payload.QuestionID)
		if err != nil {
			continue
		}

		prev := answerMap[qid]
		prev.QuestionID = qid
		prev.Value = slices.Clone(payload.Value)
		answerMap[qid] = prev
	}

	out := make([]Answer, 0, len(answerMap))
	for _, ans := range answerMap {
		out = append(out, ans)
	}
	return out
}

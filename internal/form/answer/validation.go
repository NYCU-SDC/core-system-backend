package answer

import (
	"context"
	"errors"
	"fmt"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/question"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// WorkflowResolver resolves which sections are active for a form response based on workflow and current answers.
type WorkflowResolver interface {
	ResolveSections(ctx context.Context, formID uuid.UUID, answers []Answer, answerableMap map[string]question.Answerable) ([]uuid.UUID, error)
}

// ValidatePatchAnswersAgainstWorkflow returns nil if the PATCH may proceed.
// ErrWorkflowNotFound is treated as no constraint. Other errors should be passed to WriteError.
func ValidatePatchAnswersAgainstWorkflow(
	ctx context.Context,
	resolver WorkflowResolver,
	formID uuid.UUID,
	responseID uuid.UUID,
	answersForWorkflow []Answer,
	answerableMap map[string]question.Answerable,
	payloads []Payload,
	logger *zap.Logger,
	span trace.Span,
) error {
	sectionIDs, err := resolver.ResolveSections(ctx, formID, answersForWorkflow, answerableMap)
	if err != nil {
		if errors.Is(err, internal.ErrWorkflowNotFound) {
			return nil
		}
		err = fmt.Errorf("%w: %w", internal.ErrWorkflowResolveSectionsFailed, err)
		logger.Error("workflow section resolution failed",
			zap.String("formID", formID.String()),
			zap.String("responseID", responseID.String()),
			zap.Error(err),
		)
		span.RecordError(err)
		return err
	}

	sectionActiveMap := make(map[string]bool, len(sectionIDs))
	for _, sid := range sectionIDs {
		sectionActiveMap[sid.String()] = true
	}
	for _, p := range payloads {
		answerable, ok := answerableMap[p.QuestionID]
		if !ok {
			continue // question not in form; will be rejected later by Upsert
		}
		sectionIDStr := answerable.Question().SectionID.String()
		if !sectionActiveMap[sectionIDStr] {
			logger.Warn("rejected answer patch for workflow-skipped section",
				zap.String("formID", formID.String()),
				zap.String("responseID", responseID.String()),
				zap.String("questionID", p.QuestionID),
				zap.String("sectionID", sectionIDStr),
			)
			return internal.ErrAnswerSectionSkipped
		}
	}
	return nil
}

package answer

import (
	"context"
	"errors"
	"fmt"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/question"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ValidatePatchAnswersAgainstWorkflow returns nil if the PATCH may proceed.
// ErrWorkflowNotFound is treated as no constraint. Other errors should be passed to WriteError.
func (s Service) ValidatePatchAnswersAgainstWorkflow(
	ctx context.Context,
	formID, responseID uuid.UUID,
	answersForWorkflow []Answer,
	answerableMap map[string]question.Answerable,
	payloads []Payload,
) error {
	traceCtx, span := s.tracer.Start(ctx, "ValidatePatchAnswersAgainstWorkflow")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	sectionIDs, err := s.workflowResolver.ResolveSections(traceCtx, formID, answersForWorkflow, answerableMap)
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

			span.RecordError(internal.ErrAnswerSectionSkipped)
			return internal.ErrAnswerSectionSkipped
		}
	}

	return nil
}

// ValidateUploadFilesAgainstWorkflow returns nil if the file upload may proceed.
// ErrWorkflowNotFound is treated as no section constraint. Other ResolveSections errors are wrapped.
// If the question is missing from answerableMap, returns nil so UploadFiles can validate membership.
func (s Service) ValidateUploadFilesAgainstWorkflow(
	ctx context.Context,
	formID uuid.UUID,
	responseID uuid.UUID,
	questionID uuid.UUID,
	answersForWorkflow []Answer,
	answerableMap map[string]question.Answerable,
) error {
	traceCtx, span := s.tracer.Start(ctx, "ValidateUploadFilesAgainstWorkflow")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	sectionIDs, err := s.workflowResolver.ResolveSections(traceCtx, formID, answersForWorkflow, answerableMap)
	if err != nil {
		if errors.Is(err, internal.ErrWorkflowNotFound) {
			return nil
		}

		err = fmt.Errorf("%w: %w", internal.ErrWorkflowResolveSectionsFailed, err)
		logger.Warn("rejected file upload: workflow section resolution failed",
			zap.String("formID", formID.String()),
			zap.String("responseID", responseID.String()),
			zap.String("questionID", questionID.String()),
			zap.Error(err),
		)

		span.RecordError(err)
		return err
	}

	sectionActiveMap := make(map[string]bool, len(sectionIDs))
	for _, sid := range sectionIDs {
		sectionActiveMap[sid.String()] = true
	}

	answerable, ok := answerableMap[questionID.String()]
	if !ok {
		return nil
	}

	sectionIDStr := answerable.Question().SectionID.String()
	if !sectionActiveMap[sectionIDStr] {
		logger.Warn("rejected file upload for workflow-skipped section",
			zap.String("formID", formID.String()),
			zap.String("responseID", responseID.String()),
			zap.String("questionID", questionID.String()),
			zap.String("sectionID", sectionIDStr),
		)

		return internal.ErrAnswerSectionSkipped
	}

	return nil
}

package answer

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/file"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	ListByResponseID(ctx context.Context, responseID uuid.UUID) ([]Answer, error)
	GetByResponseIDAndQuestionID(ctx context.Context, arg GetByResponseIDAndQuestionIDParams) (Answer, error)
	BatchUpsert(ctx context.Context, arg BatchUpsertParams) ([]Answer, error)
}

type QuestionStore interface {
	ListSectionsWithAnswersByFormID(ctx context.Context, formID uuid.UUID) ([]question.SectionWithAnswerableList, error)
	GetAnswerableMapByFormID(ctx context.Context, formID uuid.UUID) (map[string]question.Answerable, error)
}

// FileService defines the file storage operations needed by the answer service
type FileService interface {
	SaveFile(ctx context.Context, fileContent io.Reader, originalFilename, contentType string, uploadedBy *uuid.UUID, opts ...file.ValidatorOption) (file.File, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type Answerable interface {
	Question() question.Question

	Validate(rawValue json.RawMessage) error

	DisplayValue(rawValue json.RawMessage) (string, error)

	// DecodeRequest decodes the raw JSON value from the request into the appropriate Go type based on the question type.
	DecodeRequest(param shared.AnswerParam) (any, error)

	// DecodeStorage decodes the raw JSON value from the database into the appropriate Go type based on the question type.
	DecodeStorage(rawValue json.RawMessage) (any, error)

	// EncodeRequest encodes the Go value into raw JSON for storage in the database or for sending in a response, based on the question type.
	EncodeRequest(answer any) (json.RawMessage, error)
}

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer

	questionStore QuestionStore
	fileService   FileService
}

func NewService(logger *zap.Logger, db DBTX, questionStore QuestionStore, fileService FileService) *Service {
	return &Service{
		logger:        logger,
		queries:       newLoggedQuerier(New(db), logger),
		tracer:        otel.Tracer("answer/service"),
		questionStore: questionStore,
		fileService:   fileService,
	}
}

func (s Service) List(ctx context.Context, formID, responseID uuid.UUID) ([]Answer, []question.Answerable, map[string]question.Answerable, error) {
	traceCtx, span := s.tracer.Start(ctx, "List")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)
	bizFlowLogger := withEvent(logger, eventTypeBizFlow)

	bizFlowLogger.Info(
		"Method List started",
		zap.String("method.name", "List"),
		zap.Any("method.params", map[string]string{
			"form_id":     formID.String(),
			"response_id": responseID.String(),
		}),
	)

	answers, err := s.queries.ListByResponseID(traceCtx, responseID)
	if err != nil {
		span.RecordError(err)
		return nil, nil, nil, fmt.Errorf("failed to retrieve answers by response id: %w", err)
	}

	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		span.RecordError(err)
		return nil, nil, nil, fmt.Errorf("failed to retrieve answerable map for form %s: %w", formID, err)
	}

	// Resolve dynamic choices for Ranking questions. In the read path there is
	// no request batch, so pass an empty slice, the resolver will fall back to
	// DB lookups using the stored answers for the source questions.
	answerableMap, err = s.resolveRankingChoices(traceCtx, responseID, answerableMap, nil)
	if err != nil {
		span.RecordError(err)
		return nil, nil, nil, fmt.Errorf("failed to resolve ranking choices: %w", err)
	}

	transformedAnswers := make([]Answer, 0, len(answers))
	answerableList := make([]question.Answerable, 0, len(answers))
	for _, answer := range answers {
		transformedAnswer, answerable, err := s.transformAnswerForResponse(traceCtx, answer, answerableMap, formID)
		if err != nil {
			span.RecordError(err)
			return nil, nil, nil, fmt.Errorf("failed to transform answer for question %s: %w", answer.QuestionID, err)
		}
		transformedAnswers = append(transformedAnswers, transformedAnswer)
		answerableList = append(answerableList, answerable)
	}

	bizFlowLogger.Info(
		"Method List completed",
		zap.String("service.name", "List"),
		zap.Any("service.result", map[string]any{
			"response_id": responseID.String(),
			"answer_cnt":  len(transformedAnswers),
		}),
	)

	return transformedAnswers, answerableList, answerableMap, nil
}

func (s Service) Get(ctx context.Context, formID, responseID, questionID uuid.UUID) (Answer, Answerable, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)
	bizFlowLogger := withEvent(logger, eventTypeBizFlow)

	bizFlowLogger.Info(
		"Method Get started",
		zap.String("method.name", "Get"),
		zap.Any("method.params", map[string]string{
			"form_id":     formID.String(),
			"response_id": responseID.String(),
			"question_id": questionID.String(),
		}),
	)

	answer, err := s.queries.GetByResponseIDAndQuestionID(traceCtx, GetByResponseIDAndQuestionIDParams{
		ResponseID: responseID,
		QuestionID: questionID,
	})
	if err != nil {
		span.RecordError(err)
		return Answer{}, nil, fmt.Errorf("failed to retrieve answer by response %s and question %s: %w", responseID, questionID, err)
	}

	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		span.RecordError(err)
		return Answer{}, nil, fmt.Errorf("failed to retrieve answerable map for form %s: %w", formID, err)
	}

	transformedAnswer, answerable, err := s.transformAnswerForResponse(traceCtx, answer, answerableMap, formID)
	if err != nil {
		span.RecordError(err)
		return Answer{}, nil, fmt.Errorf("failed to transform answer for question %s: %w", questionID, err)
	}

	bizFlowLogger.Info(
		"Method Get completed",
		zap.String("service.name", "Get"),
		zap.Any("service.result", map[string]string{
			"response_id": responseID.String(),
			"question_id": transformedAnswer.QuestionID.String(),
		}),
	)

	return transformedAnswer, answerable, nil
}

// transformAnswerForResponse transforms an answer from storage format to response format
func (s Service) transformAnswerForResponse(ctx context.Context, answer Answer, answerableMap map[string]question.Answerable, formID uuid.UUID) (Answer, question.Answerable, error) {
	_, span := s.tracer.Start(ctx, "transformAnswerForResponse")
	defer span.End()

	questionID := answer.QuestionID

	answerable, found := answerableMap[questionID.String()]
	if !found {
		return Answer{}, nil, fmt.Errorf("question with ID %s not found in form %s", questionID, formID)
	}

	return answer, answerable, nil
}

// Upsert validates and upserts answers for a given form response. It returns the upserted answers and any validation errors that occurred during the process.
func (s Service) Upsert(ctx context.Context, formID, responseID uuid.UUID, answers []shared.AnswerParam) ([]Answer, []Answerable, []error) {
	answerQuestionPairs := make([]struct {
		AnswerParam shared.AnswerParam
		Answerable  Answerable
	}, 0, len(answers))

	traceCtx, span := s.tracer.Start(ctx, "Upsert")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)
	bizFlowLogger := withEvent(logger, eventTypeBizFlow)
	bizLogicLogger := withEvent(logger, eventTypeBizLogic)
	codecLogger := withEvent(logger, eventTypeUtilCodec)

	bizFlowLogger.Info(
		"Method Upsert started",
		zap.String("method.name", "Upsert"),
		zap.Any("method.params", map[string]any{
			"form_id":      formID.String(),
			"response_id":  responseID.String(),
			"answer_count": len(answers),
		}),
	)

	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		span.RecordError(err)
		return nil, nil, []error{err}
	}

	// Resolve dynamic choices for Ranking questions whose options come from a
	// source DetailedMultiChoice question's stored answer.
	answerableMap, err = s.resolveRankingChoices(traceCtx, responseID, answerableMap, answers)
	if err != nil {
		span.RecordError(err)
		return nil, nil, []error{err}
	}

	validationErrors := make([]error, 0)
	answeredQuestionIDs := make(map[string]bool)

	for _, ans := range answers {
		answerable, found := answerableMap[ans.QuestionID]
		if !found {
			bizLogicLogger.Warn(
				"Decision made: question_not_found_in_form, action: reject_answer",
				zap.String("logic.category", "decision_point"),
				zap.String("logic.reason", "question_not_found_in_form"),
				zap.String("logic.action", "reject_answer"),
				zap.Any("logic.context", map[string]string{
					"form_id":     formID.String(),
					"question_id": ans.QuestionID,
				}),
			)
			validationErrors = append(validationErrors, fmt.Errorf("question with ID %s not found in form %s", ans.QuestionID, formID))
			continue
		}

		answeredQuestionIDs[ans.QuestionID] = true

		// Validate answer value (convert string to json.RawMessage)
		err := answerable.Validate(ans.Value)
		if err != nil {
			bizLogicLogger.Warn(
				"Decision made: answer_validation_failed, action: reject_answer",
				zap.String("logic.category", "decision_point"),
				zap.String("logic.reason", "answer_validation_failed"),
				zap.String("logic.action", "reject_answer"),
				zap.Any("logic.context", map[string]string{
					"question_id": ans.QuestionID,
				}),
			)
			validationErrors = append(validationErrors, fmt.Errorf("validation error for question ID %s: %w", ans.QuestionID, err))
		}

		answerQuestionPairs = append(answerQuestionPairs, struct {
			AnswerParam shared.AnswerParam
			Answerable  Answerable
		}{
			AnswerParam: ans,
			Answerable:  answerable,
		})
	}

	if len(validationErrors) > 0 {
		bizLogicLogger.Warn(
			"Decision made: validation_failed, action: reject_upsert",
			zap.String("logic.category", "decision_point"),
			zap.String("logic.reason", "validation_failed"),
			zap.String("logic.action", "reject_upsert"),
			zap.Any("logic.context", map[string]any{
				"error_count": len(validationErrors),
			}),
		)
		span.RecordError(internal.ErrValidationFailed)
		validationErrors = append([]error{internal.ErrValidationFailed}, validationErrors...)
		return nil, nil, validationErrors
	}

	// Prepare batch upsert parameters
	responseIDs := make([]uuid.UUID, len(answers))
	questionIDs := make([]uuid.UUID, len(answers))
	values := make([][]byte, len(answers))

	for i, pair := range answerQuestionPairs {
		responseIDs[i] = responseID

		questionID, err := uuid.Parse(pair.AnswerParam.QuestionID)
		if err != nil {
			codecLogger.Warn(
				"failed to parse question UUID",
				zap.String("codec.operation", "uuid_parse"),
				zap.String("question_id", pair.AnswerParam.QuestionID),
				zap.Error(err),
			)
			span.RecordError(fmt.Errorf("invalid question ID format for question ID %s: %w", pair.AnswerParam.QuestionID, err))
			return nil, nil, []error{fmt.Errorf("invalid question ID format for question ID %s: %w", pair.AnswerParam.QuestionID, err)}
		}

		questionIDs[i] = questionID

		encodedValue, err := pair.Answerable.DecodeRequest(pair.AnswerParam)
		if err != nil {
			codecLogger.Warn(
				"failed to decode request answer payload",
				zap.String("codec.operation", "decode_request"),
				zap.String("question_id", pair.AnswerParam.QuestionID),
				zap.Error(err),
			)
			span.RecordError(fmt.Errorf("failed to encode answer value for question ID %s: %w", pair.AnswerParam.QuestionID, err))
			return nil, nil, []error{fmt.Errorf("failed to encode answer value for question ID %s: %w", pair.AnswerParam.QuestionID, err)}
		}

		jsonValue, err := json.Marshal(encodedValue)
		if err != nil {
			codecLogger.Error(
				"failed to marshal answer payload for storage",
				zap.String("codec.operation", "json_marshal"),
				zap.String("question_id", pair.AnswerParam.QuestionID),
				zap.Error(err),
			)
			span.RecordError(fmt.Errorf("failed to marshal encoded answer value to JSON for question ID %s: %w", pair.AnswerParam.QuestionID, err))
			return nil, nil, []error{fmt.Errorf("failed to marshal encoded answer value to JSON for question ID %s: %w", pair.AnswerParam.QuestionID, err)}
		}

		values[i] = jsonValue
	}

	// Batch upsert answers into database
	upsertedAnswers, err := s.queries.BatchUpsert(traceCtx, BatchUpsertParams{
		ResponseIds: responseIDs,
		QuestionIds: questionIDs,
		Values:      values,
	})
	if err != nil {
		span.RecordError(err)
		return nil, nil, []error{fmt.Errorf("failed to save answers: %w", err)}
	}

	transformedAnswers := make([]Answer, 0, len(answers))
	answerableList := make([]Answerable, 0, len(answers))
	for _, answer := range upsertedAnswers {
		transformedAnswer, answerable, err := s.transformAnswerForResponse(traceCtx, answer, answerableMap, formID)
		if err != nil {
			span.RecordError(err)
			return nil, nil, []error{fmt.Errorf("failed to transform answer for question %s: %w", answer.QuestionID, err)}
		}
		transformedAnswers = append(transformedAnswers, transformedAnswer)
		answerableList = append(answerableList, answerable)
	}

	bizFlowLogger.Info(
		"Method Upsert completed",
		zap.String("service.name", "Upsert"),
		zap.Any("service.result", map[string]any{
			"response_id": responseID.String(),
			"answer_cnt":  len(upsertedAnswers),
		}),
	)

	return transformedAnswers, answerableList, nil
}

// resolveRankingChoices populates the Rank field of every Ranking answerable
// whose SourceID points to a DetailedMultiChoice question.
//
// Resolution order:
//  1. Look for the source question's answer in the current request batch.
//     The batch value is a JSON []string of selected choice IDs; filter the
//     source question's Choices accordingly.
//  2. If not found in the batch, query the database for the response's stored
//     answer of the source question and decode it as a
//     shared.DetailedMultipleChoiceAnswer.
//
// If the source answer is absent in both places the Ranking's Rank stays nil,
// which means Validate will pass for an empty submission but reject any
// submitted choice IDs (they cannot be validated against an empty list).
func (s Service) resolveRankingChoices(
	ctx context.Context,
	responseID uuid.UUID,
	answerableMap map[string]question.Answerable,
	requestAnswers []shared.AnswerParam,
) (map[string]question.Answerable, error) {
	// Build a quick lookup for answers included in this request batch.
	requestAnswerMap := make(map[string]json.RawMessage, len(requestAnswers))
	for _, ans := range requestAnswers {
		requestAnswerMap[ans.QuestionID] = ans.Value
	}

	for questionID, answerable := range answerableMap {
		ranking, ok := answerable.(question.Ranking)
		if !ok {
			continue
		}

		sourceID := ranking.SourceID()
		if !sourceID.Valid {
			continue
		}

		sourceQIDStr := sourceID.UUID.String()

		// Try the request batch first
		if rawVal, found := requestAnswerMap[sourceQIDStr]; found {
			sourceAnswerable, found := answerableMap[sourceQIDStr]
			if !found {
				return nil, fmt.Errorf("source question %s for ranking question %s not found in form", sourceQIDStr, questionID)
			}

			dmc, ok := sourceAnswerable.(question.DetailedMultiChoice)
			if !ok {
				return nil, fmt.Errorf("source question %s for ranking question %s is not a detailed_multiple_choice question", sourceQIDStr, questionID)
			}

			// Request format is []string of selected choice IDs.
			var selectedIDs []string
			if err := json.Unmarshal(rawVal, &selectedIDs); err != nil {
				return nil, fmt.Errorf("failed to parse source answer for ranking question %s: %w", questionID, err)
			}

			selectedSet := make(map[string]bool, len(selectedIDs))
			for _, id := range selectedIDs {
				selectedSet[id] = true
			}

			choices := make([]question.Choice, 0, len(selectedIDs))
			for _, c := range dmc.Choices {
				if selectedSet[c.ID.String()] {
					choices = append(choices, c)
				}
			}

			answerableMap[questionID] = ranking.WithSourceChoices(choices)
			continue
		}

		// Fall back to the stored DB answer
		stored, err := s.queries.GetByResponseIDAndQuestionID(ctx, GetByResponseIDAndQuestionIDParams{
			ResponseID: responseID,
			QuestionID: sourceID.UUID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Source question has no stored answer, return empty choices so that validation will fail for any submitted choice IDs
			choices := make([]question.Choice, 0)
			answerableMap[questionID] = ranking.WithSourceChoices(choices)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get source answer for ranking question %s: %w", questionID, err)
		}

		choices, err := extractChoicesFromStoredDetailedMultiAnswer(stored.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to extract choices from stored source answer for ranking question %s: %w", questionID, err)
		}

		answerableMap[questionID] = ranking.WithSourceChoices(choices)
	}

	return answerableMap, nil
}

// extractChoicesFromStoredDetailedMultiAnswer decodes a storage-format
// DetailedMultipleChoiceAnswer and converts its entries to []question.Choice.
func extractChoicesFromStoredDetailedMultiAnswer(rawValue []byte) ([]question.Choice, error) {
	var answer shared.DetailedMultipleChoiceAnswer
	err := json.Unmarshal(rawValue, &answer)
	if err != nil {
		return nil, fmt.Errorf("invalid detailed_multiple_choice answer in storage: %w", err)
	}

	choices := make([]question.Choice, len(answer.Choices))
	for i, c := range answer.Choices {
		choices[i] = question.Choice{
			ID:          c.ChoiceID,
			Name:        c.Snapshot.Name,
			Description: c.Snapshot.Description,
		}
	}
	return choices, nil
}

// UploadFiles uploads files for an upload_file question and upserts the answer.// It validates that the question exists, belongs to the form, and is of type upload_file.
// Files are saved via fileService, and the resulting file IDs are stored as the answer.
//
// Eventual consistency for orphan file cleanup:
//   - If saving a new file fails mid-loop, already-saved new files are cleaned up.
//   - If Upsert fails, all newly saved files are cleaned up.
//   - After a successful Upsert, old file IDs from the previous answer are deleted
//     on a best-effort basis; failures are logged as warnings but do not affect the response.
func (s Service) UploadFiles(ctx context.Context, formID, responseID, questionID uuid.UUID, files []*multipart.FileHeader, uploadedBy *uuid.UUID) ([]shared.UploadFileEntry, Answer, Answerable, error) {
	traceCtx, span := s.tracer.Start(ctx, "UploadFiles")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)
	bizFlowLogger := withEvent(logger, eventTypeBizFlow)
	bizLogicLogger := withEvent(logger, eventTypeBizLogic)
	codecLogger := withEvent(logger, eventTypeUtilCodec)

	bizFlowLogger.Info(
		"Method UploadFiles started",
		zap.String("method.name", "UploadFiles"),
		zap.Any("method.params", map[string]any{
			"form_id":      formID.String(),
			"response_id":  responseID.String(),
			"question_id":  questionID.String(),
			"upload_count": len(files),
		}),
	)

	// Get the answerable map to validate question type and membership
	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		span.RecordError(err)
		return nil, Answer{}, nil, fmt.Errorf("failed to retrieve answerable map for form %s: %w", formID, internal.ErrInternalServerError)
	}

	answerable, found := answerableMap[questionID.String()]
	if !found {
		bizLogicLogger.Warn(
			"Decision made: question_not_found_in_form, action: reject_upload",
			zap.String("logic.category", "decision_point"),
			zap.String("logic.reason", "question_not_found_in_form"),
			zap.String("logic.action", "reject_upload"),
			zap.Any("logic.context", map[string]string{
				"form_id":     formID.String(),
				"question_id": questionID.String(),
			}),
		)
		return nil, Answer{}, nil, internal.ErrQuestionNotFound
	}

	// Validate the question is of upload_file type
	if answerable.Question().Type != question.QuestionTypeUploadFile {
		bizLogicLogger.Warn(
			"Decision made: question_type_mismatch, action: reject_upload",
			zap.String("logic.category", "decision_point"),
			zap.String("logic.reason", "question_type_mismatch"),
			zap.String("logic.action", "reject_upload"),
			zap.Any("logic.context", map[string]string{
				"question_id":   questionID.String(),
				"expected_type": string(question.QuestionTypeUploadFile),
				"actual_type":   string(answerable.Question().Type),
			}),
		)
		span.RecordError(internal.ErrQuestionTypeMismatch)
		return nil, Answer{}, nil, internal.ErrQuestionTypeMismatch
	}

	// Read old file IDs from the existing answer (if any) for later cleanup
	var oldFileIDs []string
	existingAnswer, err := s.queries.GetByResponseIDAndQuestionID(traceCtx, GetByResponseIDAndQuestionIDParams{
		ResponseID: responseID,
		QuestionID: questionID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		span.RecordError(err)
		return nil, Answer{}, nil, fmt.Errorf("failed to retrieve existing answer for question %s: %w", questionID, internal.ErrInternalServerError)
	}
	if err == nil {
		// Extract old file IDs from the stored answer for later cleanup
		var existingUploadAnswer shared.UploadFileAnswer
		if jsonErr := json.Unmarshal(existingAnswer.Value, &existingUploadAnswer); jsonErr == nil {
			for _, entry := range existingUploadAnswer.Files {
				oldFileIDs = append(oldFileIDs, entry.FileID.String())
			}
		} else {
			codecLogger.Warn(
				"failed to unmarshal existing upload_file answer",
				zap.String("codec.operation", "json_unmarshal"),
				zap.String("question_id", questionID.String()),
				zap.Error(jsonErr),
			)
		}
	}

	// deleteFiles is a best-effort helper that logs warnings on failure
	deleteFiles := func(ids []string) {
		for _, idStr := range ids {
			id, parseErr := uuid.Parse(idStr)
			if parseErr != nil {
				logger.Warn("failed to parse file ID for cleanup", zap.String("fileID", idStr), zap.Error(parseErr))
				continue
			}
			if delErr := s.fileService.Delete(traceCtx, id); delErr != nil {
				logger.Warn("failed to delete orphan file", zap.String("fileID", idStr), zap.Error(delErr))
			}
		}
	}

	// Save each uploaded file; on failure clean up any already-saved new files
	entries := make([]shared.UploadFileEntry, 0, len(files))
	fileIDs := make([]string, 0, len(files))

	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			span.RecordError(err)
			deleteFiles(fileIDs)
			return nil, Answer{}, nil, fmt.Errorf("failed to open uploaded file %q: %w", fh.Filename, internal.ErrFailedToSaveFile)
		}

		savedFile, saveErr := s.fileService.SaveFile(traceCtx, f, fh.Filename, fh.Header.Get("Content-Type"), uploadedBy)
		_ = f.Close()

		if saveErr != nil {
			span.RecordError(saveErr)
			deleteFiles(fileIDs)
			return nil, Answer{}, nil, fmt.Errorf("failed to save file %q: %w", fh.Filename, internal.ErrFailedToSaveFile)
		}

		fileIDs = append(fileIDs, savedFile.ID.String())
		entries = append(entries, shared.UploadFileEntry{
			FileID:           savedFile.ID,
			OriginalFilename: savedFile.OriginalFilename,
			ContentType:      savedFile.ContentType,
			Size:             savedFile.Size,
		})
	}

	// Build the answer value as a full UploadFileAnswer and upsert
	answerValue, err := json.Marshal(shared.UploadFileAnswer{Files: entries})
	if err != nil {
		codecLogger.Error(
			"failed to marshal upload_file answer payload",
			zap.String("codec.operation", "json_marshal"),
			zap.String("question_id", questionID.String()),
			zap.Error(err),
		)
		span.RecordError(err)
		deleteFiles(fileIDs)
		return nil, Answer{}, nil, fmt.Errorf("failed to marshal upload file answer: %w", internal.ErrInternalServerError)
	}

	upsertedAnswers, answerableList, errs := s.Upsert(traceCtx, formID, responseID, []shared.AnswerParam{
		{QuestionID: questionID.String(), Value: answerValue},
	})
	if len(errs) > 0 {
		span.RecordError(errs[0])
		deleteFiles(fileIDs)
		return nil, Answer{}, nil, fmt.Errorf("failed to upsert upload file answer: %w", errs[0])
	}

	// Best-effort: delete old files now that the new answer is committed
	if len(oldFileIDs) > 0 {
		deleteFiles(oldFileIDs)
	}

	bizFlowLogger.Info(
		"Method UploadFiles completed",
		zap.String("service.name", "UploadFiles"),
		zap.Any("service.result", map[string]any{
			"question_id": questionID.String(),
			"file_count":  len(fileIDs),
		}),
	)

	return entries, upsertedAnswers[0], answerableList[0], nil
}

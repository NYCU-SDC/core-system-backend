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

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	ListByResponseID(ctx context.Context, responseID uuid.UUID) ([]Answer, error)
	GetByID(ctx context.Context, id uuid.UUID) (Answer, error)
	GetByResponseIDAndQuestionID(ctx context.Context, arg GetByResponseIDAndQuestionIDParams) (Answer, error)
	BatchUpsert(ctx context.Context, arg BatchUpsertParams) ([]Answer, error)
	WithTx(tx pgx.Tx) *Queries
}

type TxBeginner interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

type QuestionStore interface {
	ListSectionsWithAnswersByFormID(ctx context.Context, formID uuid.UUID) ([]question.SectionWithAnswerableList, error)
	GetAnswerableMapByFormID(ctx context.Context, formID uuid.UUID) (map[string]question.Answerable, error)
}

// FileService defines the file storage operations needed by the answer service
type FileService interface {
	SaveFile(ctx context.Context, fileContent io.Reader, originalFilename, contentType string, uploadedBy *uuid.UUID, opts ...file.ValidatorOption) (file.File, error)
	CreateAttachment(ctx context.Context, fileID uuid.UUID, resourceType file.ResourceType, resourceID uuid.UUID, createdBy uuid.UUID) (file.FileAttachment, error)
	DeletePhysicalFile(ctx context.Context, fileID uuid.UUID) error
	WithTx(tx pgx.Tx) *file.Service
}

type WorkflowResolver interface {
	ResolveSections(ctx context.Context, formID uuid.UUID, answers []Answer, answerableMap map[string]question.Answerable) ([]uuid.UUID, error)
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
	logger           *zap.Logger
	db               DBTX
	queries          Querier
	tracer           trace.Tracer
	workflowResolver WorkflowResolver

	questionStore QuestionStore
	fileService   FileService
}

func NewService(logger *zap.Logger, db DBTX, questionStore QuestionStore, fileService FileService, workflowResolver WorkflowResolver) *Service {
	return &Service{
		logger:           logger,
		db:               db,
		queries:          New(db),
		tracer:           otel.Tracer("answer/service"),
		workflowResolver: workflowResolver,
		questionStore:    questionStore,
		fileService:      fileService,
	}
}

func (s Service) List(ctx context.Context, formID, responseID uuid.UUID) ([]Answer, []question.Answerable, map[string]question.Answerable, error) {
	traceCtx, span := s.tracer.Start(ctx, "List")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	answers, err := s.queries.ListByResponseID(traceCtx, responseID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list answers")
		span.RecordError(err)
		return nil, nil, nil, fmt.Errorf("failed to list answers: %w", err)
	}

	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get answerable map for form ID %s: %w", formID, err)
	}

	// Resolve dynamic choices for Ranking questions. In the read path there is
	// no request batch, so pass an empty slice, the resolver will fall back to
	// DB lookups using the stored answers for the source questions.
	answerableMap, err = s.ResolveRankingChoices(traceCtx, responseID, answerableMap, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve ranking choices: %w", err)
	}

	transformedAnswers := make([]Answer, 0, len(answers))
	answerableList := make([]question.Answerable, 0, len(answers))
	for _, answer := range answers {
		transformedAnswer, answerable, err := s.transformAnswerForResponse(traceCtx, answer, answerableMap, formID)
		if err != nil {
			logger.Error("failed to transform answer", zap.String("questionID", answer.QuestionID.String()), zap.Error(err))
			span.RecordError(err)
			return nil, nil, nil, err
		}
		transformedAnswers = append(transformedAnswers, transformedAnswer)
		answerableList = append(answerableList, answerable)
	}

	logger.Info("successfully listed answers", zap.Int("count", len(transformedAnswers)), zap.String("responseID", responseID.String()))
	return transformedAnswers, answerableList, answerableMap, nil
}

func (s Service) Get(ctx context.Context, formID, responseID, questionID uuid.UUID) (Answer, Answerable, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	answer, err := s.queries.GetByResponseIDAndQuestionID(traceCtx, GetByResponseIDAndQuestionIDParams{
		ResponseID: responseID,
		QuestionID: questionID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get answer by response ID and question ID")
		span.RecordError(err)
		return Answer{}, nil, fmt.Errorf("failed to get answer for response ID %s and question ID %s: %w", responseID, questionID, err)
	}

	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		return Answer{}, nil, fmt.Errorf("failed to get answerable map for form ID %s: %w", formID, err)
	}

	transformedAnswer, answerable, err := s.transformAnswerForResponse(traceCtx, answer, answerableMap, formID)
	if err != nil {
		span.RecordError(err)
		return Answer{}, nil, err
	}

	logger.Info("successfully retrieved answer", zap.String("responseID", responseID.String()), zap.String("questionID", questionID.String()))

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

	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		return nil, nil, []error{err}
	}

	// Resolve dynamic choices for Ranking questions whose options come from a
	// source DetailedMultiChoice question's stored answer.
	answerableMap, err = s.ResolveRankingChoices(traceCtx, responseID, answerableMap, answers)
	if err != nil {
		return nil, nil, []error{err}
	}

	validationErrors := make([]error, 0)
	answeredQuestionIDs := make(map[string]bool)

	for _, ans := range answers {
		answerable, found := answerableMap[ans.QuestionID]
		if !found {
			validationErrors = append(validationErrors, fmt.Errorf("question with ID %s not found in form %s", ans.QuestionID, formID))
			continue
		}

		answeredQuestionIDs[ans.QuestionID] = true

		// Validate answer value (convert string to json.RawMessage)
		err := answerable.Validate(ans.Value)
		if err != nil {
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
		logger.Error("validation errors occurred", zap.Error(fmt.Errorf("validation errors occurred")), zap.Any("errors", validationErrors))
		span.RecordError(fmt.Errorf("validation errors occurred"))
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
			logger.Error("invalid question ID format", zap.String("questionID", pair.AnswerParam.QuestionID), zap.Error(err))
			span.RecordError(fmt.Errorf("invalid question ID format for question ID %s: %w", pair.AnswerParam.QuestionID, err))
			return nil, nil, []error{fmt.Errorf("invalid question ID format for question ID %s: %w", pair.AnswerParam.QuestionID, err)}
		}

		questionIDs[i] = questionID

		encodedValue, err := pair.Answerable.DecodeRequest(pair.AnswerParam)
		if err != nil {
			logger.Error("failed to encode answer value for storage", zap.String("questionID", pair.AnswerParam.QuestionID), zap.Error(err))
			span.RecordError(fmt.Errorf("failed to encode answer value for question ID %s: %w", pair.AnswerParam.QuestionID, err))
			return nil, nil, []error{fmt.Errorf("failed to encode answer value for question ID %s: %w", pair.AnswerParam.QuestionID, err)}
		}

		jsonValue, err := json.Marshal(encodedValue)
		if err != nil {
			logger.Error("failed to marshal encoded answer value to JSON", zap.String("questionID", pair.AnswerParam.QuestionID), zap.Error(err))
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
		err = databaseutil.WrapDBError(err, logger, "batch upsert answers")
		span.RecordError(err)
		return nil, nil, []error{fmt.Errorf("failed to save answers: %w", err)}
	}

	transformedAnswers := make([]Answer, 0, len(answers))
	answerableList := make([]Answerable, 0, len(answers))
	for _, answer := range upsertedAnswers {
		transformedAnswer, answerable, err := s.transformAnswerForResponse(traceCtx, answer, answerableMap, formID)
		if err != nil {
			logger.Error("failed to transform answer", zap.String("questionID", answer.QuestionID.String()), zap.Error(err))
			span.RecordError(err)
			return nil, nil, []error{err}
		}
		transformedAnswers = append(transformedAnswers, transformedAnswer)
		answerableList = append(answerableList, answerable)
	}

	logger.Info("successfully upserted answers", zap.Int("count", len(upsertedAnswers)))
	return transformedAnswers, answerableList, nil
}

// ResolveRankingChoices populates the Rank field of every Ranking answerable
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
//
// For PATCH, pass the incoming answers so List()-only DB resolution can be
// overridden when DMC and RANKING are submitted together (Upsert and workflow
// merge already call this with the batch).
func (s Service) ResolveRankingChoices(
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

// UploadFiles uploads files for an upload_file question and upserts the answer.
// It validates that the question exists, belongs to the form, and is of type upload_file.
//
// Existing upload_file entries are loaded first, then new file rows, answer upsert,
// and file attachments are all executed within the same database transaction.
// Any failure before commit rolls back the whole operation.
func (s Service) UploadFiles(ctx context.Context, formID, responseID, questionID uuid.UUID, files []*multipart.FileHeader, uploadedBy uuid.UUID) ([]shared.UploadFileEntry, Answer, Answerable, error) {
	traceCtx, span := s.tracer.Start(ctx, "UploadFiles")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Get the answerable map to validate question type and membership
	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		logger.Error("failed to get answerable map for form", zap.String("formID", formID.String()), zap.Error(err))
		span.RecordError(err)
		return nil, Answer{}, nil, err
	}

	answerable, found := answerableMap[questionID.String()]
	if !found {
		return nil, Answer{}, nil, internal.ErrQuestionNotFound
	}

	// Validate the question is of upload_file type
	if answerable.Question().Type != question.QuestionTypeUploadFile {
		logger.Error("invalid question type", zap.String("questionID", questionID.String()), zap.String("expectedType", string(question.QuestionTypeUploadFile)), zap.String("actualType", string(answerable.Question().Type)))
		span.RecordError(internal.ErrQuestionTypeMismatch)
		return nil, Answer{}, nil, internal.ErrQuestionTypeMismatch
	}

	// Read existing uploaded files from the existing answer (if any) for append.
	var (
		tx         pgx.Tx
		needCommit bool
	)

	tx, needCommit, err = s.beginOrReuseTx(traceCtx)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "begin tx for upload files")
		span.RecordError(err)
		return nil, Answer{}, nil, err
	}

	if needCommit {
		defer func() {
			rollbackErr := tx.Rollback(traceCtx)
			if rollbackErr == nil {
				return
			}
			if errors.Is(rollbackErr, pgx.ErrTxClosed) {
				return
			}

			logger.Error("rollback failed", zap.Error(rollbackErr))
			span.RecordError(rollbackErr)
		}()
	}

	qtx := s.queries.WithTx(tx)
	ftx := s.fileService.WithTx(tx)

	existingEntries, err := s.loadPreviousUploadFileEntries(traceCtx, qtx, responseID, questionID)
	if err != nil {
		logger.Error("failed to load previous upload file entries",
			zap.String("questionID", questionID.String()),
			zap.Error(err),
		)
		span.RecordError(err)
		return nil, Answer{}, nil, err
	}

	// Save each uploaded file; on failure clean up any already-saved new files
	newEntries := make([]shared.UploadFileEntry, 0, len(files))

	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			logger.Error("failed to open uploaded file", zap.String("filename", fh.Filename), zap.Error(err))
			span.RecordError(err)
			return nil, Answer{}, nil, fmt.Errorf("failed to open uploaded file %q: %w", fh.Filename, internal.ErrFailedToSaveFile)
		}

		savedFile, saveErr := ftx.SaveFile(traceCtx, f, fh.Filename, fh.Header.Get("Content-Type"), &uploadedBy)
		closeErr := f.Close()
		if closeErr != nil {
			logger.Warn("failed to close uploaded file stream",
				zap.String("filename", fh.Filename),
				zap.Error(closeErr),
			)
		}

		if saveErr != nil {
			logger.Error("failed to save uploaded file", zap.String("filename", fh.Filename), zap.Error(saveErr))
			span.RecordError(saveErr)
			return nil, Answer{}, nil, fmt.Errorf("failed to save file %q: %w", fh.Filename, internal.ErrFailedToSaveFile)
		}

		newEntries = append(newEntries, shared.UploadFileEntry{
			FileID:           savedFile.ID,
			OriginalFilename: savedFile.OriginalFilename,
			ContentType:      savedFile.ContentType,
			Size:             savedFile.Size,
		})
	}

	// Append new files to existing files
	mergedEntries := make([]shared.UploadFileEntry, 0, len(existingEntries)+len(newEntries))
	mergedEntries = append(mergedEntries, existingEntries...)
	mergedEntries = append(mergedEntries, newEntries...)

	// Build the answer value as a full UploadFileAnswer and upsert
	answerValue, err := json.Marshal(shared.UploadFileAnswer{Files: mergedEntries})
	if err != nil {
		logger.Error("failed to marshal upload file answer value", zap.String("questionID", questionID.String()), zap.Error(err))
		span.RecordError(err)
		return nil, Answer{}, nil, fmt.Errorf("failed to marshal upload file answer: %w", internal.ErrValidationFailed)
	}

	upsertedAnswers, err := qtx.BatchUpsert(
		traceCtx,
		BatchUpsertParams{
			ResponseIds: []uuid.UUID{responseID},
			QuestionIds: []uuid.UUID{questionID},
			Values:      [][]byte{answerValue},
		})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "batch upsert upload file answer")
		span.RecordError(err)
		return nil, Answer{}, nil, fmt.Errorf("failed to upsert upload file answer: %w", err)
	}

	upsertedAnswer := upsertedAnswers[0]

	for _, entry := range newEntries {
		_, attachErr := ftx.CreateAttachment(
			traceCtx,
			entry.FileID,
			file.ResourceTypeFormAnswer,
			upsertedAnswer.ID,
			uploadedBy,
		)
		if attachErr != nil {
			logger.Error("failed to create file attachment after upload answer upsert",
				zap.String("answerID", upsertedAnswer.ID.String()),
				zap.String("fileID", entry.FileID.String()),
				zap.Error(attachErr),
			)
			span.RecordError(attachErr)
			return nil, Answer{}, nil, fmt.Errorf("failed to create attachment for file %s: %w", entry.FileID, attachErr)
		}
	}

	if needCommit {
		if err := tx.Commit(traceCtx); err != nil {
			err = databaseutil.WrapDBError(err, logger, "commit upload files tx")
			span.RecordError(err)
			return nil, Answer{}, nil, err
		}
	}

	logger.Info("successfully uploaded files and upserted answer",
		zap.String("questionID", questionID.String()),
		zap.String("answerID", upsertedAnswer.ID.String()),
		zap.Int("fileCount", len(mergedEntries)),
	)

	return newEntries, upsertedAnswer, answerable, nil
}

func (s Service) loadPreviousUploadFileEntries(ctx context.Context, q Querier, responseID uuid.UUID, questionID uuid.UUID) ([]shared.UploadFileEntry, error) {
	existingAnswer, err := q.GetByResponseIDAndQuestionID(ctx, GetByResponseIDAndQuestionIDParams{
		ResponseID: responseID,
		QuestionID: questionID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []shared.UploadFileEntry{}, nil
		}
		return nil, err
	}

	var existingUploadAnswer shared.UploadFileAnswer
	err = json.Unmarshal(existingAnswer.Value, &existingUploadAnswer)
	if err != nil {
		return nil, fmt.Errorf("unmarshal existing upload_file answer: %w", internal.ErrValidationFailed)
	}

	if existingUploadAnswer.Files == nil {
		return []shared.UploadFileEntry{}, nil
	}

	return existingUploadAnswer.Files, nil
}

func (s Service) beginOrReuseTx(
	ctx context.Context,
) (pgx.Tx, bool, error) {
	if existingTx, ok := s.db.(pgx.Tx); ok {
		return existingTx, false, nil
	}

	beginner, ok := s.db.(TxBeginner)
	if !ok {
		return nil, false, fmt.Errorf("db does not support transactions")
	}

	tx, err := beginner.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, false, err
	}

	return tx, true, nil
}

// MergeAnswersForWorkflowResolution returns a copy of current answers with each payload
// applied by question ID (payload wins). Request values are normalized with the same
// DecodeRequest + JSON marshal path as Upsert so workflow resolution (DecodeStorage /
// MatchesPattern) sees storage-shaped bytes, not raw API wire JSON.
func (s Service) MergeAnswersForWorkflowResolution(
	ctx context.Context,
	currentAnswers []Answer,
	payloads []Payload,
	answerableMap map[string]question.Answerable,
) ([]Answer, error) {
	traceCtx, span := s.tracer.Start(ctx, "MergeAnswersForWorkflowResolution")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

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
			err = fmt.Errorf("%w: invalid questionId %q: %w", internal.ErrWorkflowMergeInvalidQuestionID, payload.QuestionID, err)
			logger.Error("workflow merge: invalid question ID", zap.String("questionID", payload.QuestionID), zap.Error(err))
			span.RecordError(err)
			return nil, err
		}

		answerable, ok := answerableMap[payload.QuestionID]
		if !ok {
			err := fmt.Errorf("%w: question %s not found in form", internal.ErrWorkflowMergeQuestionNotInForm, payload.QuestionID)
			logger.Error("workflow merge: question not in form", zap.String("questionID", payload.QuestionID), zap.Error(err))
			span.RecordError(err)
			return nil, err
		}

		decoded, err := answerable.DecodeRequest(shared.AnswerParam{
			QuestionID: payload.QuestionID,
			Value:      payload.Value,
			OtherText:  payload.OtherText,
		})
		if err != nil {
			err = fmt.Errorf("%w: answer value for question %s: %w", internal.ErrWorkflowMergeAnswerValueInvalid, payload.QuestionID, err)
			logger.Error("workflow merge: decode answer failed", zap.String("questionID", payload.QuestionID), zap.Error(err))
			span.RecordError(err)
			return nil, err
		}

		storageBytes, err := json.Marshal(decoded)
		if err != nil {
			err = fmt.Errorf("%w: encode answer for question %s: %w", internal.ErrWorkflowMergeAnswerEncodeFailed, payload.QuestionID, err)
			logger.Error("workflow merge: marshal encoded answer failed", zap.String("questionID", payload.QuestionID), zap.Error(err))
			span.RecordError(err)
			return nil, err
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

	logger.Info("merged answers for workflow resolution",
		zap.Int("payloadCount", len(payloads)),
		zap.Int("mergedAnswerCount", len(out)),
	)
	return out, nil
}

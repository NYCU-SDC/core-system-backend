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
	DecodeRequest(rawValue json.RawMessage) (any, error)

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
		queries:       New(db),
		tracer:        otel.Tracer("answer/service"),
		questionStore: questionStore,
		fileService:   fileService,
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

		encodedValue, err := pair.Answerable.DecodeRequest(pair.AnswerParam.Value)
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

// UploadFiles uploads files for an upload_file question and upserts the answer.
// It validates that the question exists, belongs to the form, and is of type upload_file.
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

	// Get the answerable map to validate question type and membership
	answerableMap, err := s.questionStore.GetAnswerableMapByFormID(traceCtx, formID)
	if err != nil {
		s.logger.Error("failed to get answerable map for form", zap.String("formID", formID.String()), zap.Error(err))
		span.RecordError(err)
		return nil, Answer{}, nil, internal.ErrInternalServerError
	}

	answerable, found := answerableMap[questionID.String()]
	if !found {
		return nil, Answer{}, nil, internal.ErrQuestionNotFound
	}

	// Validate the question is of upload_file type
	if answerable.Question().Type != question.QuestionTypeUploadFile {
		s.logger.Error("invalid question type", zap.String("questionID", questionID.String()), zap.String("expectedType", string(question.QuestionTypeUploadFile)), zap.String("actualType", string(answerable.Question().Type)))
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
		s.logger.Error("failed to get existing answer for answer", zap.String("questionID", questionID.String()), zap.Error(err))
		span.RecordError(err)
		return nil, Answer{}, nil, fmt.Errorf("failed to get existing answer for question %s: %w", questionID, internal.ErrInternalServerError)
	}
	if err == nil {
		// Extract old file IDs from the stored answer for later cleanup
		var existingUploadAnswer shared.UploadFileAnswer
		if jsonErr := json.Unmarshal(existingAnswer.Value, &existingUploadAnswer); jsonErr == nil {
			for _, entry := range existingUploadAnswer.Files {
				oldFileIDs = append(oldFileIDs, entry.FileID.String())
			}
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
			s.logger.Error("failed to open uploaded file", zap.String("fileID", fh.Filename), zap.Error(err))
			span.RecordError(err)
			deleteFiles(fileIDs)
			return nil, Answer{}, nil, fmt.Errorf("failed to open uploaded file %q: %w", fh.Filename, internal.ErrFailedToSaveFile)
		}

		savedFile, saveErr := s.fileService.SaveFile(traceCtx, f, fh.Filename, fh.Header.Get("Content-Type"), uploadedBy)
		_ = f.Close()

		if saveErr != nil {
			s.logger.Error("failed to save uploaded file", zap.String("fileID", fh.Filename), zap.Error(saveErr))
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
		s.logger.Error("failed to marshal upload file answer value", zap.String("questionID", questionID.String()), zap.Error(err))
		span.RecordError(err)
		deleteFiles(fileIDs)
		return nil, Answer{}, nil, fmt.Errorf("failed to marshal upload file answer: %w", internal.ErrInternalServerError)
	}

	upsertedAnswers, answerableList, errs := s.Upsert(traceCtx, formID, responseID, []shared.AnswerParam{
		{QuestionID: questionID.String(), Value: answerValue},
	})
	if len(errs) > 0 {
		s.logger.Error("failed to upsert upload file answer", zap.String("questionID", questionID.String()), zap.Error(errs[0]))
		span.RecordError(errs[0])
		deleteFiles(fileIDs)
		return nil, Answer{}, nil, fmt.Errorf("failed to upsert upload file answer: %w", errs[0])
	}

	// Best-effort: delete old files now that the new answer is committed
	if len(oldFileIDs) > 0 {
		deleteFiles(oldFileIDs)
	}

	logger.Info("successfully uploaded files and upserted answer",
		zap.String("questionID", questionID.String()),
		zap.Int("fileCount", len(fileIDs)),
	)

	return entries, upsertedAnswers[0], answerableList[0], nil
}

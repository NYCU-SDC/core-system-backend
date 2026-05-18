package answer

import (
	"context"
	"encoding/json"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/file"
	"NYCU-SDC/core-system-backend/internal/form/shared"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type FileReferenceQuerier interface {
	Get(ctx context.Context, id uuid.UUID) (Answer, error)
	BatchUpsert(ctx context.Context, arg BatchUpsertParams) ([]Answer, error)
}

type FileResourceHandler struct {
	logger  *zap.Logger
	queries FileReferenceQuerier
	tracer  trace.Tracer
}

func NewFileResourceHandler(logger *zap.Logger, queries FileReferenceQuerier) *FileResourceHandler {
	return &FileResourceHandler{
		logger:  logger,
		queries: queries,
		tracer:  otel.Tracer("answer/file_resource_handler"),
	}
}

func (h *FileResourceHandler) ResourceType() file.ResourceType {
	return file.ResourceTypeFormAnswer
}

func (h *FileResourceHandler) RemoveFileReference(ctx context.Context, fileID uuid.UUID, resourceID uuid.UUID) error {
	traceCtx, span := h.tracer.Start(ctx, "RemoveFileReference")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	logger.Info("removing file reference from form_answer resource",
		zap.String("fileID", fileID.String()),
		zap.String("answerID", resourceID.String()),
	)

	answer, err := h.queries.Get(traceCtx, resourceID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get answer by id for file reference removal")
		span.RecordError(err)
		return err
	}

	var uploadAnswer shared.UploadFileAnswer
	err = json.Unmarshal(answer.Value, &uploadAnswer)
	if err != nil {
		logger.Error("failed to unmarshal upload_file answer during file reference removal",
			zap.String("answerID", resourceID.String()),
			zap.String("fileID", fileID.String()),
			zap.Error(err),
		)
		span.RecordError(err)
		return internal.ErrValidationFailed
	}

	filtered := make([]shared.UploadFileEntry, 0, len(uploadAnswer.Files))
	found := false

	for _, entry := range uploadAnswer.Files {
		if entry.FileID == fileID {
			found = true
			continue
		}
		filtered = append(filtered, entry)
	}

	// Idempotent remove: if already absent, treat as success.
	if !found {
		logger.Info("file reference already absent from upload_file answer",
			zap.String("answerID", resourceID.String()),
			zap.String("fileID", fileID.String()),
		)
		return nil
	}

	newValue, err := json.Marshal(shared.UploadFileAnswer{
		Files: filtered,
	})
	if err != nil {
		logger.Error("failed to marshal upload_file answer during file reference removal",
			zap.String("answerID", resourceID.String()),
			zap.String("fileID", fileID.String()),
			zap.Error(err),
		)
		span.RecordError(err)
		return err
	}

	_, err = h.queries.BatchUpsert(traceCtx, BatchUpsertParams{
		ResponseIds: []uuid.UUID{answer.ResponseID},
		QuestionIds: []uuid.UUID{answer.QuestionID},
		Values:      [][]byte{newValue},
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "batch upsert answer after file reference removal")
		span.RecordError(err)
		return err
	}

	logger.Info("removed file reference from upload_file answer",
		zap.String("answerID", resourceID.String()),
		zap.String("responseID", answer.ResponseID.String()),
		zap.String("questionID", answer.QuestionID.String()),
		zap.String("fileID", fileID.String()),
		zap.Int("remainingFileCount", len(filtered)),
	)

	return nil
}

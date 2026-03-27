package file

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Create(ctx context.Context, arg CreateParams) (File, error)
	GetByID(ctx context.Context, id uuid.UUID) (File, error)
	GetMetadataByID(ctx context.Context, id uuid.UUID) (GetMetadataByIDRow, error)
	GetByUploadedBy(ctx context.Context, uploadedBy pgtype.UUID) ([]GetByUploadedByRow, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
	LockFileByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	GetAll(ctx context.Context, arg GetAllParams) ([]GetAllRow, error)
	Count(ctx context.Context) (int64, error)
	CreateAttachment(ctx context.Context, arg CreateAttachmentParams) (FileAttachment, error)
	ListAttachmentsByFileID(ctx context.Context, fileID uuid.UUID) ([]FileAttachment, error)
	ListAttachmentsByResource(ctx context.Context, arg ListAttachmentsByResourceParams) ([]FileAttachment, error)
	ExistsAttachmentByFileAndResource(ctx context.Context, arg ExistsAttachmentByFileAndResourceParams) (bool, error)
	GetAttachmentByFileAndResource(ctx context.Context, arg GetAttachmentByFileAndResourceParams) (FileAttachment, error)
	GetAttachmentByID(ctx context.Context, id uuid.UUID) (FileAttachment, error)
	DeleteAttachmentByID(ctx context.Context, id uuid.UUID) error
	DeleteAttachmentsByFileID(ctx context.Context, fileID uuid.UUID) error
	WithTx(tx pgx.Tx) *Queries
}

type TxBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

type ResourceHandler interface {
	ResourceType() ResourceType
	RemoveFileReference(ctx context.Context, fileID uuid.UUID, resourceID uuid.UUID) error
}

type Service struct {
	logger           *zap.Logger
	db               DBTX
	queries          Querier
	tracer           trace.Tracer
	validator        *Validator
	resourceHandlers map[ResourceType]ResourceHandler
}

func NewService(logger *zap.Logger, db DBTX, handlers ...ResourceHandler) *Service {
	handlerMap := make(map[ResourceType]ResourceHandler, len(handlers))
	for _, h := range handlers {
		handlerMap[h.ResourceType()] = h
	}

	return &Service{
		logger:           logger,
		db:               db,
		queries:          New(db),
		tracer:           otel.Tracer("file/service"),
		validator:        NewValidator(),
		resourceHandlers: handlerMap,
	}
}

func (s *Service) WithTx(tx pgx.Tx) *Service {
	return &Service{
		logger:           s.logger,
		db:               tx,
		queries:          s.queries.WithTx(tx),
		tracer:           s.tracer,
		validator:        s.validator,
		resourceHandlers: s.resourceHandlers,
	}
}

// SaveFile saves the uploaded file data to database with validation
// uploadedBy can be nil for system uploads
// opts are validation options (e.g., WithWebP(), WithMaxSize(1024))
func (s *Service) SaveFile(ctx context.Context, fileContent io.Reader, originalFilename, contentType string, uploadedBy *uuid.UUID, opts ...ValidatorOption) (File, error) {
	traceCtx, span := s.tracer.Start(ctx, "SaveFile")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Validate and read file content using internal validator
	var data []byte
	var err error

	if len(opts) > 0 {
		// Validation is requested
		data, err = s.validator.ValidateStream(fileContent, contentType, opts...)
		if err != nil {
			logger.Warn("File validation failed", zap.Error(err))
			span.RecordError(err)
			return File{}, err
		}
	} else {
		// No validation, just read the stream
		data, err = io.ReadAll(fileContent)
		if err != nil {
			logger.Error("Failed to read file content", zap.Error(err))
			span.RecordError(err)
			return File{}, fmt.Errorf("failed to read file content: %w", err)
		}
	}

	size := int64(len(data))

	// Convert uploadedBy to pgtype.UUID
	var pgUploadedBy pgtype.UUID
	if uploadedBy != nil {
		pgUploadedBy = pgtype.UUID{
			Bytes: *uploadedBy,
			Valid: true,
		}
	}

	// Create database record with file data
	file, err := s.queries.Create(traceCtx, CreateParams{
		OriginalFilename: originalFilename,
		ContentType:      contentType,
		Size:             size,
		Data:             data,
		UploadedBy:       pgUploadedBy,
	})
	if err != nil {
		logger.Error("Failed to create file record", zap.Error(err))
		span.RecordError(err)
		return File{}, databaseutil.WrapDBError(err, logger, "create file record")
	}

	logger.Info("File saved successfully",
		zap.String("file_id", file.ID.String()),
		zap.String("original_filename", originalFilename),
		zap.Int64("size", size),
	)

	return file, nil
}

// CreateAttachment links an existing file to a resource
func (s *Service) CreateAttachment(ctx context.Context, fileID uuid.UUID, resourceType ResourceType, resourceID uuid.UUID, createdBy *uuid.UUID) (FileAttachment, error) {
	traceCtx, span := s.tracer.Start(ctx, "CreateAttachment")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	var (
		tx         pgx.Tx
		err        error
		needCommit bool
	)

	existingTx, ok := s.db.(pgx.Tx)
	if ok {
		tx = existingTx
	} else {
		beginner, ok := s.db.(TxBeginner)
		if !ok {
			return FileAttachment{}, fmt.Errorf("db does not support transactions")
		}

		tx, err = beginner.BeginTx(traceCtx, pgx.TxOptions{})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "begin tx for create attachment")
			span.RecordError(err)
			return FileAttachment{}, err
		}
		needCommit = true

		defer func() {
			err := tx.Rollback(traceCtx)
			if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				logger.Error("rollback failed", zap.Error(err))
			}
		}()
	}

	qtx := s.queries.WithTx(tx)

	_, err = qtx.LockFileByID(traceCtx, fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FileAttachment{}, internal.ErrFileNotFound
		}
		err = databaseutil.WrapDBError(err, logger, "lock file before create attachment")
		span.RecordError(err)
		return FileAttachment{}, err
	}

	pgCreatedBy := pgtype.UUID{Valid: false}
	if createdBy != nil {
		pgCreatedBy = pgtype.UUID{
			Bytes: *createdBy,
			Valid: true,
		}
	}

	attachment, err := qtx.CreateAttachment(traceCtx, CreateAttachmentParams{
		FileID:       fileID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		CreatedBy:    pgCreatedBy,
	})
	if err == nil {
		if needCommit {
			err := tx.Commit(traceCtx)
			if err != nil {
				err = databaseutil.WrapDBError(err, logger, "commit create attachment tx")
				span.RecordError(err)
				return FileAttachment{}, err
			}
		}

		logger.Info("file attachment created",
			zap.String("attachment_id", attachment.ID.String()),
			zap.String("file_id", attachment.FileID.String()),
			zap.String("resource_type", string(attachment.ResourceType)),
			zap.String("resource_id", attachment.ResourceID.String()),
		)
		return attachment, nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		existing, getErr := qtx.GetAttachmentByFileAndResource(traceCtx, GetAttachmentByFileAndResourceParams{
			FileID:       fileID,
			ResourceType: resourceType,
			ResourceID:   resourceID,
		})
		if getErr != nil {
			getErr = databaseutil.WrapDBError(getErr, logger, "get existing attachment after unique violation")
			span.RecordError(getErr)
			return FileAttachment{}, getErr
		}

		if needCommit {
			err := tx.Commit(traceCtx)
			if err != nil {
				err = databaseutil.WrapDBError(err, logger, "commit create attachment tx")
				span.RecordError(err)
				return FileAttachment{}, err
			}
		}

		logger.Info("file attachment already exists, returning existing record",
			zap.String("attachment_id", existing.ID.String()),
			zap.String("file_id", existing.FileID.String()),
			zap.String("resource_type", string(existing.ResourceType)),
			zap.String("resource_id", existing.ResourceID.String()),
		)
		return existing, nil
	}

	err = databaseutil.WrapDBError(err, logger, "create attachment")
	span.RecordError(err)
	return FileAttachment{}, err
}

// SaveFileForResource saves the file first, then creates the attachment
func (s *Service) SaveFileForResource(
	ctx context.Context,
	fileContent io.Reader,
	originalFilename, contentType string,
	uploadedBy *uuid.UUID,
	resourceType ResourceType,
	resourceID uuid.UUID,
	opts ...ValidatorOption,
) (File, FileAttachment, error) {
	traceCtx, span := s.tracer.Start(ctx, "SaveFileForResource")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	savedFile, err := s.SaveFile(traceCtx, fileContent, originalFilename, contentType, uploadedBy, opts...)
	if err != nil {
		span.RecordError(err)
		return File{}, FileAttachment{}, err
	}

	attachment, err := s.CreateAttachment(traceCtx, savedFile.ID, resourceType, resourceID, uploadedBy)
	if err != nil {
		span.RecordError(err)
		logger.Warn("failed to create attachment after saving file; cleaning up orphan file",
			zap.String("file_id", savedFile.ID.String()),
			zap.Error(err),
		)

		if delErr := s.DeletePhysicalFile(traceCtx, savedFile.ID); delErr != nil {
			logger.Warn("failed to clean up orphan file after attachment creation failure",
				zap.String("file_id", savedFile.ID.String()),
				zap.Error(delErr),
			)
		}

		return File{}, FileAttachment{}, err
	}

	return savedFile, attachment, nil
}

func (s *Service) GetAttachmentByID(ctx context.Context, attachmentID uuid.UUID) (FileAttachment, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetAttachmentByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	attachment, err := s.queries.GetAttachmentByID(traceCtx, attachmentID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get attachment by id")
		span.RecordError(err)
		return FileAttachment{}, err
	}

	return attachment, nil
}

func (s *Service) ListAttachmentsByFileID(ctx context.Context, fileID uuid.UUID) ([]FileAttachment, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListAttachmentsByFileID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	attachments, err := s.queries.ListAttachmentsByFileID(traceCtx, fileID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list attachments by file id")
		span.RecordError(err)
		return nil, err
	}

	return attachments, nil
}

func (s *Service) ListAttachmentsByResource(ctx context.Context, resourceType ResourceType, resourceID uuid.UUID) ([]FileAttachment, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListAttachmentsByResource")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	attachments, err := s.queries.ListAttachmentsByResource(traceCtx, ListAttachmentsByResourceParams{
		ResourceType: resourceType,
		ResourceID:   resourceID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list attachments by resource")
		span.RecordError(err)
		return nil, err
	}

	return attachments, nil
}

// DeleteAttachmentByID only removes the link, not the file itself.
func (s *Service) DeleteAttachmentByID(ctx context.Context, attachmentID uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "DeleteAttachmentByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	err := s.queries.DeleteAttachmentByID(traceCtx, attachmentID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "delete attachment by id")
		span.RecordError(err)
		return err
	}

	logger.Info("file attachment deleted",
		zap.String("attachment_id", attachmentID.String()),
	)

	return nil
}

// DeletePhysicalFile unconditionally deletes the file row.
func (s *Service) DeletePhysicalFile(ctx context.Context, fileID uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "DeletePhysicalFile")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	err := s.queries.Delete(traceCtx, fileID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "delete physical file")
		span.RecordError(err)
		return err
	}

	logger.Info("physical file deleted successfully",
		zap.String("file_id", fileID.String()),
	)

	return nil
}

// Delete is the generic delete orchestration entrypoint.
// It asks the corresponding resource handler to remove the reference from each attached resource,
// then deletes the attachment row, and finally deletes the physical file.
func (s *Service) Delete(ctx context.Context, fileID uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "DeleteFile")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	var (
		tx         pgx.Tx
		err        error
		needCommit bool
	)

	existingTx, ok := s.db.(pgx.Tx)
	if ok {
		tx = existingTx
	} else {
		beginner, ok := s.db.(TxBeginner)
		if !ok {
			return fmt.Errorf("db does not support transactions")
		}
		tx, err = beginner.BeginTx(traceCtx, pgx.TxOptions{})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "begin tx")
			span.RecordError(err)
			return err
		}
		needCommit = true

		defer func() {
			err := tx.Rollback(traceCtx)
			if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				logger.Error("rollback failed", zap.Error(err))
			}
		}()
	}

	qtx := s.queries.WithTx(tx)

	_, err = qtx.LockFileByID(traceCtx, fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return internal.ErrFileNotFound
		}
		err = databaseutil.WrapDBError(err, logger, "lock file before delete")
		span.RecordError(err)
		return err
	}

	attachments, err := qtx.ListAttachmentsByFileID(traceCtx, fileID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list attachments before delete file")
		span.RecordError(err)
		return err
	}

	for _, att := range attachments {
		handler, ok := s.resourceHandlers[att.ResourceType]
		if !ok {
			err := fmt.Errorf("no resource handler registered for resource type %s", att.ResourceType)
			span.RecordError(err)
			return err
		}

		if err := handler.RemoveFileReference(traceCtx, fileID, att.ResourceID); err != nil {
			err = fmt.Errorf(
				"remove file reference from resource type %s resource id %s: %w",
				att.ResourceType,
				att.ResourceID.String(),
				err,
			)
			span.RecordError(err)
			return err
		}

		if err := qtx.DeleteAttachmentByID(traceCtx, att.ID); err != nil {
			err = databaseutil.WrapDBError(err, logger, "delete attachment after removing resource reference")
			span.RecordError(err)
			return err
		}
	}

	err = qtx.Delete(traceCtx, fileID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "delete file row")
		span.RecordError(err)
		return err
	}

	if needCommit {
		err := tx.Commit(traceCtx)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "commit tx")
			span.RecordError(err)
			return err
		}
	}

	logger.Info("file deleted successfully through orchestration",
		zap.String("file_id", fileID.String()),
		zap.Int("attachment_count", len(attachments)),
	)

	return nil
}

// GetByID retrieves a file record with data by ID
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (File, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	file, err := s.queries.GetByID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get file by id")
		span.RecordError(err)
		return File{}, err
	}

	return file, nil
}

// GetMetadataByID retrieves file metadata without the binary data
func (s *Service) GetMetadataByID(ctx context.Context, id uuid.UUID) (GetMetadataByIDRow, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetMetadataByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	metadata, err := s.queries.GetMetadataByID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get file metadata by id")
		span.RecordError(err)
		return GetMetadataByIDRow{}, err
	}

	return metadata, nil
}

// GetByUploadedBy retrieves all files (metadata only) uploaded by a specific user
func (s *Service) GetByUploadedBy(ctx context.Context, userID uuid.UUID) ([]GetByUploadedByRow, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetByUploadedBy")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	pgUserID := pgtype.UUID{
		Bytes: [16]byte(userID),
		Valid: true,
	}

	files, err := s.queries.GetByUploadedBy(traceCtx, pgUserID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get files by uploaded by")
		span.RecordError(err)
		return nil, err
	}

	return files, nil
}

// GetAll retrieves all files (metadata only) with pagination
func (s *Service) GetAll(ctx context.Context, limit, offset int32) ([]GetAllRow, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetAll")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	files, err := s.queries.GetAll(traceCtx, GetAllParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get all files")
		span.RecordError(err)
		return nil, err
	}

	return files, nil
}

// Count returns the total number of files
func (s *Service) Count(ctx context.Context) (int64, error) {
	traceCtx, span := s.tracer.Start(ctx, "Count")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	count, err := s.queries.Count(traceCtx)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "count files")
		span.RecordError(err)
		return 0, err
	}

	return count, nil
}

// DownloadFromURL downloads a file from the given URL and saves it to the database
// This method exposes the server IP and should ONLY be used for trusted sources
// (e.g., downloading Google Profile Avatar)
// url is the source URL to download from
// filename is the desired filename; if empty, it will be extracted from the URL
// uploadedBy can be nil for system uploads
// opts are validation options (e.g., WithWebP(), WithMaxSize(1024))
func (s *Service) DownloadFromURL(ctx context.Context, url string, filename string, uploadedBy *uuid.UUID, opts ...ValidatorOption) (File, error) {
	traceCtx, span := s.tracer.Start(ctx, "DownloadFromURL")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	logger.Info("Downloading file from URL",
		zap.String("url", url),
	)

	// Download the file
	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Failed to download file from URL",
			zap.String("url", url),
			zap.Error(err))
		span.RecordError(err)
		return File{}, fmt.Errorf("failed to download file from URL: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("bad status code: %d", resp.StatusCode)
		logger.Error("Failed to download file: bad status",
			zap.String("url", url),
			zap.Int("status", resp.StatusCode))
		span.RecordError(err)
		return File{}, err
	}

	// Determine content type from response header
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream" // default
	}

	// Use provided filename or extract from URL path
	if filename == "" {
		filename = path.Base(url)
		if filename == "/" || filename == "." {
			filename = "downloaded_file"
		}
	}

	// Save the downloaded file using existing SaveFile method
	return s.SaveFile(traceCtx, resp.Body, filename, contentType, uploadedBy, opts...)
}

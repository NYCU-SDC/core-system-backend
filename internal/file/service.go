package file

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"fmt"
	"io"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// FileValidator defines validation rules for file uploads
type FileValidator struct {
	MaxSize      int64              // Maximum file size in bytes
	AllowedTypes []string           // Allowed MIME types (empty means all types allowed)
	MagicBytes   map[string][]byte  // Magic bytes to validate file format (key: format name, value: magic bytes)
	CustomCheck  func([]byte) error // Custom validation function
}

// ValidateFile validates file data against the validator rules
func (v *FileValidator) ValidateFile(data []byte, contentType string) error {
	// Check size
	if v.MaxSize > 0 && int64(len(data)) > v.MaxSize {
		return internal.ErrFileTooLarge
	}

	// Check content type if specified
	if len(v.AllowedTypes) > 0 {
		allowed := false
		for _, t := range v.AllowedTypes {
			if t == contentType {
				allowed = true
				break
			}
		}
		if !allowed {
			return internal.ErrInvalidFileType
		}
	}

	// Check magic bytes if specified
	if len(v.MagicBytes) > 0 {
		valid := false
		for _, magic := range v.MagicBytes {
			if len(data) >= len(magic) {
				match := true
				for i, b := range magic {
					if data[i] != b {
						match = false
						break
					}
				}
				if match {
					valid = true
					break
				}
			}
		}
		if !valid {
			return internal.ErrInvalidImageFormat
		}
	}

	// Custom validation
	if v.CustomCheck != nil {
		if err := v.CustomCheck(data); err != nil {
			return err
		}
	}

	return nil
}

// WebPValidator returns a validator for WebP images
func WebPValidator(maxSize int64) *FileValidator {
	return &FileValidator{
		MaxSize:      maxSize,
		AllowedTypes: []string{"image/webp"},
		CustomCheck: func(data []byte) error {
			// WebP validation: RIFF header + WEBP signature
			if len(data) < 12 ||
				string(data[0:4]) != "RIFF" ||
				string(data[8:12]) != "WEBP" {
				return internal.ErrInvalidImageFormat
			}
			return nil
		},
	}
}

type Querier interface {
	Create(ctx context.Context, arg CreateParams) (File, error)
	GetByID(ctx context.Context, id uuid.UUID) (File, error)
	GetMetadataByID(ctx context.Context, id uuid.UUID) (GetMetadataByIDRow, error)
	GetByUploadedBy(ctx context.Context, uploadedBy pgtype.UUID) ([]GetByUploadedByRow, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
	GetAll(ctx context.Context, arg GetAllParams) ([]GetAllRow, error)
	Count(ctx context.Context) (int64, error)
}

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer
}

func NewService(logger *zap.Logger, db DBTX) *Service {
	return &Service{
		logger:  logger,
		queries: New(db),
		tracer:  otel.Tracer("file/service"),
	}
}

// SaveFileWithValidation saves a file with custom validation rules
// validator can be nil to skip validation
func (s *Service) SaveFileWithValidation(ctx context.Context, fileContent io.Reader, originalFilename, contentType string, size int64, uploadedBy *uuid.UUID, validator *FileValidator) (File, error) {
	traceCtx, span := s.tracer.Start(ctx, "SaveFileWithValidation")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Read file content into memory
	data, err := io.ReadAll(fileContent)
	if err != nil {
		logger.Error("Failed to read file content", zap.Error(err))
		span.RecordError(err)
		return File{}, fmt.Errorf("failed to read file content: %w", err)
	}

	// Validate file if validator is provided
	if validator != nil {
		if err := validator.ValidateFile(data, contentType); err != nil {
			logger.Warn("File validation failed", zap.Error(err))
			span.RecordError(err)
			return File{}, err
		}
	}

	// Verify size
	actualSize := int64(len(data))
	if actualSize != size {
		logger.Warn("File size mismatch", zap.Int64("expected", size), zap.Int64("actual", actualSize))
		size = actualSize // Use actual size
	}

	// Convert uploadedBy to pgtype.UUID
	var pgUploadedBy pgtype.UUID
	if uploadedBy != nil {
		pgUploadedBy = pgtype.UUID{
			Bytes: [16]byte(*uploadedBy),
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

	logger.Info("File saved successfully with validation",
		zap.String("file_id", file.ID.String()),
		zap.String("original_filename", originalFilename),
		zap.Int64("size", size),
	)

	return file, nil
}

// SaveFile saves the uploaded file data to database
// uploadedBy can be nil for system uploads
func (s *Service) SaveFile(ctx context.Context, fileContent io.Reader, originalFilename, contentType string, size int64, uploadedBy *uuid.UUID) (File, error) {
	traceCtx, span := s.tracer.Start(ctx, "SaveFile")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Read file content into memory
	data, err := io.ReadAll(fileContent)
	if err != nil {
		logger.Error("Failed to read file content", zap.Error(err))
		span.RecordError(err)
		return File{}, fmt.Errorf("failed to read file content: %w", err)
	}

	// Verify size
	actualSize := int64(len(data))
	if actualSize != size {
		logger.Warn("File size mismatch", zap.Int64("expected", size), zap.Int64("actual", actualSize))
		size = actualSize // Use actual size
	}

	// Convert uploadedBy to pgtype.UUID
	var pgUploadedBy pgtype.UUID
	if uploadedBy != nil {
		pgUploadedBy = pgtype.UUID{
			Bytes: [16]byte(*uploadedBy),
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

// Delete removes a file from database
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Delete from database
	if err := s.queries.Delete(traceCtx, id); err != nil {
		err = databaseutil.WrapDBError(err, logger, "delete file from database")
		span.RecordError(err)
		return err
	}

	logger.Info("File deleted successfully", zap.String("file_id", id.String()))

	return nil
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

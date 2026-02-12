package file

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/user"
	"bytes"
	"net/http"
	"strconv"
	"time"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	service       *Service
	tracer        trace.Tracer
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	service *Service,
) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		service:       service,
		tracer:        otel.Tracer("file/handler"),
	}
}

type Response struct {
	ID               string  `json:"id"`
	OriginalFilename string  `json:"originalFilename"`
	ContentType      string  `json:"contentType"`
	Size             int64   `json:"size"`
	UploadedBy       *string `json:"uploadedBy,omitempty"`
	CreatedAt        string  `json:"createdAt"`
}

type ListFilesResponse struct {
	Files  []Response `json:"files"`
	Total  int64      `json:"total"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}

// Download handles GET /files/{id} - downloads a file
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Download")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Get file ID from path
	fileIDStr := r.PathValue("id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidFileID, logger)
		return
	}

	// Get file with data from database
	fileInfo, err := h.service.GetByID(traceCtx, fileID)
	if err != nil {
		logger.Warn("Failed to get file", zap.Error(err), zap.String("file_id", fileIDStr))
		h.problemWriter.WriteError(traceCtx, w, internal.ErrFileNotFound, logger)
		span.RecordError(err)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", fileInfo.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+fileInfo.OriginalFilename+"\"")
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size, 10))

	// Stream file data from database to response
	reader := bytes.NewReader(fileInfo.Data)
	http.ServeContent(w, r, fileInfo.OriginalFilename, fileInfo.CreatedAt.Time, reader)
}

// GetByID handles GET /files/{id}/info - gets file info (without binary data)
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Get file ID from path
	fileIDStr := r.PathValue("id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidFileID, logger)
		return
	}

	// Get file metadata (without binary data)
	fileInfo, err := h.service.GetMetadataByID(traceCtx, fileID)
	if err != nil {
		logger.Warn("Failed to get file", zap.Error(err), zap.String("file_id", fileIDStr))
		h.problemWriter.WriteError(traceCtx, w, internal.ErrFileNotFound, logger)
		span.RecordError(err)
		return
	}

	var uploadedByStr *string
	if fileInfo.UploadedBy.Valid {
		uid := uuid.UUID(fileInfo.UploadedBy.Bytes)
		str := uid.String()
		uploadedByStr = &str
	}

	response := Response{
		ID:               fileInfo.ID.String(),
		OriginalFilename: fileInfo.OriginalFilename,
		ContentType:      fileInfo.ContentType,
		Size:             fileInfo.Size,
		UploadedBy:       uploadedByStr,
		CreatedAt:        fileInfo.CreatedAt.Time.Format(time.RFC3339),
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

// Delete handles DELETE /files/{id} - deletes a file
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Delete")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Get file ID from path
	fileIDStr := r.PathValue("id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidFileID, logger)
		return
	}

	// Delete file from database
	if err := h.service.Delete(traceCtx, fileID); err != nil {
		logger.Error("Failed to delete file", zap.Error(err), zap.String("file_id", fileIDStr))
		h.problemWriter.WriteError(traceCtx, w, internal.ErrFailedToDeleteFile, logger)
		span.RecordError(err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List handles GET /files - lists all files with pagination
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "List")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := int32(20) // default
	offset := int32(0) // default

	if limitStr != "" {
		parsedLimit, err := strconv.ParseInt(limitStr, 10, 32)
		if err != nil || parsedLimit <= 0 {
			h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidLimit, logger)
			return
		}
		limit = int32(parsedLimit)
	}

	if offsetStr != "" {
		parsedOffset, err := strconv.ParseInt(offsetStr, 10, 32)
		if err != nil || parsedOffset < 0 {
			h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidOffset, logger)
			return
		}
		offset = int32(parsedOffset)
	}

	// Get files (metadata only, without binary data)
	files, err := h.service.GetAll(traceCtx, limit, offset)
	if err != nil {
		logger.Error("Failed to get files", zap.Error(err))
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		span.RecordError(err)
		return
	}

	// Get total count
	total, err := h.service.Count(traceCtx)
	if err != nil {
		logger.Error("Failed to count files", zap.Error(err))
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		span.RecordError(err)
		return
	}

	// Build response
	fileResponses := make([]Response, len(files))
	for i, f := range files {
		var uploadedByStr *string
		if f.UploadedBy.Valid {
			uid := uuid.UUID(f.UploadedBy.Bytes)
			str := uid.String()
			uploadedByStr = &str
		}

		fileResponses[i] = Response{
			ID:               f.ID.String(),
			OriginalFilename: f.OriginalFilename,
			ContentType:      f.ContentType,
			Size:             f.Size,
			UploadedBy:       uploadedByStr,
			CreatedAt:        f.CreatedAt.Time.Format(time.RFC3339),
		}
	}

	response := ListFilesResponse{
		Files:  fileResponses,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

// ListMyFiles handles GET /files/me - lists files uploaded by the current user
func (h *Handler) ListMyFiles(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListMyFiles")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Get current user
	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	// Get files uploaded by user (metadata only, without binary data)
	files, err := h.service.GetByUploadedBy(traceCtx, currentUser.ID)
	if err != nil {
		logger.Error("Failed to get user files", zap.Error(err))
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		span.RecordError(err)
		return
	}

	// Build response
	fileResponses := make([]Response, len(files))
	for i, f := range files {
		var uploadedByStr *string
		if f.UploadedBy.Valid {
			uid := uuid.UUID(f.UploadedBy.Bytes)
			str := uid.String()
			uploadedByStr = &str
		}

		fileResponses[i] = Response{
			ID:               f.ID.String(),
			OriginalFilename: f.OriginalFilename,
			ContentType:      f.ContentType,
			Size:             f.Size,
			UploadedBy:       uploadedByStr,
			CreatedAt:        f.CreatedAt.Time.Format(time.RFC3339),
		}
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, fileResponses)
}

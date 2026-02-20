package question

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
)

// FileType represents allowed file extensions
type FileType string

const (
	// Documents
	FileTypeTxt  FileType = "txt"
	FileTypeMd   FileType = "md"
	FileTypeDoc  FileType = "doc"
	FileTypeDocx FileType = "docx"
	FileTypeOdt  FileType = "odt"
	FileTypeRtf  FileType = "rtf"

	// Presentations
	FileTypePpt  FileType = "ppt"
	FileTypePptx FileType = "pptx"
	FileTypeOdp  FileType = "odp"

	// Spreadsheets
	FileTypeXls  FileType = "xls"
	FileTypeXlsx FileType = "xlsx"
	FileTypeOds  FileType = "ods"
	FileTypeCsv  FileType = "csv"

	// Vector Graphics
	FileTypeSvg FileType = "svg"
	FileTypeAi  FileType = "ai"
	FileTypeEps FileType = "eps"

	// PDF
	FileTypePdf FileType = "pdf"

	// Images
	FileTypeJpg  FileType = "jpg"
	FileTypeJpeg FileType = "jpeg"
	FileTypePng  FileType = "png"
	FileTypeWebp FileType = "webp"
	FileTypeGif  FileType = "gif"
	FileTypeTiff FileType = "tiff"
	FileTypeBmp  FileType = "bmp"
	FileTypeHeic FileType = "heic"
	FileTypeRaw  FileType = "raw"

	// Videos
	FileTypeMp4  FileType = "mp4"
	FileTypeWebm FileType = "webm"
	FileTypeMov  FileType = "mov"
	FileTypeMkv  FileType = "mkv"
	FileTypeAvi  FileType = "avi"

	// Audio
	FileTypeMp3  FileType = "mp3"
	FileTypeWav  FileType = "wav"
	FileTypeM4a  FileType = "m4a"
	FileTypeAac  FileType = "aac"
	FileTypeOgg  FileType = "ogg"
	FileTypeFlac FileType = "flac"

	// Archive
	FileTypeZip FileType = "zip"
)

// UploadFileOption represents the request from frontend
type UploadFileOption struct {
	AllowedFileTypes []string `json:"allowedFileTypes" validate:"required"`
	MaxFileAmount    int32    `json:"maxFileAmount" validate:"required,min=1,max=10"`
	MaxFileSizeLimit int64    `json:"maxFileSizeLimit" validate:"required,min=1,max=10485760"`
}

// UploadFileMetadata represents the metadata stored in DB
type UploadFileMetadata struct {
	AllowedFileTypes []FileType `json:"allowedFileTypes"`
	MaxFileAmount    int32      `json:"maxFileAmount"`
	MaxFileSizeLimit int64      `json:"maxFileSizeLimit"`
}

type UploadFile struct {
	question         Question
	formID           uuid.UUID
	AllowedFileTypes []FileType
	MaxFileAmount    int32
	MaxFileSizeLimit int64
}

func (u UploadFile) Question() Question {
	return u.question
}

func (u UploadFile) FormID() uuid.UUID {
	return u.formID
}

func (u UploadFile) Validate(rawValue json.RawMessage) error {
	var answer shared.UploadFileAnswer
	if err := json.Unmarshal(rawValue, &answer); err != nil {
		return fmt.Errorf("invalid file upload value format: %w", err)
	}

	if int32(len(answer.Files)) > u.MaxFileAmount {
		return fmt.Errorf("too many files: %d (max: %d)", len(answer.Files), u.MaxFileAmount)
	}

	return nil
}

func NewUploadFile(q Question, formID uuid.UUID) (UploadFile, error) {
	if q.Metadata == nil {
		return UploadFile{}, errors.New("metadata is nil")
	}

	uploadFile, err := ExtractUploadFile(q.Metadata)
	if err != nil {
		return UploadFile{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    q.Metadata,
			Message:    "could not extract upload file from metadata",
		}
	}

	// Validate metadata
	if len(uploadFile.AllowedFileTypes) == 0 {
		return UploadFile{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    q.Metadata,
			Message:    "allowedFileTypes cannot be empty",
		}
	}

	if uploadFile.MaxFileAmount < 1 || uploadFile.MaxFileAmount > 10 {
		return UploadFile{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    q.Metadata,
			Message:    fmt.Sprintf("maxFileAmount must be between 1 and 10, got: %d", uploadFile.MaxFileAmount),
		}
	}

	// Validate file size limit (1 byte to 10 MB)
	const maxFileSizeBytes int64 = 10485760 // 10 MB in bytes
	if uploadFile.MaxFileSizeLimit < 1 || uploadFile.MaxFileSizeLimit > maxFileSizeBytes {
		return UploadFile{}, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    q.Metadata,
			Message:    fmt.Sprintf("maxFileSizeLimit must be between 1 and %d bytes (10 MB), got: %d", maxFileSizeBytes, uploadFile.MaxFileSizeLimit),
		}
	}

	return UploadFile{
		question:         q,
		formID:           formID,
		AllowedFileTypes: uploadFile.AllowedFileTypes,
		MaxFileAmount:    uploadFile.MaxFileAmount,
		MaxFileSizeLimit: uploadFile.MaxFileSizeLimit,
	}, nil
}

func (u UploadFile) DecodeRequest(rawValue json.RawMessage) (any, error) {
	// Request format (from UploadFiles): {"files": [{"fileId": "...", "originalFilename": "...", "contentType": "...", "size": 0}]}
	var answer shared.UploadFileAnswer
	if err := json.Unmarshal(rawValue, &answer); err != nil {
		return nil, fmt.Errorf("invalid upload file value format: %w", err)
	}

	return answer, nil
}

func (u UploadFile) DecodeStorage(rawValue json.RawMessage) (any, error) {
	// Storage format: {"files": [{"fileId": "...", "originalFilename": "...", "contentType": "...", "size": 0}]}
	var answer shared.UploadFileAnswer
	if err := json.Unmarshal(rawValue, &answer); err != nil {
		return nil, fmt.Errorf("invalid upload file answer in storage: %w", err)
	}

	return answer, nil
}

func (u UploadFile) EncodeRequest(answer any) (json.RawMessage, error) {
	uploadFileAnswer, ok := answer.(shared.UploadFileAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.UploadFileAnswer, got %T", answer)
	}

	// API response returns the full files array with metadata
	return json.Marshal(uploadFileAnswer.Files)
}

func (u UploadFile) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := u.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	uploadFileAnswer, ok := answer.(shared.UploadFileAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.UploadFileAnswer, got %T", answer)
	}

	count := len(uploadFileAnswer.Files)
	if count == 0 {
		return "0 files", nil
	}

	parts := make([]string, 0, count)
	for _, f := range uploadFileAnswer.Files {
		parts = append(parts, fmt.Sprintf("%s (%d bytes)", f.OriginalFilename, f.Size))
	}
	return fmt.Sprintf("%d file(s): %s", count, strings.Join(parts, ", ")), nil
}

func (u UploadFile) MatchesPattern(rawValue json.RawMessage, pattern string) (bool, error) {
	return false, errors.New("MatchesPattern is not supported for upload_file question type")
}

// GenerateUploadFileMetadata generates metadata for upload file question
func GenerateUploadFileMetadata(option UploadFileOption) ([]byte, error) {
	if len(option.AllowedFileTypes) == 0 {
		return nil, errors.New("allowedFileTypes cannot be empty")
	}

	if option.MaxFileAmount < 1 || option.MaxFileAmount > 10 {
		return nil, fmt.Errorf("maxFileAmount must be between 1 and 10, got: %d", option.MaxFileAmount)
	}

	// Validate and convert file types
	fileTypes := make([]FileType, len(option.AllowedFileTypes))
	for i, ft := range option.AllowedFileTypes {
		fileType := FileType(strings.ToLower(strings.TrimSpace(ft)))
		if !isValidFileType(fileType) {
			return nil, fmt.Errorf("invalid file type: %s", ft)
		}
		fileTypes[i] = fileType
	}

	// Validate file size limit (1 byte to 10 MB)
	const maxFileSizeBytes int64 = 10485760 // 10 MB in bytes
	if option.MaxFileSizeLimit < 1 || option.MaxFileSizeLimit > maxFileSizeBytes {
		return nil, fmt.Errorf("file size limit must be between 1 and %d bytes (10 MB), got: %d", maxFileSizeBytes, option.MaxFileSizeLimit)
	}

	metadata := map[string]any{
		"uploadFile": UploadFileMetadata{
			AllowedFileTypes: fileTypes,
			MaxFileAmount:    option.MaxFileAmount,
			MaxFileSizeLimit: option.MaxFileSizeLimit,
		},
	}

	return json.Marshal(metadata)
}

// isValidFileType checks if the file type is allowed
func isValidFileType(ft FileType) bool {
	validTypes := []FileType{
		// Documents
		FileTypeTxt, FileTypeMd, FileTypeDoc, FileTypeDocx, FileTypeOdt, FileTypeRtf,
		// Presentations
		FileTypePpt, FileTypePptx, FileTypeOdp,
		// Spreadsheets
		FileTypeXls, FileTypeXlsx, FileTypeOds, FileTypeCsv,
		// Vector Graphics
		FileTypeSvg, FileTypeAi, FileTypeEps,
		// PDF
		FileTypePdf,
		// Images
		FileTypeJpg, FileTypeJpeg, FileTypePng, FileTypeWebp, FileTypeGif,
		FileTypeTiff, FileTypeBmp, FileTypeHeic, FileTypeRaw,
		// Videos
		FileTypeMp4, FileTypeWebm, FileTypeMov, FileTypeMkv, FileTypeAvi,
		// Audio
		FileTypeMp3, FileTypeWav, FileTypeM4a, FileTypeAac, FileTypeOgg, FileTypeFlac,
		// Archive
		FileTypeZip,
	}

	for _, valid := range validTypes {
		if ft == valid {
			return true
		}
	}
	return false
}

func ExtractUploadFile(data []byte) (UploadFileMetadata, error) {
	var partial map[string]json.RawMessage
	if err := json.Unmarshal(data, &partial); err != nil {
		return UploadFileMetadata{}, fmt.Errorf("could not parse partial json: %w", err)
	}

	var metadata UploadFileMetadata
	if raw, ok := partial["uploadFile"]; ok {
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return UploadFileMetadata{}, fmt.Errorf("could not parse upload file: %w", err)
		}
	}
	return metadata, nil
}

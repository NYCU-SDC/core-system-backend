package file

import (
	"NYCU-SDC/core-system-backend/internal"
	"fmt"
	"io"
)

// ValidatorOption is a function that configures validation rules
type ValidatorOption func(*validatorConfig)

// validatorConfig holds the validation configuration
type validatorConfig struct {
	maxSize      int64
	allowedTypes []string
	checkFormat  func([]byte) error
}

// Validator performs file validation based on configured rules
type Validator struct {
	// Internal validator state, encapsulated from external callers
}

// NewValidator creates a new validator instance
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateStream validates a file stream and returns the validated data
// It applies the provided validation options internally
func (v *Validator) ValidateStream(stream io.Reader, contentType string, opts ...ValidatorOption) ([]byte, error) {
	// Apply options to build configuration
	config := &validatorConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Read stream into memory
	data, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("failed to read file stream: %w", err)
	}

	// Validate size
	if config.maxSize > 0 && int64(len(data)) > config.maxSize {
		return nil, internal.ErrFileTooLarge
	}

	// Validate content type
	if len(config.allowedTypes) > 0 {
		allowed := false
		for _, t := range config.allowedTypes {
			if t == contentType {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, internal.ErrInvalidFileType
		}
	}

	// Validate file format
	if config.checkFormat != nil {
		if err := config.checkFormat(data); err != nil {
			return nil, err
		}
	}

	return data, nil
}

// WithMaxSize sets the maximum allowed file size in bytes
func WithMaxSize(size int64) ValidatorOption {
	return func(c *validatorConfig) {
		c.maxSize = size
	}
}

// WithWebP configures validation for WebP images
func WithWebP() ValidatorOption {
	return func(c *validatorConfig) {
		c.allowedTypes = []string{"image/webp"}
		c.checkFormat = func(data []byte) error {
			// WebP validation: RIFF header + WEBP signature
			if len(data) < 12 ||
				string(data[0:4]) != "RIFF" ||
				string(data[8:12]) != "WEBP" {
				return internal.ErrInvalidImageFormat
			}
			return nil
		}
	}
}

// WithJPEG configures validation for JPEG images
func WithJPEG() ValidatorOption {
	return func(c *validatorConfig) {
		c.allowedTypes = []string{"image/jpeg"}
		c.checkFormat = func(data []byte) error {
			// JPEG validation: check magic bytes FF D8 FF
			if len(data) < 3 || data[0] != 0xFF || data[1] != 0xD8 || data[2] != 0xFF {
				return internal.ErrInvalidImageFormat
			}
			return nil
		}
	}
}

// WithPNG configures validation for PNG images
func WithPNG() ValidatorOption {
	return func(c *validatorConfig) {
		c.allowedTypes = []string{"image/png"}
		c.checkFormat = func(data []byte) error {
			// PNG validation: check PNG signature
			pngSignature := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
			if len(data) < len(pngSignature) {
				return internal.ErrInvalidImageFormat
			}
			for i, b := range pngSignature {
				if data[i] != b {
					return internal.ErrInvalidImageFormat
				}
			}
			return nil
		}
	}
}

// WithContentType sets allowed MIME types without format validation
func WithContentType(contentTypes ...string) ValidatorOption {
	return func(c *validatorConfig) {
		c.allowedTypes = contentTypes
	}
}

// WithCustomValidation allows custom validation logic
func WithCustomValidation(check func([]byte) error) ValidatorOption {
	return func(c *validatorConfig) {
		c.checkFormat = check
	}
}

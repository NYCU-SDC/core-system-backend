package question

import (
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestUploadFile_MatchesPattern(t *testing.T) {
	u := newTestUploadFile(t)

	testCases := []struct {
		name          string
		rawBytes      json.RawMessage
		pattern       string
		expected      bool
		expectedError bool
	}{
		{
			name:     "Should match zero files display",
			rawBytes: mustMarshalUploadAnswer(t, shared.UploadFileAnswer{Files: nil}),
			pattern:  `^0 files$`,
			expected: true,
		},
		{
			name: "Should match filename substring in display value",
			rawBytes: mustMarshalUploadAnswer(t, shared.UploadFileAnswer{
				Files: []shared.UploadFileEntry{
					{
						FileID:           uuid.New(),
						OriginalFilename: "report.pdf",
						ContentType:      "application/pdf",
						Size:             1234,
					},
				},
			}),
			pattern:  `report\.pdf`,
			expected: true,
		},
		{
			name: "Should not match when pattern misses display value",
			rawBytes: mustMarshalUploadAnswer(t, shared.UploadFileAnswer{
				Files: []shared.UploadFileEntry{
					{
						FileID:           uuid.New(),
						OriginalFilename: "report.pdf",
						ContentType:      "application/pdf",
						Size:             1234,
					},
				},
			}),
			pattern:  `^report\.pdf$`,
			expected: false,
		},
		{
			name:          "Should error on invalid JSON",
			rawBytes:      json.RawMessage(`not-json`),
			pattern:       `^0 files$`,
			expectedError: true,
		},
		{
			name: "Should error on invalid regex",
			rawBytes: mustMarshalUploadAnswer(t, shared.UploadFileAnswer{
				Files: []shared.UploadFileEntry{
					{
						FileID:           uuid.New(),
						OriginalFilename: "report.pdf",
						ContentType:      "application/pdf",
						Size:             1234,
					},
				},
			}),
			pattern:       "(",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			match, err := u.MatchesPattern(tc.rawBytes, tc.pattern)

			if tc.expectedError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, match)
		})
	}
}

func mustMarshalUploadAnswer(t *testing.T, answer shared.UploadFileAnswer) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(answer)
	if err != nil {
		t.Fatalf("Failed to marshal answer: %v", err)
	}
	return raw
}

func newTestUploadFile(t *testing.T) UploadFile {
	t.Helper()

	metadata, err := GenerateUploadFileMetadata(UploadFileOption{
		AllowedFileTypes: []string{"pdf"},
		MaxFileAmount:    3,
		MaxFileSizeLimit: 1048576,
	})
	if err != nil {
		t.Fatalf("Failed to generate upload file metadata: %v", err)
	}

	u, err := NewUploadFile(Question{
		ID:       uuid.New(),
		Metadata: metadata,
	}, uuid.New())
	if err != nil {
		t.Fatalf("Failed to create UploadFile: %v", err)
	}
	return u
}

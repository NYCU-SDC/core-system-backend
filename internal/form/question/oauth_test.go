package question

import (
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestOAuthConnect_MatchesPattern(t *testing.T) {
	o := newTestOAuthConnect(t)

	testCases := []struct {
		name          string
		rawBytes      json.RawMessage
		pattern       string
		expected      bool
		expectedError bool
	}{
		{
			name: "Should match username and email display",
			rawBytes: mustMarshalOAuthAnswer(t, shared.OAuthConnectAnswer{
				Username: "user",
				Email:    "user@example.com",
			}),
			pattern:  `user\(user@example\.com\)`,
			expected: true,
		},
		{
			name: "Should match username only display",
			rawBytes: mustMarshalOAuthAnswer(t, shared.OAuthConnectAnswer{
				Username: "alice",
			}),
			pattern:  `^alice$`,
			expected: true,
		},
		{
			name: "Should match email only display",
			rawBytes: mustMarshalOAuthAnswer(t, shared.OAuthConnectAnswer{
				Email: "alice@example.com",
			}),
			pattern:  `@example\.com$`,
			expected: true,
		},
		{
			name:     "Should match empty display with caret-dollar pattern",
			rawBytes: mustMarshalOAuthAnswer(t, shared.OAuthConnectAnswer{}),
			pattern:  `^$`,
			expected: true,
		},
		{
			name:     "Should not match empty display with non-empty pattern",
			rawBytes: mustMarshalOAuthAnswer(t, shared.OAuthConnectAnswer{}),
			pattern:  `.+`,
			expected: false,
		},
		{
			name:          "Should error on invalid JSON",
			rawBytes:      json.RawMessage(`not-json`),
			pattern:       `^$`,
			expectedError: true,
		},
		{
			name: "Should error on invalid regex",
			rawBytes: mustMarshalOAuthAnswer(t, shared.OAuthConnectAnswer{
				Email: "alice@example.com",
			}),
			pattern:       "(",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			match, err := o.MatchesPattern(tc.rawBytes, tc.pattern)

			if tc.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.expected, match)
		})
	}
}

func mustMarshalOAuthAnswer(t *testing.T, answer shared.OAuthConnectAnswer) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(answer)
	if err != nil {
		t.Fatalf("Failed to marshal answer: %v", err)
	}
	return raw
}

func newTestOAuthConnect(t *testing.T) OAuthConnect {
	t.Helper()

	metadata, err := GenerateOauthConnectMetadata("google")
	if err != nil {
		t.Fatalf("Failed to generate oauth metadata: %v", err)
	}

	return OAuthConnect{
		question: Question{
			ID:       uuid.New(),
			Metadata: metadata,
		},
		formID: uuid.New(),
	}
}

package shared

import (
	"fmt"

	"github.com/google/uuid"
)

// ShortTextAnswer represents answer for short_text question type
type ShortTextAnswer struct {
	Value string `json:"value"`
}

// LongTextAnswer represents answer for long_text question type
type LongTextAnswer struct {
	Value string `json:"value"`
}

// HyperlinkAnswer represents answer for hyperlink question type
type HyperlinkAnswer struct {
	Value string `json:"value"` // URL string
}

// DateAnswer represents answer for date question type
// Supports flexible date formats: year only, year-month, or full date
type DateAnswer struct {
	Year  *int `json:"year,omitempty"`  // Year component (e.g., 2026)
	Month *int `json:"month,omitempty"` // Month component (1-12)
	Day   *int `json:"day,omitempty"`   // Day component (1-31)
}

// String returns the string representation of the date answer
// Returns empty string if all components are nil
// Formats: "2026", "2026-02", "2026-02-15", "02-15", "02", etc.
func (d DateAnswer) String() string {
	// Build the string based on which components exist
	if d.Year != nil {
		if d.Month != nil {
			if d.Day != nil {
				// Year-Month-Day: "2026-02-15"
				return fmt.Sprintf("%04d-%02d-%02d", *d.Year, *d.Month, *d.Day)
			}
			// Year-Month: "2026-02"
			return fmt.Sprintf("%04d-%02d", *d.Year, *d.Month)
		}
		if d.Day != nil {
			// Year-Day (no month, unusual): "2026-15"
			return fmt.Sprintf("%04d-%02d", *d.Year, *d.Day)
		}
		// Year only: "2026"
		return fmt.Sprintf("%04d", *d.Year)
	}

	// No year component
	if d.Month != nil {
		if d.Day != nil {
			// Month-Day: "02-15"
			return fmt.Sprintf("%02d-%02d", *d.Month, *d.Day)
		}
		// Month only: "02"
		return fmt.Sprintf("%02d", *d.Month)
	}

	if d.Day != nil {
		// Day only: "15"
		return fmt.Sprintf("%02d", *d.Day)
	}

	// All nil
	return ""
}

// ChoiceSnapshot represents a snapshot of a choice option at the time of answer
type ChoiceSnapshot struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SingleChoiceAnswer represents answer for single_choice and dropdown question types
type SingleChoiceAnswer struct {
	ChoiceID uuid.UUID      `json:"choiceId"`
	Snapshot ChoiceSnapshot `json:"snapshot"`
}

// MultipleChoiceAnswer represents answer for multiple_choice question type
type MultipleChoiceAnswer struct {
	Choices []struct {
		ChoiceID uuid.UUID      `json:"choiceId"`
		Snapshot ChoiceSnapshot `json:"snapshot"`
	} `json:"choices"`
}

// DetailedMultipleChoiceAnswer represents answer for detailed_multiple_choice question type
type DetailedMultipleChoiceAnswer struct {
	Choices []struct {
		ChoiceID uuid.UUID      `json:"choiceId"`
		Snapshot ChoiceSnapshot `json:"snapshot"`
	} `json:"choices"`
}

// RankingAnswer represents answer for ranking question type
type RankingAnswer struct {
	RankedChoices []struct {
		ChoiceID uuid.UUID      `json:"choiceId"`
		Snapshot ChoiceSnapshot `json:"snapshot"`
		Rank     int            `json:"rank"` // 1-based ranking order
	} `json:"rankedChoices"`
}

// LinearScaleAnswer represents answer for linear_scale question type
type LinearScaleAnswer struct {
	Value int `json:"value"` // Numeric value within configured min/max range
}

// RatingAnswer represents answer for rating question type
type RatingAnswer struct {
	Value int `json:"value"` // Numeric value within configured min/max range
}

// UploadFileAnswer represents answer for upload_file question type
type UploadFileAnswer struct {
	FileIDs []string `json:"fileIds"` // Array of uploaded file IDs or URLs
}

// OAuthConnectAnswer represents answer for oauth_connect question type
type OAuthConnectAnswer struct {
	Provider   string `json:"provider"`   // OAuth provider (e.g., "google", "github")
	ProviderID string `json:"providerId"` // User ID from the OAuth provider
	Email      string `json:"email"`      // Connected account email
	Username   string `json:"username"`   // Connected account username (if available)
}

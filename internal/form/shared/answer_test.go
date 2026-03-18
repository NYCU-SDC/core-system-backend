package shared

import "testing"

func intPtr(v int) *int {
	return &v
}

func TestDateAnswer_String(t *testing.T) {
	tests := []struct {
		name     string
		input    DateAnswer
		expected string
	}{
		{
			name:     "Should return empty string when all components are nil",
			input:    DateAnswer{},
			expected: "",
		},
		{
			name:     "Should format year only as YYYY",
			input:    DateAnswer{Year: intPtr(2026)},
			expected: "2026",
		},
		{
			name:     "Should format year-month as YYYY-MM",
			input:    DateAnswer{Year: intPtr(2026), Month: intPtr(2)},
			expected: "2026-02",
		},
		{
			name:     "Should format full date as YYYY-MM-DD",
			input:    DateAnswer{Year: intPtr(2026), Month: intPtr(2), Day: intPtr(15)},
			expected: "2026-02-15",
		},
		{
			name:     "Should format month-day as MM-DD when year is nil",
			input:    DateAnswer{Month: intPtr(2), Day: intPtr(15)},
			expected: "02-15",
		},
		{
			name:     "Should format month only as MM",
			input:    DateAnswer{Month: intPtr(2)},
			expected: "02",
		},
		{
			name:     "Should format day only as DD",
			input:    DateAnswer{Day: intPtr(15)},
			expected: "15",
		},
		{
			name:     "Should format year-day as YYYY-DD when month is nil",
			input:    DateAnswer{Year: intPtr(2026), Day: intPtr(15)},
			expected: "2026-15",
		},
		{
			name:     "Should pad single digit month with zero",
			input:    DateAnswer{Year: intPtr(2026), Month: intPtr(1)},
			expected: "2026-01",
		},
		{
			name:     "Should pad single digit day with zero",
			input:    DateAnswer{Year: intPtr(2026), Month: intPtr(12), Day: intPtr(5)},
			expected: "2026-12-05",
		},
		{
			name:     "Should handle double digit month and day",
			input:    DateAnswer{Year: intPtr(2026), Month: intPtr(12), Day: intPtr(31)},
			expected: "2026-12-31",
		},
		{
			name:     "Should handle minimum year value with zero padding",
			input:    DateAnswer{Year: intPtr(1), Month: intPtr(1), Day: intPtr(1)},
			expected: "0001-01-01",
		},
		{
			name:     "Should handle maximum year value",
			input:    DateAnswer{Year: intPtr(9999), Month: intPtr(12), Day: intPtr(31)},
			expected: "9999-12-31",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.String()
			if result != tt.expected {
				t.Errorf("DateAnswer.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

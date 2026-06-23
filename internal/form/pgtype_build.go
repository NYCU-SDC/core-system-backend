package form

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// optionalPtrText maps an optional JSON field to pgtype.Text: unset pointer → unset SQL param (COALESCE noop).
func optionalPtrText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// optionalPtrTimestamptz maps an optional time to pgtype.Timestamptz.
func optionalPtrTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// nonEmptyText maps a dressing (or similar) string to pgtype.Text: empty string stays SQL-unset (Valid:false).
func nonEmptyText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

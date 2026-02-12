package formbuilder

import (
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/test/testdata"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

type Builder struct {
	t  *testing.T
	db dbbuilder.DBTX
}

func New(t *testing.T, db dbbuilder.DBTX) *Builder {
	return &Builder{t: t, db: db}
}

func (b Builder) Queries() *form.Queries {
	return form.New(b.db)
}

func (b Builder) Create(opts ...Option) form.CreateRow {
	queries := b.Queries()

	p := &FactoryParams{
		Title:                  testdata.RandomName(),
		Description:            testdata.RandomDescription(),
		MessageAfterSubmission: testdata.RandomDescription(),
		PublishTime:            pgtype.Timestamptz{Valid: false},
		GoogleSheetUrl:         nil,
		Visibility:             form.Visibility("private"),
		Deadline:               pgtype.Timestamptz{Valid: false},
	}
	for _, opt := range opts {
		opt(p)
	}

	preview := pgtype.Text{Valid: false}
	if p.PreviewMessage != nil {
		preview = pgtype.Text{String: *p.PreviewMessage, Valid: true}
	}

	googleSheet := pgtype.Text{Valid: false}
	if p.GoogleSheetUrl != nil && *p.GoogleSheetUrl != "" {
		googleSheet = pgtype.Text{String: *p.GoogleSheetUrl, Valid: true}
	}

	dressingColor := pgtype.Text{Valid: false}
	dressingHeaderFont := pgtype.Text{Valid: false}
	dressingQuestionFont := pgtype.Text{Valid: false}
	dressingTextFont := pgtype.Text{Valid: false}

	if p.Dressing != nil {
		if p.Dressing.Color != nil && *p.Dressing.Color != "" {
			dressingColor = pgtype.Text{String: *p.Dressing.Color, Valid: true}
		}
		if p.Dressing.HeaderFont != nil && *p.Dressing.HeaderFont != "" {
			dressingHeaderFont = pgtype.Text{String: *p.Dressing.HeaderFont, Valid: true}
		}
		if p.Dressing.QuestionFont != nil && *p.Dressing.QuestionFont != "" {
			dressingQuestionFont = pgtype.Text{String: *p.Dressing.QuestionFont, Valid: true}
		}
		if p.Dressing.TextFont != nil && *p.Dressing.TextFont != "" {
			dressingTextFont = pgtype.Text{String: *p.Dressing.TextFont, Valid: true}
		}
	}

	formRow, err := queries.Create(context.Background(), form.CreateParams{
		Title:                  p.Title,
		Description:            pgtype.Text{String: p.Description, Valid: p.Description != ""},
		PreviewMessage:         preview,
		UnitID:                 p.UnitID,
		LastEditor:             p.LastEditor,
		Deadline:               p.Deadline,
		PublishTime:            p.PublishTime,
		MessageAfterSubmission: p.MessageAfterSubmission,
		GoogleSheetUrl:         googleSheet,
		Visibility:             p.Visibility,
		DressingColor:          dressingColor,
		DressingHeaderFont:     dressingHeaderFont,
		DressingQuestionFont:   dressingQuestionFont,
		DressingTextFont:       dressingTextFont,
	})
	require.NoError(b.t, err)

	return formRow
}

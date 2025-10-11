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
		Title:       testdata.RandomName(),
		Description: testdata.RandomDescription(),
	}
	for _, opt := range opts {
		opt(p)
	}

	preview := pgtype.Text{Valid: false}
	if p.PreviewMessage != nil {
		preview = pgtype.Text{String: *p.PreviewMessage, Valid: true}
	}

	formRow, err := queries.Create(context.Background(), form.CreateParams{
		Title:          p.Title,
		Description:    pgtype.Text{String: p.Description, Valid: p.Description != ""},
		PreviewMessage: preview,
		UnitID:         p.UnitID,
		LastEditor:     p.LastEditor,
		Deadline:       p.Deadline,
	})
	require.NoError(b.t, err)

	return formRow
}

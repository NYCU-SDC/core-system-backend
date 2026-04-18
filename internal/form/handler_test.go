package form

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"NYCU-SDC/core-system-backend/internal/markdown"
	"NYCU-SDC/core-system-backend/internal/user"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestToResponse_proseMirrorAndHTML(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hi"}]}]}`)
	md := markdown.NewService(zap.NewNop())
	canonical, html, err := md.ProcessProseMirrorJSON(context.Background(), doc)
	require.NoError(t, err)

	f := Form{
		ID:                     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Title:                  "T",
		DescriptionJson:        canonical,
		DescriptionHtml:        html,
		PreviewMessage:         pgtype.Text{String: "pv", Valid: true},
		MessageAfterSubmission: "thanks",
		Status:                 StatusDraft,
		UnitID:                 pgtype.UUID{Bytes: uuid.MustParse("22222222-2222-2222-2222-222222222222"), Valid: true},
		LastEditor:             uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		CreatedAt:              pgtype.Timestamptz{Time: time.Unix(1, 0).UTC(), Valid: true},
		UpdatedAt:              pgtype.Timestamptz{Time: time.Unix(2, 0).UTC(), Valid: true},
		Visibility:             VisibilityPrivate,
	}

	resp := ToResponse(f, "unit", "org", user.User{
		ID:       f.LastEditor,
		Name:     pgtype.Text{String: "Ed", Valid: true},
		Username: pgtype.Text{String: "ed", Valid: true},
	}, nil)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(resp.Description, &decoded))
	require.Equal(t, "doc", decoded["type"])
	require.NotEmpty(t, resp.DescriptionHTML)
	require.Equal(t, "pv", resp.PreviewMessage)
}

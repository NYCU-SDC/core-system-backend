package formbuilder

import (
	"NYCU-SDC/core-system-backend/internal/form"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Option func(*FactoryParams)

type DressingParams struct {
	Color        *string
	HeaderFont   *string
	QuestionFont *string
	TextFont     *string
}

type FactoryParams struct {
	Title                  string
	Description            string
	PreviewMessage         *string
	UnitID                 pgtype.UUID
	LastEditor             uuid.UUID
	Deadline               pgtype.Timestamptz
	PublishTime            pgtype.Timestamptz
	MessageAfterSubmission string
	GoogleSheetUrl         *string
	Visibility             form.Visibility
	Dressing               *DressingParams
}

func WithTitle(title string) Option {
	return func(p *FactoryParams) { p.Title = title }
}

func WithDescription(description string) Option {
	return func(p *FactoryParams) { p.Description = description }
}

func WithPreviewMessage(preview string) Option {
	return func(p *FactoryParams) { p.PreviewMessage = &preview }
}

func WithUnitID(unitID uuid.UUID) Option {
	return func(p *FactoryParams) { p.UnitID = pgtype.UUID{Bytes: unitID, Valid: true} }
}

func WithLastEditor(userID uuid.UUID) Option {
	return func(p *FactoryParams) { p.LastEditor = userID }
}

func WithDeadline(deadline pgtype.Timestamptz) Option {
	return func(p *FactoryParams) { p.Deadline = deadline }
}

func WithPublishTime(publishTime pgtype.Timestamptz) Option {
	return func(p *FactoryParams) { p.PublishTime = publishTime }
}

func WithMessageAfterSubmission(msg string) Option {
	return func(p *FactoryParams) { p.MessageAfterSubmission = msg }
}

func WithGoogleSheetUrl(url string) Option {
	return func(p *FactoryParams) { p.GoogleSheetUrl = &url }
}

func WithVisibility(v form.Visibility) Option {
	return func(p *FactoryParams) { p.Visibility = v }
}

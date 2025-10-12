package formbuilder

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Option func(*FactoryParams)

type FactoryParams struct {
	Title          string
	Description    string
	PreviewMessage *string
	UnitID         pgtype.UUID
	LastEditor     uuid.UUID
	Deadline       pgtype.Timestamptz
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

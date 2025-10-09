package unitbuilder

import (
	"context"

	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/test/testdata"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"

	"testing"

	"github.com/google/uuid"
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

func (b Builder) Queries() *unit.Queries {
	return unit.New(b.db)
}

func (b Builder) Create(unitType unit.UnitType, opts ...Option) unit.Unit {
	queries := b.Queries()

	p := &FactoryParams{
		Name:        testdata.RandomName(),
		Description: testdata.RandomDescription(),
		Metadata:    []byte("{}"),
	}
	for _, opt := range opts {
		opt(p)
	}

	if p.OrgID.Valid && len(p.ParentIDs) == 0 {
		p.ParentIDs = append(p.ParentIDs, uuid.UUID(p.OrgID.Bytes))
	}

	var parentID pgtype.UUID
	if unitType == unit.UnitTypeUnit && p.OrgID.Valid {
		parentID = p.OrgID
	}

	unitRow, err := queries.Create(context.Background(), unit.CreateParams{
		Name:        pgtype.Text{String: p.Name, Valid: p.Name != ""},
		OrgID:       p.OrgID,
		Description: pgtype.Text{String: p.Description, Valid: p.Description != ""},
		Metadata:    p.Metadata,
		Type:        unitType,
		ParentID:    parentID,
	})
	require.NoError(b.t, err)

	for _, parentID := range p.ParentIDs {
		parent := parentID
		b.AddParentChild(&parent, unitRow.ID)
	}

	for _, childID := range p.ChildIDs {
		parent := unitRow.ID
		b.AddParentChild(&parent, childID)
	}

	return unitRow
}

func (b Builder) AddMember(unitID uuid.UUID, memberEmail string) unit.AddMemberRow {
	member, err := b.Queries().AddMember(context.Background(), unit.AddMemberParams{
		UnitID:      unitID,
		MemberEmail: memberEmail,
	})
	require.NoError(b.t, err)
	return member
}

func (b Builder) RemoveMember(unitID, memberID uuid.UUID) {
	err := b.Queries().RemoveMember(context.Background(), unit.RemoveMemberParams{
		UnitID:   unitID,
		MemberID: memberID,
	})
	require.NoError(b.t, err)
}

func (b Builder) AddParentChild(parentID *uuid.UUID, childID uuid.UUID) {
	var parent pgtype.UUID
	if parentID != nil {
		parent = pgtype.UUID{Bytes: *parentID, Valid: true}
	}

	_, err := b.Queries().UpdateParent(context.Background(), unit.UpdateParentParams{
		ID:       childID,
		ParentID: parent,
	})
	require.NoError(b.t, err)
}

func (b Builder) RemoveParentChild(childID uuid.UUID) {
	_, err := b.Queries().UpdateParent(context.Background(), unit.UpdateParentParams{
		ID:       childID,
		ParentID: pgtype.UUID{Valid: false},
	})
	require.NoError(b.t, err)
}

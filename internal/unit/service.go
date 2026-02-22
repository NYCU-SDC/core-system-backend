package unit

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/tenant"
	"context"
	"errors"
	"fmt"
	"regexp"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Create(ctx context.Context, arg CreateParams) (Unit, error)
	GetByID(ctx context.Context, id uuid.UUID) (Unit, error)
	GetAllOrganizations(ctx context.Context) ([]GetAllOrganizationsRow, error)
	ListOrganizationsOfUser(ctx context.Context, memberID uuid.UUID) ([]ListOrganizationsOfUserRow, error)
	GetOrganizationByIDWithSlug(ctx context.Context, id uuid.UUID) (GetOrganizationByIDWithSlugRow, error)
	ListSubUnits(ctx context.Context, parentID pgtype.UUID) ([]Unit, error)
	ListSubUnitIDs(ctx context.Context, parentID pgtype.UUID) ([]uuid.UUID, error)
	Update(ctx context.Context, arg UpdateParams) (Unit, error)
	UpdateParent(ctx context.Context, arg UpdateParentParams) (Unit, error)
	Delete(ctx context.Context, id uuid.UUID) error

	AddMember(ctx context.Context, arg AddMemberParams) (AddMemberRow, error)
	AddUnitMemberWithRole(ctx context.Context, arg AddUnitMemberWithRoleParams) (UnitMember, error)
	ListMembers(ctx context.Context, unitID uuid.UUID) ([]ListMembersRow, error)
	ListUnitsMembers(ctx context.Context, unitIDs []uuid.UUID) ([]ListUnitsMembersRow, error)
	RemoveMember(ctx context.Context, arg RemoveMemberParams) error

	CountMembers(ctx context.Context, unitID uuid.UUID) (int64, error)
	CountMembersByRole(ctx context.Context, arg CountMembersByRoleParams) (int64, error)
	GetMemberRole(ctx context.Context, arg GetMemberRoleParams) (UnitRole, error)
	UpdateMemberRole(ctx context.Context, arg UpdateMemberRoleParams) error
	LockAdminsForUnit(ctx context.Context, unitID uuid.UUID) ([]uuid.UUID, error)

	WithTx(tx pgx.Tx) *Queries
}

type TxBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

type Service struct {
	db          DBTX
	logger      *zap.Logger
	queries     Querier
	tracer      trace.Tracer
	tenantStore tenantStore
}

type Organization struct {
	Unit Unit
	Slug string
}

type Base struct {
	ID          uuid.UUID
	Name        string
	Description string
	Metadata    []byte
}

type Type int

const (
	TypeOrg Type = iota
	TypeUnit
)

var typeStrings = [...]string{"organization", "unit"}

func (t Type) String() string {
	if int(t) < 0 || int(t) >= len(typeStrings) {
		return "unknown"
	}
	return typeStrings[t]
}

func NewService(logger *zap.Logger, db DBTX, tenantStore tenantStore) *Service {
	return &Service{
		db:          db,
		logger:      logger,
		queries:     New(db),
		tracer:      otel.Tracer("unit/service"),
		tenantStore: tenantStore,
	}
}

func (s *Service) CreateOrganization(ctx context.Context, name string, description string, slug string, currentUserID uuid.UUID, metadata []byte) (Unit, error) {
	traceCtx, span := s.tracer.Start(ctx, "CreateOrganization")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exists, err := s.tenantStore.SlugExists(traceCtx, slug)
	if err != nil {
		span.RecordError(err)
		return Unit{}, err
	}

	if exists {
		span.RecordError(internal.ErrOrgSlugAlreadyExists)
		return Unit{}, internal.ErrOrgSlugAlreadyExists
	}

	org, err := s.queries.Create(traceCtx, CreateParams{
		Name:        pgtype.Text{String: name, Valid: name != ""},
		OrgID:       pgtype.UUID{Valid: false},
		Description: pgtype.Text{String: description, Valid: true},
		Metadata:    metadata,
		Type:        UnitTypeOrganization,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create organization")
		span.RecordError(err)
		return Unit{}, err
	}

	_, err = s.tenantStore.Create(traceCtx, org.ID, currentUserID, slug)
	if err != nil {
		span.RecordError(err)
		return Unit{}, err
	}

	logger.Info("Created organization",
		zap.String("org_id", org.ID.String()),
		zap.String("name", org.Name.String),
		zap.String("description", org.Description.String),
		zap.String("metadata", string(org.Metadata)))

	return org, nil
}

// CreateUnit creates a new unit or organization
func (s *Service) CreateUnit(ctx context.Context, name string, description string, slug string, metadata []byte) (Unit, error) {
	traceCtx, span := s.tracer.Start(ctx, "CreateUnit")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	_, orgID, err := s.tenantStore.GetSlugStatus(traceCtx, slug)
	if err != nil {
		span.RecordError(err)
		return Unit{}, err
	}

	unit, err := s.queries.Create(traceCtx, CreateParams{
		Name:        pgtype.Text{String: name, Valid: name != ""},
		OrgID:       pgtype.UUID{Bytes: orgID, Valid: true},
		ParentID:    pgtype.UUID{Bytes: orgID, Valid: true},
		Description: pgtype.Text{String: description, Valid: true},
		Metadata:    metadata,
		Type:        UnitTypeUnit,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create unit")
		span.RecordError(err)
		return Unit{}, err
	}

	logger.Info(fmt.Sprintf("Created %s", unit.Type),
		zap.String("unit_id", unit.ID.String()),
		zap.String("org_id", orgID.String()),
		zap.String("name", unit.Name.String),
		zap.String("description", unit.Description.String),
		zap.String("metadata", string(unit.Metadata)))

	return unit, nil
}

func (s *Service) GetAllOrganizations(ctx context.Context) ([]Organization, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetAllOrganizations")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	organizations, err := s.queries.GetAllOrganizations(traceCtx)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get all organizations")
		span.RecordError(err)
		return nil, err
	}

	result := make([]Organization, 0, len(organizations))
	for _, o := range organizations {
		result = append(result, Organization{
			Unit: Unit{
				ID:          o.ID,
				OrgID:       o.OrgID,
				ParentID:    o.ParentID,
				Type:        o.Type,
				Name:        o.Name,
				Description: o.Description,
				Metadata:    o.Metadata,
				CreatedAt:   o.CreatedAt,
				UpdatedAt:   o.UpdatedAt,
			},
			Slug: o.Slug.String,
		})
	}

	return result, nil
}

func (s *Service) ListOrganizationsOfUser(ctx context.Context, userID uuid.UUID) ([]Organization, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListOrganizationsOfUser")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	organizations, err := s.queries.ListOrganizationsOfUser(traceCtx, userID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "organizations", "member_id", userID.String(), logger, "get all organizations of user")
		span.RecordError(err)
		return nil, err
	}

	result := make([]Organization, 0, len(organizations))
	for _, o := range organizations {
		result = append(result, Organization{
			Unit: Unit{
				ID:          o.ID,
				OrgID:       o.OrgID,
				Type:        o.Type,
				Name:        o.Name,
				Description: o.Description,
				Metadata:    o.Metadata,
				CreatedAt:   o.CreatedAt,
				UpdatedAt:   o.UpdatedAt,
			},
			Slug: o.Slug.String,
		})
	}

	return result, nil
}

func (s *Service) GetOrganizationByIDWithSlug(ctx context.Context, id uuid.UUID) (Organization, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetOrganizationByIDWithSlug")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	r, err := s.queries.GetOrganizationByIDWithSlug(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "organizations", "id", id.String(), logger, "get organization by id with slug")
		span.RecordError(err)
		return Organization{}, err
	}

	return Organization{
		Unit: Unit{
			ID:          r.ID,
			OrgID:       r.OrgID,
			ParentID:    r.ParentID,
			Type:        r.Type,
			Name:        r.Name,
			Description: r.Description,
			Metadata:    r.Metadata,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		},
		Slug: r.Slug.String,
	}, nil
}

// GetByID retrieves a unit by ID
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, unitType Type) (Unit, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	unit, err := s.queries.GetByID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "unit", "unitType", unitType.String(), logger, "get by id")
		span.RecordError(err)
		return Unit{}, err
	}
	return unit, nil
}

// ListSubUnits retrieves all subunits of a parent unit
func (s *Service) ListSubUnits(ctx context.Context, id uuid.UUID, unitType Type) ([]Unit, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListSubUnits")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	subUnits, err := s.queries.ListSubUnits(traceCtx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, fmt.Sprintf("list sub units of an %s", unitType.String()))
		span.RecordError(err)
		return nil, err
	}

	if subUnits == nil {
		subUnits = make([]Unit, 0)
	}

	logger.Info(fmt.Sprintf("Listed sub units of an %s", unitType.String()), zap.String("parent_id", id.String()), zap.Int("count", len(subUnits)))
	return subUnits, nil
}

// ListSubUnitIDs retrieves all child unit IDs of a parent unit
func (s *Service) ListSubUnitIDs(ctx context.Context, id uuid.UUID, unitType Type) ([]uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListSubUnitIDs")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	subUnitIDs, err := s.queries.ListSubUnitIDs(traceCtx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, fmt.Sprintf("list sub unit IDs of an %s", unitType.String()))
		span.RecordError(err)
		return nil, err
	}

	if subUnitIDs == nil {
		subUnitIDs = []uuid.UUID{}
	}

	logger.Info(fmt.Sprintf("Listed sub unit IDs of an %s", unitType.String()), zap.String("parent_id", id.String()), zap.Int("count", len(subUnitIDs)))
	return subUnitIDs, nil
}

// UpdateOrg updates the fields of an organization
func (s *Service) UpdateOrg(ctx context.Context, originalSlug string, slug string, name string, description string, dbStrategy string, metadata []byte) (Unit, error) {
	traceCtx, span := s.tracer.Start(ctx, "UpdateUnit")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	_, orgID, err := s.tenantStore.GetSlugStatus(traceCtx, originalSlug)
	if err != nil {
		span.RecordError(err)
		return Unit{}, err
	}

	if slug != originalSlug {
		matched, err := regexp.MatchString(slugPattern, slug)
		if err != nil || !matched {
			span.RecordError(err)
			return Unit{}, internal.ErrOrgSlugInvalid
		}

		exists, err := s.tenantStore.SlugExists(traceCtx, slug)
		if err != nil {
			span.RecordError(err)
			return Unit{}, err
		}

		if exists {
			span.RecordError(internal.ErrOrgSlugAlreadyExists)
			return Unit{}, internal.ErrOrgSlugAlreadyExists
		}
	}

	var tenantDbStrategy tenant.DbStrategy

	if dbStrategy == "" || dbStrategy == string(DbStrategyShared) {
		tenantDbStrategy = "shared"
	} else if dbStrategy == string(DbStrategyIsolated) {
		tenantDbStrategy = "isolated"
	}

	_, err = s.tenantStore.Update(traceCtx, orgID, slug, tenantDbStrategy)
	if err != nil {
		span.RecordError(err)
		return Unit{}, err
	}

	unit, err := s.queries.Update(traceCtx, UpdateParams{
		ID:          orgID,
		Name:        pgtype.Text{String: name, Valid: name != ""},
		Description: pgtype.Text{String: description, Valid: true},
		Metadata:    metadata,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update unit")
		span.RecordError(err)
		return Unit{}, err
	}

	logger.Info("Updated unit",
		zap.String("unitID", unit.ID.String()),
		zap.String("unitName", unit.Name.String),
		zap.String("unitDescription", unit.Description.String),
		zap.ByteString("unitMetadata", unit.Metadata),
	)

	return unit, nil
}

// UpdateUnit updates the fields of a unit
func (s *Service) UpdateUnit(ctx context.Context, id uuid.UUID, name string, description string, metadata []byte) (Unit, error) {
	traceCtx, span := s.tracer.Start(ctx, "UpdateUnit")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	unit, err := s.queries.Update(traceCtx, UpdateParams{
		ID:          id,
		Name:        pgtype.Text{String: name, Valid: name != ""},
		Description: pgtype.Text{String: description, Valid: true},
		Metadata:    metadata,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update unit")
		span.RecordError(err)
		return Unit{}, err
	}

	logger.Info("Updated unit",
		zap.String("unitID", unit.ID.String()),
		zap.String("unitName", unit.Name.String),
		zap.String("unitDescription", unit.Description.String),
		zap.ByteString("unitMetadata", unit.Metadata),
	)

	return unit, nil
}

// Delete deletes a unit by ID
func (s *Service) Delete(ctx context.Context, id uuid.UUID, unitType Type) error {
	traceCtx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	err := s.queries.Delete(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "organizations", "id", id.String(), logger, fmt.Sprintf("delete %s", unitType.String()))
		span.RecordError(err)
		return err
	}

	logger.Info(fmt.Sprintf("Deleted %s", unitType), zap.String("ID: ", id.String()))

	return nil
}

// AddParent adds a parent-child relationship between two units
func (s *Service) AddParent(ctx context.Context, id uuid.UUID, parentID uuid.UUID) (Unit, error) {
	traceCtx, span := s.tracer.Start(ctx, "AddParent")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	pgParentID := pgtype.UUID{Valid: false}
	if parentID != uuid.Nil {
		pgParentID = pgtype.UUID{Bytes: parentID, Valid: true}
	}

	result, err := s.queries.UpdateParent(traceCtx, UpdateParentParams{
		ID:       id,
		ParentID: pgParentID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "add parent-child relationship")
		span.RecordError(err)
		return Unit{}, err
	}

	logger.Info("Added parent-child relationship", zap.String("parentID", parentID.String()), zap.String("id", id.String()))

	return result, nil
}

func (s *Service) GetMemberRole(
	ctx context.Context,
	unitID uuid.UUID,
	memberID uuid.UUID,
) (UnitRole, error) {

	traceCtx, span := s.tracer.Start(ctx, "GetMemberRole")
	defer span.End()

	logger := logutil.WithContext(traceCtx, s.logger)

	role, err := s.queries.GetMemberRole(traceCtx, GetMemberRoleParams{
		UnitID:   unitID,
		MemberID: memberID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", internal.ErrPermissionDenied
		}

		err = databaseutil.WrapDBError(err, logger, "get member role")
		span.RecordError(err)
		return "", err
	}

	return role, nil
}

func (s *Service) UpdateUnitMemberRole(
	ctx context.Context,
	unitID uuid.UUID,
	memberID uuid.UUID,
	newRole UnitRole,
) error {
	traceCtx, span := s.tracer.Start(ctx, "UpdateUnitMemberRole")
	defer span.End()

	logger := logutil.WithContext(traceCtx, s.logger)

	var (
		tx         pgx.Tx
		err        error
		needCommit bool
	)

	existingTx, ok := s.db.(pgx.Tx)
	if ok {
		// reuse
		tx = existingTx
	} else {
		// build tx
		beginner, ok := s.db.(TxBeginner)
		if !ok {
			return fmt.Errorf("db does not support transactions")
		}

		tx, err = beginner.BeginTx(traceCtx, pgx.TxOptions{})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "begin tx")
			span.RecordError(err)
			return err
		}

		needCommit = true

		defer func() {
			err := tx.Rollback(traceCtx)
			if err != nil &&
				!errors.Is(err, pgx.ErrTxClosed) {
				logger.Error("rollback failed", zap.Error(err))
			}
		}()
	}

	qtx := s.queries.WithTx(tx)

	// lock admins
	_, err = qtx.LockAdminsForUnit(traceCtx, unitID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "lock admin rows")
		span.RecordError(err)
		return err
	}

	currentRole, err := qtx.GetMemberRole(traceCtx, GetMemberRoleParams{
		UnitID:   unitID,
		MemberID: memberID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.RecordError(internal.ErrNotFound)
			return internal.ErrNotFound
		}
		err = databaseutil.WrapDBError(err, logger, "get member role")
		span.RecordError(err)
		return err
	}

	if currentRole == newRole {
		if needCommit {
			return tx.Commit(traceCtx)
		}
		return nil
	}

	if currentRole == UnitRoleAdmin && newRole != UnitRoleAdmin {
		count, err := qtx.CountMembersByRole(traceCtx, CountMembersByRoleParams{
			UnitID: unitID,
			Role:   UnitRoleAdmin,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "count admin")
			span.RecordError(err)
			return err
		}

		if count <= 1 {
			logger.Error("cannot remove last admin")
			span.RecordError(internal.ErrCannotRemoveLastAdmin)
			return internal.ErrCannotRemoveLastAdmin
		}
	}

	err = qtx.UpdateMemberRole(traceCtx, UpdateMemberRoleParams{
		UnitID:   unitID,
		MemberID: memberID,
		Role:     newRole,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update member role")
		span.RecordError(err)
		return err
	}

	if needCommit {
		return tx.Commit(traceCtx)
	}

	return nil
}

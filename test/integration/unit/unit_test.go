package unit

import (
	"NYCU-SDC/core-system-backend/internal/tenant"
	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	tenantbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/tenant"
	unitbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/unit"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	resourceManager, _, err := integration.GetOrInitResource()
	if err != nil {
		panic(err)
	}

	_, rollback, err := resourceManager.SetupPostgres()
	if err != nil {
		panic(err)
	}

	code := m.Run()

	rollback()
	resourceManager.Cleanup()

	os.Exit(code)
}

func TestUnitService_Create(t *testing.T) {
	type params struct {
		name         string
		parentUnitID uuid.UUID
		ownerID      uuid.UUID
		description  string
		slug         string
		metadata     []byte
		unitType     unit.Type
	}
	testCases := []struct {
		name        string
		params      params
		setup       func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context
		validate    func(t *testing.T, params params, db dbbuilder.DBTX, result unit.Unit)
		expectedErr bool
	}{
		{
			name: "Create organization without parent",
			params: params{
				name:        "student affairs",
				description: "handles student-related matters",
				metadata:    []byte(`{"timezone":"UTC"}`),
				unitType:    unit.TypeOrg,
			},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				userBuilder := userbuilder.New(t, db)
				user := userBuilder.Create()
				params.ownerID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, result unit.Unit) {
				require.NotZero(t, result.ID)
				require.False(t, result.OrgID.Valid)
				require.Equal(t, unit.UnitTypeOrganization, result.Type)
				require.Equal(t, params.name, result.Name.String)
				require.Equal(t, params.description, result.Description.String)
				require.JSONEq(t, string(params.metadata), string(result.Metadata))

				stored, err := unit.New(db).GetByID(context.Background(), result.ID)
				require.NoError(t, err)
				require.False(t, stored.OrgID.Valid)
				require.Equal(t, unit.UnitTypeOrganization, stored.Type)
				require.JSONEq(t, string(params.metadata), string(stored.Metadata))
			},
		},
		{
			name: "Create unit under existing organization",
			params: params{
				name:        "department of cs",
				description: "teaches computer science",
				slug:        "nycu",
				metadata:    []byte(`{"building":"EE"}`),
				unitType:    unit.TypeUnit,
			},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				user := userbuilder.New(t, db).Create()
				org := unitbuilder.New(t, db).Create(unit.UnitTypeOrganization)
				params.parentUnitID = org.ID
				tenantbuilder.New(t, db).Create(tenantbuilder.WithID(org.ID), tenantbuilder.WithSlug(params.slug), tenantbuilder.WithOwnerID(user.ID))

				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, result unit.Unit) {
				require.True(t, result.OrgID.Valid)
				require.Equal(t, params.parentUnitID, uuid.UUID(result.OrgID.Bytes))
				require.Equal(t, params.parentUnitID, uuid.UUID(result.ParentID.Bytes))
				require.Equal(t, unit.UnitTypeUnit, result.Type)
				require.Equal(t, params.name, result.Name.String)
				require.Equal(t, params.description, result.Description.String)
				require.JSONEq(t, string(params.metadata), string(result.Metadata))
			},
		},
		{
			name: "Fail when organization does not exist",
			params: params{
				name:         "ghost unit",
				description:  "references a missing org",
				slug:         "ghost",
				metadata:     []byte(`{"status":"ghost"}`),
				parentUnitID: uuid.New(),
				unitType:     unit.TypeUnit,
			},
			expectedErr: true,
		},
	}

	resourceManager, logger, err := integration.GetOrInitResource()
	if err != nil {
		t.Fatalf("failed to get resource manager: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			if err != nil {
				t.Fatalf("failed to setup postgres: %v", err)
			}
			defer rollback()

			ctx := context.Background()
			params := tc.params
			if tc.setup != nil {
				ctx = tc.setup(t, &params, db)
			}

			tenantStore := tenant.NewService(logger, db)
			unitService := unit.NewService(logger, db, tenantStore)

			var result unit.Unit
			if params.unitType == unit.TypeOrg {
				result, err = unitService.CreateOrganization(ctx, params.name, params.description, params.slug, params.ownerID, params.metadata)
				require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)
			} else {
				result, err = unitService.CreateUnit(ctx, params.name, params.description, params.slug, params.metadata)
				require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)
			}

			if tc.validate != nil {
				tc.validate(t, params, db, result)
			}
		})
	}
}

func TestUnitService_ListAllUnitsOfUser(t *testing.T) {
	type params struct {
		userID   uuid.UUID
		expected []uuid.UUID
	}
	testCases := []struct {
		name        string
		params      params
		setup       func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context
		expectedErr bool
	}{
		{
			name:   "List all organizations of user",
			params: params{},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				userB := userbuilder.New(t, db)
				unitB := unitbuilder.New(t, db)
				user := userB.Create(userbuilder.WithName("current-user"))
				orgOne := unitB.Create(unit.UnitTypeOrganization)
				orgTwo := unitB.Create(unit.UnitTypeOrganization)

				unitB.AddMember(orgOne.ID, user.ID)
				unitB.AddMember(orgTwo.ID, user.ID)

				params.userID = user.ID
				params.expected = []uuid.UUID{orgOne.ID, orgTwo.ID}
				return context.Background()
			},
			expectedErr: false,
		},
		{
			name:   "Return empty when user has no organizations",
			params: params{},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				userB := userbuilder.New(t, db)
				user := userB.Create(userbuilder.WithName("user-without-org"))

				params.userID = user.ID
				params.expected = []uuid.UUID{}
				return context.Background()
			},
			expectedErr: false,
		},
		{
			name:        "Error when user ID is invalid",
			params:      params{userID: uuid.New(), expected: []uuid.UUID{}},
			expectedErr: false,
		},
	}

	resourceManager, logger, err := integration.GetOrInitResource()
	if err != nil {
		t.Fatalf("failed to get resource manager: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			if err != nil {
				t.Fatalf("failed to setup postgres: %v", err)
			}
			defer rollback()

			ctx := context.Background()
			params := tc.params
			if tc.setup != nil {
				ctx = tc.setup(t, &params, db)
			}

			service := unit.NewService(logger, db)
			orgs, err := service.ListOrganizationsOfUser(ctx, params.userID)

			orgIDs := make([]uuid.UUID, len(orgs))
			for i, org := range orgs {
				orgIDs[i] = org.Unit.ID
			}

			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)
			require.ElementsMatch(t, params.expected, orgIDs)
		})
	}
}

func TestUnitService_ListSubUnits(t *testing.T) {
	type params struct {
		parentID uuid.UUID
		unitType unit.Type
		expected []unit.Unit
	}
	testCases := []struct {
		name        string
		params      params
		setup       func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context
		validate    func(t *testing.T, params params, db dbbuilder.DBTX, result []unit.Unit)
		expectedErr bool
	}{
		{
			name:   "List child units for organization",
			params: params{unitType: unit.TypeOrg},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				builder := unitbuilder.New(t, db)
				org := builder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("root-org"))
				params.parentID = org.ID

				childOneMetadata := []byte(`{"label":"child-one"}`)
				childOne := builder.Create(
					unit.UnitTypeUnit,
					unitbuilder.WithOrgID(org.ID),
					unitbuilder.WithName("child-one"),
					unitbuilder.WithDescription("first child"),
					unitbuilder.WithMetadata(childOneMetadata),
				)

				childTwoMetadata := []byte(`{"label":"child-two"}`)
				childTwo := builder.Create(
					unit.UnitTypeUnit,
					unitbuilder.WithOrgID(org.ID),
					unitbuilder.WithName("child-two"),
					unitbuilder.WithDescription("second child"),
					unitbuilder.WithMetadata(childTwoMetadata),
				)

				params.expected = []unit.Unit{childOne, childTwo}

				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, result []unit.Unit) {
				require.Len(t, result, len(params.expected))

				expectedByID := make(map[uuid.UUID]unit.Unit)
				for _, expected := range params.expected {
					expectedByID[expected.ID] = expected
				}

				for _, child := range result {
					require.True(t, child.OrgID.Valid)
					require.Equal(t, params.parentID, uuid.UUID(child.OrgID.Bytes))
					require.Equal(t, unit.UnitTypeUnit, child.Type)

					expected, ok := expectedByID[child.ID]
					require.True(t, ok, "unexpected child %s", child.ID)
					require.Equal(t, expected.Name.String, child.Name.String)
					require.Equal(t, expected.Description.String, child.Description.String)
					require.JSONEq(t, string(expected.Metadata), string(child.Metadata))
				}
			},
		},
		{
			name:   "Return empty when parent-child removed",
			params: params{unitType: unit.TypeOrg},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				builder := unitbuilder.New(t, db)
				org := builder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("detached-org"))
				params.parentID = org.ID

				child := builder.Create(
					unit.UnitTypeUnit,
					unitbuilder.WithOrgID(org.ID),
					unitbuilder.WithName("orphan-child"),
				)
				builder.RemoveParentChild(child.ID)

				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, result []unit.Unit) {
				require.Empty(t, result)
			},
		},
	}

	resourceManager, logger, err := integration.GetOrInitResource()
	if err != nil {
		t.Fatalf("failed to get resource manager: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			if err != nil {
				t.Fatalf("failed to setup postgres: %v", err)
			}
			defer rollback()

			ctx := context.Background()
			params := tc.params
			if tc.setup != nil {
				ctx = tc.setup(t, &params, db)
			}

			tenantStore := tenant.NewService(logger, db)
			unitService := unit.NewService(logger, db, tenantStore)

			result, err := unitService.ListSubUnits(ctx, params.parentID, params.unitType)
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.validate != nil {
				tc.validate(t, params, db, result)
			}
		})
	}
}

func TestUnitService_ListSubUnitIDs(t *testing.T) {
	type params struct {
		parentID uuid.UUID
		unitType unit.Type
		expected []uuid.UUID
	}

	testCases := []struct {
		name        string
		params      params
		setup       func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context
		validate    func(t *testing.T, params params, db dbbuilder.DBTX, result []uuid.UUID)
		expectedErr bool
	}{
		{
			name:   "List child unit IDs",
			params: params{unitType: unit.TypeOrg},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				builder := unitbuilder.New(t, db)
				org := builder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("org-with-ids"))
				params.parentID = org.ID

				childOne := builder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("ids-child-one"))
				childTwo := builder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("ids-child-two"))

				params.expected = []uuid.UUID{childOne.ID, childTwo.ID}

				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, result []uuid.UUID) {
				require.ElementsMatch(t, params.expected, result)
			},
		},
		{
			name:   "Return empty IDs when relationships removed",
			params: params{unitType: unit.TypeOrg},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				builder := unitbuilder.New(t, db)
				org := builder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("ids-detached-org"))
				params.parentID = org.ID

				child := builder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("ids-orphan-child"))
				builder.RemoveParentChild(child.ID)

				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, result []uuid.UUID) {
				require.Empty(t, result)
			},
		},
	}

	resourceManager, logger, err := integration.GetOrInitResource()
	if err != nil {
		t.Fatalf("failed to get resource manager: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			if err != nil {
				t.Fatalf("failed to setup postgres: %v", err)
			}
			defer rollback()

			ctx := context.Background()
			params := tc.params
			if tc.setup != nil {
				ctx = tc.setup(t, &params, db)
			}

			tenantStore := tenant.NewService(logger, db)
			unitService := unit.NewService(logger, db, tenantStore)

			result, err := unitService.ListSubUnitIDs(ctx, params.parentID, params.unitType)
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.validate != nil {
				tc.validate(t, params, db, result)
			}
		})
	}
}

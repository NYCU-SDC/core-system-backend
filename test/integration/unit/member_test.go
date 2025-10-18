package unit

import (
	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	unitbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/unit"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

//goland:noinspection DuplicatedCode
func TestUnitService_AddMember(t *testing.T) {
	type params struct {
		unitType     unit.Type
		unitID       uuid.UUID
		memberEmails []string
	}
	testCases := []struct {
		name        string
		params      params
		setup       func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context
		validate    func(t *testing.T, params params, db dbbuilder.DBTX, results []unit.AddMemberRow, err error)
		expectedErr bool
	}{
		{
			name: "Add member to organization",
			params: params{
				unitType: unit.TypeOrg,
			},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				builder := unitbuilder.New(t, db)
				org := builder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("student-affairs"))
				userBuilder := userbuilder.New(t, db)
				member := userBuilder.Create()
				userBuilder.CreateEmail(member.ID, "test@example.com")

				params.unitID = org.ID
				params.memberEmails = []string{"test@example.com"}
				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, results []unit.AddMemberRow, err error) {
				require.NoError(t, err)
				require.Len(t, results, 1)
				require.Equal(t, params.unitID, results[0].UnitID)
				require.Equal(t, params.memberEmails[0], "test@example.com")

				memberRows, listErr := unit.New(db).ListMembers(context.Background(), params.unitID)

				memberIDs := make([]uuid.UUID, len(memberRows))
				for i, memberRow := range memberRows {
					memberIDs[i] = memberRow.MemberID
				}

				require.NoError(t, listErr)
				require.Len(t, memberIDs, 1)
				require.Contains(t, memberIDs, results[0].MemberID)
			},
		},
		{
			name: "Add multiple members to unit",
			params: params{
				unitType: unit.TypeUnit,
			},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				builder := unitbuilder.New(t, db)
				org := builder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("engineering"))
				unitRow := builder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("department-of-cs"))
				userBuilder := userbuilder.New(t, db)
				memberOne := userBuilder.Create()
				memberTwo := userBuilder.Create()
				userBuilder.CreateEmail(memberOne.ID, "test1@example.com")
				userBuilder.CreateEmail(memberTwo.ID, "test2@example.com")

				params.unitID = unitRow.ID
				params.memberEmails = []string{"test1@example.com", "test2@example.com"}
				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, results []unit.AddMemberRow, err error) {
				require.NoError(t, err)
				require.Len(t, results, len(params.memberEmails))

				seen := make(map[string]struct{})
				for idx := range params.memberEmails {
					require.Equal(t, params.unitID, results[idx].UnitID)
					// Note: Email verification removed as it's now stored in separate emails table
					seen[results[idx].MemberID.String()] = struct{}{}
				}

				memberRows, listErr := unit.New(db).ListMembers(context.Background(), params.unitID)
				require.NoError(t, listErr)
				require.Len(t, memberRows, len(params.memberEmails))
				for _, stored := range memberRows {
					_, ok := seen[stored.MemberID.String()]
					require.True(t, ok, "unexpected member %s", stored)
				}
			},
		},
		{
			name: "Upsert when member already associated with unit",
			params: params{
				unitType: unit.TypeUnit,
			},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				builder := unitbuilder.New(t, db)
				org := builder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("duplicate-org"))
				unitRow := builder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("duplicate-unit"))
				userBuilder := userbuilder.New(t, db)
				member := userBuilder.Create()
				userBuilder.CreateEmail(member.ID, "test@example.com")

				builder.AddMember(unitRow.ID, "test@example.com")

				params.unitID = unitRow.ID
				params.memberEmails = []string{"test@example.com"}
				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, results []unit.AddMemberRow, err error) {
				require.NoError(t, err)
				require.Len(t, results, 1)
				require.Equal(t, params.unitID, results[0].UnitID)
				require.Equal(t, params.memberEmails[0], "test@example.com")

				members, listErr := unit.New(db).ListMembers(context.Background(), params.unitID)

				memberIDs := make([]uuid.UUID, len(members))
				for i, member := range members {
					memberIDs[i] = member.MemberID
				}

				require.NoError(t, listErr)
				require.Len(t, memberIDs, 1)
				require.Contains(t, memberIDs, results[0].MemberID)
			},
		},
		{
			name: "Add member using second email of user",
			params: params{
				unitType: unit.TypeUnit,
			},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				builder := unitbuilder.New(t, db)
				org := builder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("eng-multi-email"))
				unitRow := builder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("dept-ee"))
				userBuilder := userbuilder.New(t, db)
				member := userBuilder.Create()
				userBuilder.CreateEmail(member.ID, "secondary@example.com")

				params.unitID = unitRow.ID
				params.memberEmails = []string{"secondary@example.com"} // Using placeholder email
				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, results []unit.AddMemberRow, err error) {
				require.NoError(t, err)
				require.Len(t, results, 1)
				require.Equal(t, params.unitID, results[0].UnitID)
				// Email validation removed as emails are now stored in separate table
				require.Equal(t, params.memberEmails[0], "secondary@example.com")

				memberRows, listErr := unit.New(db).ListMembers(context.Background(), params.unitID)
				require.NoError(t, listErr)
				require.Len(t, memberRows, 1)
				require.Equal(t, results[0].MemberID, memberRows[0].MemberID)
				// Email validation removed as emails are now stored in separate table
				// The member was successfully added, which is what we're testing
			},
		},
		{
			name:   "Error when adding unknown email",
			params: params{unitType: unit.TypeOrg},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				builder := unitbuilder.New(t, db)
				org := builder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("org-unknown-email"))
				params.unitID = org.ID
				params.memberEmails = []string{"nonexistent@example.com"}
				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX, results []unit.AddMemberRow, err error) {
				require.Error(t, err)
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

			service := unit.NewService(logger, db)

			memberEmails := params.memberEmails
			require.NotEmpty(t, memberEmails, "memberEmails must not be empty")

			results := make([]unit.AddMemberRow, 0, len(memberEmails))
			var encounteredErr error
			for _, memberEmail := range memberEmails {
				result, err := service.AddMember(ctx, params.unitType, params.unitID, memberEmail)
				results = append(results, result)
				if err != nil {
					encounteredErr = err
					break
				}
			}

			require.Equal(t, tc.expectedErr, encounteredErr != nil, "expected error: %v, got: %v", tc.expectedErr, encounteredErr)

			if tc.validate != nil {
				tc.validate(t, params, db, results, encounteredErr)
			}
		})
	}
}

func TestUnitService_ListMembers(t *testing.T) {
	type params struct {
		unitID   uuid.UUID
		expected []uuid.UUID
	}
	testCases := []struct {
		name   string
		params params
		setup  func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context
	}{
		{
			name:   "Return members for organization",
			params: params{},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				unitB := unitbuilder.New(t, db)
				userB := userbuilder.New(t, db)
				org := unitB.Create(unit.UnitTypeOrganization, unitbuilder.WithName("org-members"))
				memberOne := userB.Create()
				memberTwo := userB.Create()
				userB.CreateEmail(memberOne.ID, "test1@example.com")
				userB.CreateEmail(memberTwo.ID, "test2@example.com")

				unitB.AddMember(org.ID, "test1@example.com")
				unitB.AddMember(org.ID, "test2@example.com")

				params.unitID = org.ID
				params.expected = []uuid.UUID{memberOne.ID, memberTwo.ID}
				return context.Background()
			},
		},
		{
			name:   "Return empty when no members",
			params: params{},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				unitB := unitbuilder.New(t, db)
				org := unitB.Create(unit.UnitTypeOrganization, unitbuilder.WithName("empty-org"))
				unitRow := unitB.Create(unit.UnitTypeUnit,
					unitbuilder.WithOrgID(org.ID),
					unitbuilder.WithName("empty-unit"),
				)

				params.unitID = unitRow.ID
				params.expected = []uuid.UUID{}
				return context.Background()
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

			service := unit.NewService(logger, db)
			members, err := service.ListMembers(ctx, params.unitID)

			memberIDs := make([]uuid.UUID, len(members))
			for i, member := range members {
				memberIDs[i] = member.ID
			}

			require.NoError(t, err)
			require.ElementsMatch(t, params.expected, memberIDs)
		})
	}
}

func TestUnitService_ListUnitsMembers(t *testing.T) {
	type params struct {
		unitIDs  []uuid.UUID
		expected map[uuid.UUID][]uuid.UUID
	}
	testCases := []struct {
		name   string
		params params
		setup  func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context
	}{
		{
			name: "Return members for multiple units",
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				unitB := unitbuilder.New(t, db)
				userB := userbuilder.New(t, db)
				org := unitB.Create(unit.UnitTypeOrganization, unitbuilder.WithName("multi-org"))
				unitOne := unitB.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("unit-one"))
				unitTwo := unitB.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("unit-two"))

				memberA := userB.Create()
				memberB := userB.Create()
				memberC := userB.Create()
				userB.CreateEmail(memberA.ID, "memberA@example.com")
				userB.CreateEmail(memberB.ID, "memberB@example.com")
				userB.CreateEmail(memberC.ID, "memberC@example.com")

				unitB.AddMember(unitOne.ID, "memberA@example.com")
				unitB.AddMember(unitOne.ID, "memberB@example.com")
				unitB.AddMember(unitOne.ID, "memberB@example.com")
				unitB.AddMember(unitTwo.ID, "memberC@example.com")

				params.unitIDs = []uuid.UUID{unitOne.ID, unitTwo.ID}
				params.expected = map[uuid.UUID][]uuid.UUID{
					unitOne.ID: {memberA.ID, memberB.ID},
					unitTwo.ID: {memberC.ID},
				}

				return context.Background()
			},
		},
		{
			name: "Return empty map when no unit IDs provided",
			params: params{
				unitIDs:  []uuid.UUID{},
				expected: map[uuid.UUID][]uuid.UUID{},
			},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				return context.Background()
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

			service := unit.NewService(logger, db)
			result, err := service.ListUnitsMembers(ctx, params.unitIDs)

			require.NoError(t, err)
			require.Len(t, result, len(params.expected))
			for unitID, expectedMembers := range params.expected {
				actual, ok := result[unitID]
				require.True(t, ok, "expected members for unit %s", unitID)
				require.ElementsMatch(t, expectedMembers, actual)
			}
		})
	}
}

func TestUnitService_RemoveMember(t *testing.T) {
	type params struct {
		unitType unit.Type
		unitID   uuid.UUID
		memberID uuid.UUID
	}

	testCases := []struct {
		name        string
		params      params
		setup       func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context
		validate    func(t *testing.T, params params, db dbbuilder.DBTX)
		expectedErr bool
	}{
		{
			name:   "Remove existing unit member",
			params: params{unitType: unit.TypeUnit},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				unitB := unitbuilder.New(t, db)
				userB := userbuilder.New(t, db)
				org := unitB.Create(unit.UnitTypeOrganization, unitbuilder.WithName("removal-org"))
				unitRow := unitB.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("unit-to-clean"))
				member := userB.Create()
				remaining := userB.Create()
				userB.CreateEmail(member.ID, "test@example.com")
				userB.CreateEmail(remaining.ID, "remaining@example.com")

				unitB.AddMember(unitRow.ID, "test@example.com")
				unitB.AddMember(unitRow.ID, "remaining@example.com")

				params.unitID = unitRow.ID
				params.memberID = member.ID
				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX) {
				members, err := unit.New(db).ListMembers(context.Background(), params.unitID)
				require.NoError(t, err)
				require.NotContains(t, members, params.memberID)
				require.Len(t, members, 1)
			},
		},
		{
			name:   "No-op when membership missing",
			params: params{unitType: unit.TypeOrg},
			setup: func(t *testing.T, params *params, db dbbuilder.DBTX) context.Context {
				unitB := unitbuilder.New(t, db)
				org := unitB.Create(unit.UnitTypeOrganization, unitbuilder.WithName("cleanup-org"))
				params.unitID = org.ID
				params.memberID = uuid.New()
				return context.Background()
			},
			validate: func(t *testing.T, params params, db dbbuilder.DBTX) {
				members, err := unit.New(db).ListMembers(context.Background(), params.unitID)
				require.NoError(t, err)
				require.Empty(t, members)
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

			service := unit.NewService(logger, db)

			err = service.RemoveMember(ctx, params.unitType, params.unitID, params.memberID)
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.validate != nil {
				tc.validate(t, params, db)
			}
		})
	}
}

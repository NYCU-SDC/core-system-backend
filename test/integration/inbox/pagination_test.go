package inbox

import (
	"NYCU-SDC/core-system-backend/internal/inbox"
	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata"
	formbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/form"
	inboxbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/inbox"
	unitbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/unit"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestInboxService_Count tests the count functionality of the inbox service
// It tests the count functionality on non-archived messages only
// Other filters are not tested here because they are tested in the filter_test.go file
func TestInboxService_Count(t *testing.T) {
	type Params struct {
		totalMessages int
		archivedCount int
	}

	testCases := []struct {
		name     string
		params   Params
		expected int64
	}{
		{
			name: "Count with 25 non-archived messages",
			params: Params{
				totalMessages: 30,
				archivedCount: 5,
			},
			expected: 25,
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

			// Setup
			unitBuilder := unitbuilder.New(t, db)
			userBuilder := userbuilder.New(t, db)
			formBuilder := formbuilder.New(t, db)
			inboxBuilder := inboxbuilder.New(t, db)

			org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("pagination-org"))
			unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("pagination-unit"))
			user := userBuilder.Create()
			email := testdata.RandomEmail()
			userBuilder.CreateEmail(user.ID, email)
			unitBuilder.AddMember(unitRow.ID, email)

			// Create messages; archive some so we have non-archived messages
			messageIDs := make([]uuid.UUID, tc.params.totalMessages)
			userInboxMessageIDs := make([]uuid.UUID, tc.params.totalMessages)
			for i := 0; i < tc.params.totalMessages; i++ {
				form := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("msg-"+uuid.NewString()),
				)
				message := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form.ID, unitRow.ID)
				messageIDs[i] = message.ID
				userInboxMessage := inboxBuilder.CreateUserInboxMessage(user.ID, message.ID)
				userInboxMessageIDs[i] = userInboxMessage.ID
				if i < tc.params.archivedCount {
					// archive first N messages
					inboxBuilder.UpdateUserInboxMessage(userInboxMessageIDs[i], user.ID, inbox.UserInboxMessageFilter{IsRead: false, IsStarred: false, IsArchived: true})
				}
			}

			ctx := context.Background()
			service := inbox.NewService(logger, db)

			// Test total count
			total, err := service.Count(ctx, user.ID, nil)
			require.NoError(t, err)
			require.Equal(t, tc.expected, total, "expected total: %v, got: %v", tc.expected, total)
		})
	}
}

// TestInboxService_ListWithPagination tests the pagination functionality of the inbox service
// It tests the pagination functionality on non-archived messages only
// Other filters are not tested here because they are tested in the filter_test.go file
func TestInboxService_ListWithPagination(t *testing.T) {
	type PaginationParams struct {
		pageNumber  int
		description string
	}

	type Params struct {
		pageSize      int
		totalMessages int
		archivedCount int
		expectedIDs   []uuid.UUID
	}

	testCases := []struct {
		name            string
		params          Params
		paginationTests []PaginationParams
	}{
		{
			name: "Pagination with 25 non-archived messages",
			params: Params{
				totalMessages: 30,
				archivedCount: 5,
				pageSize:      10,
			},
			paginationTests: []PaginationParams{
				{
					pageNumber:  1,
					description: "First page with 10 items",
				},
				{
					pageNumber:  2,
					description: "Second page with 10 items",
				},
				{
					pageNumber:  3,
					description: "Third page with remaining 5 items",
				},
				{
					pageNumber:  4,
					description: "Fourth page should be empty",
				},
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

			// Setup
			unitBuilder := unitbuilder.New(t, db)
			userBuilder := userbuilder.New(t, db)
			formBuilder := formbuilder.New(t, db)
			inboxBuilder := inboxbuilder.New(t, db)

			org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("pagination-org"))
			unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("pagination-unit"))
			user := userBuilder.Create()
			email := testdata.RandomEmail()
			userBuilder.CreateEmail(user.ID, email)
			unitBuilder.AddMember(unitRow.ID, email)

			// Create messages; archive some so we have non-archived messages
			messageIDs := make([]uuid.UUID, tc.params.totalMessages)
			userInboxMessageIDs := make([]uuid.UUID, tc.params.totalMessages)
			for i := 0; i < tc.params.totalMessages; i++ {
				form := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("msg-"+uuid.NewString()),
				)
				message := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form.ID, unitRow.ID)
				messageIDs[i] = message.ID
				userInboxMessage := inboxBuilder.CreateUserInboxMessage(user.ID, message.ID)
				userInboxMessageIDs[i] = userInboxMessage.ID
				if i < tc.params.archivedCount {
					// archive first N messages
					inboxBuilder.UpdateUserInboxMessage(userInboxMessageIDs[i], user.ID, inbox.UserInboxMessageFilter{IsRead: false, IsStarred: false, IsArchived: true})
				}
			}

			tc.params.expectedIDs = messageIDs

			ctx := context.Background()
			service := inbox.NewService(logger, db)

			// Test pagination for each test case
			// pt: pagination testcase
			for _, pt := range tc.paginationTests {
				t.Run(pt.description, func(t *testing.T) {
					result, err := service.List(ctx, user.ID, nil, pt.pageNumber, tc.params.pageSize)
					require.NoError(t, err)

					// Calculate expected length based on pagination
					nonArchivedCount := tc.params.totalMessages - tc.params.archivedCount
					startIdx := (pt.pageNumber - 1) * tc.params.pageSize
					endIdx := min(startIdx+tc.params.pageSize, nonArchivedCount)

					expectedLength := max(
						// if expectedLength is negative, means the page is empty
						endIdx-startIdx, 0)

					require.Len(t, result, expectedLength,
						"Page %d with size %d should return %d items",
						pt.pageNumber, tc.params.pageSize, expectedLength)

					// Validate message UUIDs
					actualUUIDs := make([]uuid.UUID, len(result))
					for i, row := range result {
						actualUUIDs[i] = row.MessageID
					}

					// Calculate expected UUIDs based on pagination
					// Get non-archived message IDs (skip first archivedCount messages)
					nonArchivedMessageIDs := tc.params.expectedIDs[tc.params.archivedCount:]

					// Calculate expected slice based on pagination
					var expectedUUIDs []uuid.UUID
					if startIdx < len(nonArchivedMessageIDs) {
						expectedUUIDs = nonArchivedMessageIDs[startIdx:endIdx]
					}

					require.ElementsMatch(t, expectedUUIDs, actualUUIDs,
						"Expected message UUIDs: %v, got: %v", expectedUUIDs, actualUUIDs)
				})
			}
		})
	}
}

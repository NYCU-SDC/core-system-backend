package inbox

import (
	"NYCU-SDC/core-system-backend/internal/inbox"
	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	formbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/form"
	inboxbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/inbox"
	unitbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/unit"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestInboxService_ListWithFilters(t *testing.T) {
	type Params struct {
		userID        uuid.UUID
		filter        *inbox.InboxFilterRequest
		expectedCount int
	}
	testCases := []struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow)
		expectedErr bool
	}{
		{
			name: "No filter should return only non-archived messages (default behavior)",
			params: Params{
				filter:        nil, // No filter
				expectedCount: 2,   // Should return unread and read, but not archived
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("comprehensive-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("comprehensive-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Create three forms with different statuses
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Unread Message"),
				)
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Read Message"),
				)
				form3 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Archived Message"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)
				message3 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form3.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)                      // Unread
				userInboxMessage2 := inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID) // Read
				userInboxMessage3 := inboxBuilder.CreateUserInboxMessage(user.ID, message3.ID) // Archived

				// Mark second message as read
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage2[0].ID, user.ID, inbox.UserInboxMessageFilter{IsRead: true, IsStarred: false, IsArchived: false})
				// Mark third message as archived
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage3[0].ID, user.ID, inbox.UserInboxMessageFilter{IsRead: false, IsStarred: false, IsArchived: true})

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, params.expectedCount)
				for _, msg := range result {
					require.False(t, msg.IsArchived, "archived messages should not appear in default list")
				}
			},
			expectedErr: false,
		},
		{
			name: "Filter by isRead=true should return only read messages",
			params: Params{
				filter: &inbox.InboxFilterRequest{
					IsRead: boolPtr(true),
				},
				expectedCount: 1,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("filter-read-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("filter-read-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Create two forms - one read, one unread
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Read Message"),
				)
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Unread Message"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				userInboxMessage1 := inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				// Mark first message as read
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage1[0].ID, user.ID, inbox.UserInboxMessageFilter{IsRead: true, IsStarred: false, IsArchived: false})

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, params.expectedCount)
				for _, msg := range result {
					require.True(t, msg.IsRead, "all returned messages should be read")
				}
			},
			expectedErr: false,
		},
		{
			name: "Filter by isArchived=true should return only archived messages",
			params: Params{
				filter: &inbox.InboxFilterRequest{
					IsArchived: boolPtr(true),
				},
				expectedCount: 1,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("archived-filter-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("archived-filter-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Create forms with different archive status
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Normal Message"),
				)
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Archived Message"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				userInboxMessage2 := inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				// Mark second message as archived
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage2[0].ID, user.ID, inbox.UserInboxMessageFilter{IsRead: false, IsStarred: false, IsArchived: true})

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, params.expectedCount)
				for _, msg := range result {
					require.True(t, msg.IsArchived, "all returned messages should be archived")
				}
			},
			expectedErr: false,
		},
		{
			name: "Filter by isStarred=true should return only starred messages",
			params: Params{
				filter: &inbox.InboxFilterRequest{
					IsStarred: boolPtr(true),
				},
				expectedCount: 1,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("starred-filter-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("starred-filter-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Create forms with different star status
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Normal Message"),
				)
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Starred Message"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				userInboxMessage2 := inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				// Mark second message as starred
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage2[0].ID, user.ID, inbox.UserInboxMessageFilter{IsRead: false, IsStarred: true, IsArchived: false})

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, params.expectedCount)
				for _, msg := range result {
					require.True(t, msg.IsStarred, "all returned messages should be starred")
				}
			},
			expectedErr: false,
		},
		{
			name: "Combined filters: isRead=true AND isStarred=true should return only read and starred messages",
			params: Params{
				filter: &inbox.InboxFilterRequest{
					IsRead:    boolPtr(true),
					IsStarred: boolPtr(true),
				},
				expectedCount: 1,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("combined-filter-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("combined-filter-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Create forms with different combinations
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Unread Unstarred"),
				)
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Read Unstarred"),
				)
				form3 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Read Starred"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)
				message3 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form3.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)                      // Unread, Unstarred
				userInboxMessage2 := inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID) // Read, Unstarred
				userInboxMessage3 := inboxBuilder.CreateUserInboxMessage(user.ID, message3.ID) // Read, Starred

				// Mark second message as read only
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage2[0].ID, user.ID, inbox.UserInboxMessageFilter{IsRead: true, IsStarred: false, IsArchived: false})
				// Mark third message as read and starred
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage3[0].ID, user.ID, inbox.UserInboxMessageFilter{IsRead: true, IsStarred: true, IsArchived: false})

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, params.expectedCount)
				for _, msg := range result {
					require.True(t, msg.IsRead, "all returned messages should be read")
					require.True(t, msg.IsStarred, "all returned messages should be starred")
				}
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match title field",
			params: Params{
				filter: &inbox.InboxFilterRequest{
					Search: "important",
				},
				expectedCount: 1,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("filter-search-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("filter-search-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Create forms with different titles
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Important Meeting"),
				)
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Regular Update"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, params.expectedCount)
				for _, msg := range result {
					require.Contains(t, msg.Title.(string), "Important", "returned message should contain search term")
				}
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match description field",
			params: Params{
				filter: &inbox.InboxFilterRequest{
					Search: "delta",
				},
				expectedCount: 1,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("filter-search-desc-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("filter-search-desc-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Title does not contain keyword; description does
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Quarterly Report"),
					formbuilder.WithDescription("Includes delta analysis for Q3"),
				)
				// Control form without keyword anywhere
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Weekly Update"),
					formbuilder.WithDescription("Status update and blockers"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, params.expectedCount)
				for _, msg := range result {
					// Matches because of description ILIKE
					require.Contains(t, msg.PreviewMessage.(string), "delta")
				}
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match preview_message field",
			params: Params{
				filter: &inbox.InboxFilterRequest{
					Search: "hotfix",
				},
				expectedCount: 1,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("filter-search-preview-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("filter-search-preview-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Title and description do not contain keyword; preview_message does
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Release Notes"),
					formbuilder.WithDescription("Minor release v1.2.4"),
					formbuilder.WithPreviewMessage("hotfix deployed to production"),
				)
				// Control form without keyword anywhere
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Changelog"),
					formbuilder.WithDescription("features and improvements"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, params.expectedCount)
				for _, msg := range result {
					require.Contains(t, msg.PreviewMessage.(string), "hotfix")
				}
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match across title, description, and preview_message",
			params: Params{
				filter: &inbox.InboxFilterRequest{
					Search: "signal",
				},
				expectedCount: 3,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("filter-search-allfields-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("filter-search-allfields-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// One form with keyword in title
				formTitle := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("signal update"),
					formbuilder.WithDescription("weekly report"),
				)
				// One form with keyword at start of description (so preview picks it up if needed)
				formDesc := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("metrics report"),
					formbuilder.WithDescription("signal metrics and notes"),
				)
				// One form with keyword in preview_message
				formPrev := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("release notes"),
					formbuilder.WithDescription("hotfix v1.2.5"),
					formbuilder.WithPreviewMessage("signal deployed to prod"),
				)

				msg1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formTitle.ID, unitRow.ID)
				msg2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formDesc.ID, unitRow.ID)
				msg3 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formPrev.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, msg1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, msg2.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, msg3.ID)

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, params.expectedCount)
				for _, msg := range result {
					// The search matches any of title, description (reflected in preview when no preview set), or preview_message
					title := ""
					if msg.Title != nil {
						title = msg.Title.(string)
					}
					preview := ""
					if msg.PreviewMessage != nil {
						preview = msg.PreviewMessage.(string)
					}
					require.True(t, (title != "" && (title == "signal update" || title == "metrics report" || title == "release notes")) ||
						(preview != "" && (preview == "signal metrics and notes"[:24] || preview == "signal deployed to prod")),
						"each result should have 'signal' in title, description (via preview), or preview_message")
				}
			},
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
				ctx = tc.setup(t, &params, db, logger)
			}

			service := inbox.NewService(logger, db)

			result, err := service.List(ctx, params.userID, params.filter, 1, 200)
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.validate != nil {
				tc.validate(t, params, db, result)
			}
		})
	}
}

func TestInboxService_ListPagination(t *testing.T) {
	type PaginationParams struct {
		pageNumber    int
		pageSize      int
		expectedCount int
		description   string
	}

	type Params struct {
		userID          uuid.UUID
		totalMessages   int
		archivedCount   int
		expectedTotal   int64
		paginationTests []PaginationParams
	}

	testCases := []struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, service *inbox.Service, ctx context.Context, userID uuid.UUID)
		expectedErr bool
	}{
		{
			name: "Pagination with 25 non-archived messages",
			params: Params{
				totalMessages: 30,
				archivedCount: 5,
				expectedTotal: 25,
				paginationTests: []PaginationParams{
					{
						pageNumber:    1,
						pageSize:      10,
						expectedCount: 10,
						description:   "First page with 10 items",
					},
					{
						pageNumber:    2,
						pageSize:      10,
						expectedCount: 10,
						description:   "Second page with 10 items",
					},
					{
						pageNumber:    3,
						pageSize:      10,
						expectedCount: 5,
						description:   "Third page with remaining 5 items",
					},
					{
						pageNumber:    4,
						pageSize:      10,
						expectedCount: 0,
						description:   "Fourth page should be empty",
					},
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
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
				for i := 0; i < params.totalMessages; i++ {
					frm := formBuilder.Create(
						formbuilder.WithUnitID(unitRow.ID),
						formbuilder.WithLastEditor(user.ID),
						formbuilder.WithTitle("msg-"+uuid.NewString()),
					)
					msg := inboxBuilder.CreateMessage(inbox.ContentTypeForm, frm.ID, unitRow.ID)
					uim := inboxBuilder.CreateUserInboxMessage(user.ID, msg.ID)
					if i < params.archivedCount {
						// archive first N messages
						inboxBuilder.UpdateUserInboxMessage(uim[0].ID, user.ID, inbox.UserInboxMessageFilter{IsRead: false, IsStarred: false, IsArchived: true})
					}
				}

				// Store userID in params for use in validation
				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, service *inbox.Service, ctx context.Context, userID uuid.UUID) {
				// Test total count
				total, err := service.Count(ctx, userID, nil)
				require.NoError(t, err)
				require.Equal(t, params.expectedTotal, total)

				// Test pagination for each test case
				for _, paginationTest := range params.paginationTests {
					t.Run(paginationTest.description, func(t *testing.T) {
						result, err := service.List(ctx, userID, nil, paginationTest.pageNumber, paginationTest.pageSize)
						require.NoError(t, err)
						require.Len(t, result, paginationTest.expectedCount,
							"Page %d with size %d should return %d items",
							paginationTest.pageNumber, paginationTest.pageSize, paginationTest.expectedCount)
					})
				}
			},
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
				ctx = tc.setup(t, &params, db, logger)
			}

			service := inbox.NewService(logger, db)

			if tc.validate != nil {
				tc.validate(t, params, db, service, ctx, params.userID)
			}
		})
	}
}

// Helper function to create bool pointer
func boolPtr(b bool) *bool {
	return &b
}

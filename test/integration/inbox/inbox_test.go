package inbox

import (
	"NYCU-SDC/core-system-backend/internal/inbox"
	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	formbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/form"
	inboxbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/inbox"
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

func TestInboxService_Create(t *testing.T) {
	type Params struct {
		contentType    inbox.ContentType
		contentID      uuid.UUID
		recipients     []uuid.UUID
		unitID         uuid.UUID
		title          string
		previewMessage string
	}
	testCases := []struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, result uuid.UUID)
		expectedErr bool
	}{
		{
			name: "Create user inbox message for multiple users",
			params: Params{
				contentType:    inbox.ContentTypeForm,
				title:          "test-title",
				previewMessage: "test-preview-message",
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("test-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("test-unit"))

				userA := userBuilder.Create()
				userB := userBuilder.Create()
				userBuilder.CreateEmail(userA.ID, "userA@example.com")
				userBuilder.CreateEmail(userB.ID, "userB@example.com")

				unitBuilder.AddMember(unitRow.ID, "userA@example.com")
				unitBuilder.AddMember(unitRow.ID, "userB@example.com")

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(userA.ID),
					formbuilder.WithTitle(params.title),
					formbuilder.WithPreviewMessage(params.previewMessage),
				)

				params.contentID = formRow.ID
				params.recipients = []uuid.UUID{userA.ID, userB.ID}
				params.unitID = unitRow.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result uuid.UUID) {
				require.NotEqual(t, uuid.Nil, result)

				inboxBuilder := inboxbuilder.New(t, db)
				for _, recipientID := range params.recipients {
					rows := inboxBuilder.GetUserInboxMessages(recipientID)
					require.Equal(t, 1, len(rows))

					found := false
					for _, r := range rows {
						if r.ContentID == params.contentID {
							found = true
							require.Equal(t, params.contentType, r.Type)
							require.Equal(t, params.title, r.Title)
							require.Equal(t, params.previewMessage, r.PreviewMessage)
							require.Equal(t, "test-org", r.OrgName)
							require.Equal(t, "test-unit", r.UnitName)
							break
						}
					}
					require.True(t, found, "message not found for recipient %s", recipientID)
				}
			},
			expectedErr: false,
		},
		{
			name: "Fail when user ID does not exist in recipients for user inbox",
			params: Params{
				contentType:    inbox.ContentTypeForm,
				title:          "test-title",
				previewMessage: "test-preview-message",
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("invalid-user-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("invalid-user-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")

				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle(params.title),
					formbuilder.WithPreviewMessage(params.previewMessage),
				)

				params.contentID = formRow.ID
				params.recipients = []uuid.UUID{uuid.New()} // Non-existent user ID
				params.unitID = unitRow.ID

				return context.Background()
			},
			expectedErr: true,
		},
		{
			name: "Fail when unit ID does not exist in user inbox",
			params: Params{
				contentType:    inbox.ContentTypeForm,
				title:          "test-title",
				previewMessage: "test-preview-message",
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)

				user := userBuilder.Create()

				// Create a valid form first
				unitBuilder := unitbuilder.New(t, db)
				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("temp-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("temp-unit"))

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle(params.title),
					formbuilder.WithPreviewMessage(params.previewMessage),
				)

				params.contentID = formRow.ID
				params.recipients = []uuid.UUID{user.ID}
				params.unitID = uuid.New() // Invalid unit ID for posted_by

				return context.Background()
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
				ctx = tc.setup(t, &params, db, logger)
			}

			service := inbox.NewService(logger, db)

			result, err := service.Create(ctx, params.contentType, params.contentID, params.recipients, params.unitID)
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.validate != nil {
				tc.validate(t, params, db, result)
			}
		})
	}
}

func TestInboxService_List(t *testing.T) {
	type Params struct {
		userID   uuid.UUID
		expected int
	}
	testCases := []struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow)
		expectedErr bool
	}{
		{
			name: "Return empty list when no user inbox messages exist",
			params: Params{
				expected: 0,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("empty-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("empty-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")

				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Empty(t, result)
			},
			expectedErr: false,
		},
		{
			name: "Return user inbox messages when they exist",
			params: Params{
				expected: 1,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("message-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("message-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")

				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("message-title"),
					formbuilder.WithPreviewMessage("message-preview"),
				)

				message := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formRow.ID, unitRow.ID)
				userInboxMessages := inboxBuilder.CreateUserInboxMessage(user.ID, message.ID)
				require.Equal(t, 1, len(userInboxMessages))

				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, params.expected)
				for _, msg := range result {
					require.Equal(t, params.userID, msg.UserID)
					require.Equal(t, "message-title", msg.Title)
					require.Equal(t, "message-preview", msg.PreviewMessage)
					require.Equal(t, "message-org", msg.OrgName)
					require.Equal(t, "message-unit", msg.UnitName)
				}
			},
			expectedErr: false,
		},
		{
			name: "Return empty list when user ID does not exist in user inbox",
			params: Params{
				expected: 0,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				// Use a non-existent user ID
				params.userID = uuid.New()

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Empty(t, result)
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

			result, err := service.List(ctx, params.userID)
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.validate != nil {
				tc.validate(t, params, db, result)
			}
		})
	}
}

func TestInboxService_UpdateByID(t *testing.T) {
	type Params struct {
		messageID uuid.UUID
		userID    uuid.UUID
		expected  inbox.UserInboxMessageFilter
	}
	testCases := []struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, result inbox.UpdateByIDRow)
		expectedErr bool
	}{
		{
			name: "Mark user inbox message as read and starred",
			params: Params{
				expected: inbox.UserInboxMessageFilter{IsRead: true, IsStarred: true, IsArchived: false},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("update-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("update-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")

				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
				)

				message := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formRow.ID, unitRow.ID)
				userInboxMessage := inboxBuilder.CreateUserInboxMessage(user.ID, message.ID)
				require.Equal(t, 1, len(userInboxMessage))

				params.messageID = userInboxMessage[0].ID
				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result inbox.UpdateByIDRow) {
				require.Equal(t, params.expected.IsRead, result.IsRead)
				require.Equal(t, params.expected.IsStarred, result.IsStarred)
				require.Equal(t, params.expected.IsArchived, result.IsArchived)
			},
			expectedErr: false,
		},
		{
			name: "Archive user inbox message",
			params: Params{
				expected: inbox.UserInboxMessageFilter{IsRead: false, IsStarred: false, IsArchived: true},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("archive-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("archive-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")

				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
				)

				message := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formRow.ID, unitRow.ID)
				userInboxMessage := inboxBuilder.CreateUserInboxMessage(user.ID, message.ID)
				require.Equal(t, 1, len(userInboxMessage))

				params.messageID = userInboxMessage[0].ID
				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result inbox.UpdateByIDRow) {
				require.Equal(t, params.expected.IsRead, result.IsRead)
				require.Equal(t, params.expected.IsStarred, result.IsStarred)
				require.Equal(t, params.expected.IsArchived, result.IsArchived)
			},
			expectedErr: false,
		},
		{
			name: "Unstar and read user inbox message",
			params: Params{
				expected: inbox.UserInboxMessageFilter{IsRead: true, IsStarred: false, IsArchived: false},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("unstar-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("unstar-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")

				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
				)

				// Create inbox message
				message := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formRow.ID, unitRow.ID)
				userInboxMessage := inboxBuilder.CreateUserInboxMessage(user.ID, message.ID)
				require.Equal(t, 1, len(userInboxMessage))
				params.messageID = userInboxMessage[0].ID
				params.userID = user.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result inbox.UpdateByIDRow) {
				require.Equal(t, params.expected.IsRead, result.IsRead)
				require.Equal(t, params.expected.IsStarred, result.IsStarred)
				require.Equal(t, params.expected.IsArchived, result.IsArchived)
			},
			expectedErr: false,
		},
		{
			name: "Fail when message ID does not exist in user inbox",
			params: Params{
				expected: inbox.UserInboxMessageFilter{IsRead: true, IsStarred: false, IsArchived: false},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("invalid-message-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("invalid-message-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")
				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				// Use a non-existent message ID
				params.messageID = uuid.New()
				params.userID = user.ID

				return context.Background()
			},
			expectedErr: true,
		},
		{
			name: "Fail when user ID does not exist in user inbox",
			params: Params{
				expected: inbox.UserInboxMessageFilter{IsRead: true, IsStarred: false, IsArchived: false},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("invalid-user-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("invalid-user-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")

				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
				)

				// Create inbox message
				message := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formRow.ID, unitRow.ID)
				userInboxMessage := inboxBuilder.CreateUserInboxMessage(user.ID, message.ID)
				require.Equal(t, 1, len(userInboxMessage))
				params.messageID = userInboxMessage[0].ID
				params.userID = uuid.New() // Non-existent user ID

				return context.Background()
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
				ctx = tc.setup(t, &params, db, logger)
			}

			service := inbox.NewService(logger, db)

			result, err := service.UpdateByID(ctx, params.messageID, params.userID, params.expected)
			require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)

			if tc.validate != nil {
				tc.validate(t, params, db, result)
			}
		})
	}
}

func TestInboxService_DuplicateCreatesProduceMultipleMessages(t *testing.T) {
	type Params struct {
		contentID uuid.UUID
		userID    uuid.UUID
		unitID    uuid.UUID
		expected  int
	}
	testCases := []struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, results []uuid.UUID)
		expectedErr bool
	}{
		{
			name: "Create multiple user inbox messages for same content and recipient",
			params: Params{
				expected: 2,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("duplicate-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("duplicate-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")

				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
				)

				params.contentID = formRow.ID
				params.userID = user.ID
				params.unitID = unitRow.ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, results []uuid.UUID) {
				require.Len(t, results, params.expected)
				require.NotEqual(t, results[0], results[1], "message IDs should be different")

				inboxBuilder := inboxbuilder.New(t, db)
				rows := inboxBuilder.GetUserInboxMessages(params.userID)

				count := 0
				for _, r := range rows {
					if r.ContentID == params.contentID {
						count++
					}
				}
				require.GreaterOrEqual(t, count, params.expected, "expect multiple messages for repeated Create calls")
			},
			expectedErr: false,
		},
		{
			name: "Fail when trying to create with invalid user ID in user inbox",
			params: Params{
				expected: 0,
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("invalid-duplicate-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("invalid-duplicate-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")

				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
				)

				params.contentID = formRow.ID
				params.userID = uuid.New() // Non-existent user ID
				params.unitID = unitRow.ID

				return context.Background()
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
				ctx = tc.setup(t, &params, db, logger)
			}

			service := inbox.NewService(logger, db)

			// Create multiple messages with same content/recipient intentionally
			results := make([]uuid.UUID, params.expected)
			for i := 0; i < params.expected; i++ {
				result, err := service.Create(ctx, inbox.ContentTypeForm, params.contentID, []uuid.UUID{params.userID}, params.unitID)
				require.Equal(t, tc.expectedErr, err != nil, "expected error: %v, got: %v", tc.expectedErr, err)
				require.NotEqual(t, uuid.Nil, result)
				results[i] = result
			}

			if tc.validate != nil {
				tc.validate(t, params, db, results)
			}
		})
	}
}

func TestInboxService_ArchiveVisibilityInList(t *testing.T) {
	type Params struct {
		userID    uuid.UUID
		messageID uuid.UUID
	}
	testCases := []struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow)
		expectedErr bool
	}{
		{
			name: "Archived user inbox messages should not appear in List",
			params: Params{
				messageID: uuid.New(),
				userID:    uuid.New(),
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger interface{}) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("archive-visibility-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("archive-visibility-unit"))
				user := userBuilder.Create()
				userBuilder.CreateEmail(user.ID, "user@example.com")

				unitBuilder.AddMember(unitRow.ID, "user@example.com")

				formRow := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
				)

				// Create inbox message
				message := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formRow.ID, unitRow.ID)
				userInboxMessage := inboxBuilder.CreateUserInboxMessage(user.ID, message.ID)
				require.Equal(t, 1, len(userInboxMessage))
				params.userID = user.ID
				params.messageID = userInboxMessage[0].ID

				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result []inbox.ListRow) {
				require.Len(t, result, 1)
				inboxBuilder := inboxbuilder.New(t, db)

				// Archive the message
				inboxBuilder.UpdateUserInboxMessage(params.messageID, params.userID, inbox.UserInboxMessageFilter{IsRead: true, IsStarred: false, IsArchived: true})

				// Service List (no filters) should not return archived messages
				rows := inboxBuilder.GetUserInboxMessages(params.userID)

				found := false
				for _, r := range rows {
					if r.ID == params.messageID {
						found = true
						require.True(t, r.IsArchived)
						break
					}
				}
				require.False(t, found, "archived message should not appear in List")
			},
			expectedErr: false,
		},
	}

	resourceManager, _, err := integration.GetOrInitResource()
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

			params := tc.params
			if tc.setup != nil {
				tc.setup(t, &params, db, nil)
			}

			inboxBuilder := inboxbuilder.New(t, db)
			results := inboxBuilder.GetUserInboxMessages(params.userID)

			if tc.validate != nil {
				tc.validate(t, params, db, results)
			}
		})
	}
}

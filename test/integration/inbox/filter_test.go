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
		userID             uuid.UUID
		filter             *inbox.FilterRequest
		expectedMessageIDs []uuid.UUID
	}
	testCases := []struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context
		expectedErr bool
	}{
		{
			name: "No filter should return only non-archived messages (default behavior)",
			params: Params{
				filter: nil, // No filter
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
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
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage2.ID, user.ID, inbox.UserInboxMessageFilter{IsRead: true, IsStarred: false, IsArchived: false})
				// Mark third message as archived
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage3.ID, user.ID, inbox.UserInboxMessageFilter{IsRead: false, IsStarred: false, IsArchived: true})

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message1.ID, message2.ID} // Unarchived messages

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by isRead=true should return only read messages",
			params: Params{
				filter: &inbox.FilterRequest{
					IsRead: new(true),
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
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
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage1.ID, user.ID, inbox.UserInboxMessageFilter{IsRead: true, IsStarred: false, IsArchived: false})

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // Read messages

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by isArchived=true should return only archived messages",
			params: Params{
				filter: &inbox.FilterRequest{
					IsArchived: new(true),
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
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
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage2.ID, user.ID, inbox.UserInboxMessageFilter{IsRead: false, IsStarred: false, IsArchived: true})

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message2.ID} // Archived messages

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by isStarred=true should return only starred messages",
			params: Params{
				filter: &inbox.FilterRequest{
					IsStarred: new(true),
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
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
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage2.ID, user.ID, inbox.UserInboxMessageFilter{IsRead: false, IsStarred: true, IsArchived: false})

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message2.ID} // Starred messages

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Combined filters: isRead=true AND isStarred=true should return only read and starred messages",
			params: Params{
				filter: &inbox.FilterRequest{
					IsRead:    new(true),
					IsStarred: new(true),
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
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
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage2.ID, user.ID, inbox.UserInboxMessageFilter{IsRead: true, IsStarred: false, IsArchived: false})
				// Mark third message as read and starred
				inboxBuilder.UpdateUserInboxMessage(userInboxMessage3.ID, user.ID, inbox.UserInboxMessageFilter{IsRead: true, IsStarred: true, IsArchived: false})

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message3.ID} // Read and starred messages

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match title field",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "important",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
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
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // with "important" in title

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match description field",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "delta",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
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
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // with "delta" in description

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match preview_message field",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "hotfix",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
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
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // with "hotfix" in preview_message

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match across title, description, and preview_message",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "signal",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
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
				params.expectedMessageIDs = []uuid.UUID{msg1.ID, msg2.ID, msg3.ID} // with "signal" in title, description, and preview_message

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match Chinese title field",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "重要",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("chinese-search-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("chinese-search-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Create forms with Chinese titles
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("重要會議通知"),
				)
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("一般更新"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // with "重要" in title

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match Chinese description field",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "報告",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("chinese-search-desc-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("chinese-search-desc-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Title does not contain keyword; description does
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("季度總結"),
					formbuilder.WithDescription("包含第三季度的報告分析"),
				)
				// Control form without keyword anywhere
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("週報更新"),
					formbuilder.WithDescription("狀態更新和阻礙事項"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // with "報告" in description

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match Chinese preview_message field",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "修復",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("chinese-search-preview-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("chinese-search-preview-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Title and description do not contain keyword; preview_message does
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("發布說明"),
					formbuilder.WithDescription("小版本 v1.2.4"),
					formbuilder.WithPreviewMessage("修復已部署到生產環境"),
				)
				// Control form without keyword anywhere
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("變更日誌"),
					formbuilder.WithDescription("功能和改進"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // with "修復" in preview_message

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match across Chinese title, description, and preview_message",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "系統",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("chinese-search-allfields-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("chinese-search-allfields-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// One form with keyword in title
				formTitle := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("系統更新"),
					formbuilder.WithDescription("週報"),
				)
				// One form with keyword at start of description (so preview picks it up if needed)
				formDesc := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("指標報告"),
					formbuilder.WithDescription("系統指標和註記"),
				)
				// One form with keyword in preview_message
				formPrev := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("發布說明"),
					formbuilder.WithDescription("修復 v1.2.5"),
					formbuilder.WithPreviewMessage("系統已部署到生產環境"),
				)

				msg1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formTitle.ID, unitRow.ID)
				msg2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formDesc.ID, unitRow.ID)
				msg3 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formPrev.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, msg1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, msg2.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, msg3.ID)

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{msg1.ID, msg2.ID, msg3.ID} // with "系統" in title, description, and preview_message

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should handle mixed Chinese and English text",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "API",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("mixed-language-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("mixed-language-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Form with mixed Chinese and English
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("API 文檔更新"),
					formbuilder.WithDescription("新的 API 端點說明"),
				)
				// Form with English only
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("API Documentation"),
					formbuilder.WithDescription("New API endpoints"),
				)
				// Control form without keyword
				form3 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("系統維護"),
					formbuilder.WithDescription("定期維護通知"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)
				message3 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form3.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message3.ID)

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message1.ID, message2.ID} // with "API" in title and description

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match special symbols in title field",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "@gmail.com",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("special-symbols-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("special-symbols-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Create forms with special symbols in titles
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Security Alert @gmail.com"),
				)
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Regular System Update"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // with "@gmail.com" in title

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match special symbols in description field",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "(測試)",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("special-symbols-desc-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("special-symbols-desc-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Title does not contain keyword; description does
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("系統效能分析報告"),
					formbuilder.WithDescription("系統狀態：(測試) 符號在資料中出現異常"),
				)
				// Control form without keyword anywhere
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("週報更新"),
					formbuilder.WithDescription("狀態更新和阻礙事項"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // with "(測試)" in description

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match special symbols in preview_message field",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "!@#$",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("special-symbols-preview-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("special-symbols-preview-unit"))
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
					formbuilder.WithPreviewMessage("Critical fix deployed !@#$ symbols resolved"),
				)
				// Control form without keyword anywhere
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Changelog"),
					formbuilder.WithDescription("Features and improvements"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // with "!@#$" in preview_message

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should match across special symbols in title, description, and preview_message",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "#!/bin/bash",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("special-symbols-allfields-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("special-symbols-allfields-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// One form with keyword in title
				formTitle := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("System #!/bin/bash Update"),
					formbuilder.WithDescription("Weekly report"),
				)
				// One form with keyword at start of description (so preview picks it up if needed)
				formDesc := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Metrics Report"),
					formbuilder.WithDescription("#!/bin/bash echo 'test'"),
				)
				// One form with keyword in preview_message
				formPrev := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Release Notes"),
					formbuilder.WithDescription("Hotfix v1.2.5"),
					formbuilder.WithPreviewMessage("System #!/bin/bash deployed to production"),
				)

				msg1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formTitle.ID, unitRow.ID)
				msg2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formDesc.ID, unitRow.ID)
				msg3 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, formPrev.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, msg1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, msg2.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, msg3.ID)

				params.userID = user.ID

				params.expectedMessageIDs = []uuid.UUID{msg1.ID, msg2.ID, msg3.ID} // with "#!/bin/bash" in title, description, and preview_message

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should handle mixed special symbols and regular text",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "++",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("mixed-symbols-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("mixed-symbols-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Form with special symbols and regular text
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("API Documentation ++"),
					formbuilder.WithDescription("New API endpoints ++"),
				)
				// Form with special symbols only
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("System Alert +++"),
					formbuilder.WithDescription("Critical issues +++"),
				)
				// Control form without keyword
				form3 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Regular Update"),
					formbuilder.WithDescription("Normal maintenance"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)
				message3 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form3.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message3.ID)

				params.userID = user.ID

				params.expectedMessageIDs = []uuid.UUID{message1.ID, message2.ID} // with "++" in title and description

				return context.Background()
			},
			expectedErr: false,
		},
		{
			name: "Filter by search should handle SQL injection attempts with special symbols",
			params: Params{
				filter: &inbox.FilterRequest{
					Search: "'; DROP TABLE",
				},
			},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX, logger any) context.Context {
				unitBuilder := unitbuilder.New(t, db)
				userBuilder := userbuilder.New(t, db)
				formBuilder := formbuilder.New(t, db)
				inboxBuilder := inboxbuilder.New(t, db)

				org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName("sql-injection-org"))
				unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName("sql-injection-unit"))
				user := userBuilder.Create()

				email := testdata.RandomEmail()
				userBuilder.CreateEmail(user.ID, email)
				unitBuilder.AddMember(unitRow.ID, email)

				// Form with SQL injection attempt in title
				form1 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Security Test '; DROP TABLE users; --"),
				)
				// Control form without keyword
				form2 := formBuilder.Create(
					formbuilder.WithUnitID(unitRow.ID),
					formbuilder.WithLastEditor(user.ID),
					formbuilder.WithTitle("Normal Security Alert"),
					formbuilder.WithDescription("Regular security update"),
				)

				message1 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form1.ID, unitRow.ID)
				message2 := inboxBuilder.CreateMessage(inbox.ContentTypeForm, form2.ID, unitRow.ID)

				inboxBuilder.CreateUserInboxMessage(user.ID, message1.ID)
				inboxBuilder.CreateUserInboxMessage(user.ID, message2.ID)

				params.userID = user.ID
				params.expectedMessageIDs = []uuid.UUID{message1.ID} // with SQL injection attempt in title

				return context.Background()
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

			// Extract MessageIDs from the result for comparison
			resultMessageIDs := make([]uuid.UUID, len(result))
			for i, row := range result {
				resultMessageIDs[i] = row.MessageID
			}

			require.Len(t, resultMessageIDs, len(params.expectedMessageIDs), "expected: %v, got: %v", params.expectedMessageIDs, resultMessageIDs)
			require.ElementsMatch(t, params.expectedMessageIDs, resultMessageIDs, "expected: %v, got: %v", params.expectedMessageIDs, resultMessageIDs)
		})
	}
}

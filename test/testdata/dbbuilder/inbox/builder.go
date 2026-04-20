package inboxbuilder

import (
	"context"

	"testing"

	"NYCU-SDC/core-system-backend/internal/inbox"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"

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

func (b Builder) Queries() *inbox.Queries {
	return inbox.New(b.db)
}

// CreateMessage creates an inbox message directly in the database
func (b Builder) CreateMessage(contentType inbox.ContentType, contentID, postedBy uuid.UUID) inbox.InboxMessage {
	queries := b.Queries()
	message, err := queries.CreateMessage(context.Background(), inbox.CreateMessageParams{
		PostedBy:  postedBy,
		Type:      contentType,
		ContentID: contentID,
	})
	require.NoError(b.t, err)
	return message
}

// CreateUserInboxMessage creates a user inbox message directly in the database
func (b Builder) CreateUserInboxMessage(userID, messageID uuid.UUID) inbox.UserInboxMessage {
	queries := b.Queries()
	messages, err := queries.CreateUserInboxBulk(context.Background(), inbox.CreateUserInboxBulkParams{
		UserIds:   []uuid.UUID{userID},
		MessageID: messageID,
	})
	require.NoError(b.t, err)
	require.Len(b.t, messages, 1)
	return messages[len(messages)-1]
}

// CreateUserInboxBulk creates multiple user inbox messages directly in the database
func (b Builder) CreateUserInboxBulk(userIDs []uuid.UUID, messageID uuid.UUID) []inbox.UserInboxMessage {
	queries := b.Queries()
	messages, err := queries.CreateUserInboxBulk(context.Background(), inbox.CreateUserInboxBulkParams{
		UserIds:   userIDs,
		MessageID: messageID,
	})
	require.NoError(b.t, err)
	return messages
}

// GetUserInboxMessages retrieves user inbox messages directly from the database
func (b Builder) GetUserInboxMessages(userID uuid.UUID) []inbox.ListRow {
	queries := b.Queries()
	params := inbox.ListParams{
		UserID:     userID,
		PageLimit:  10,
		PageOffset: 0,
	}
	messages, err := queries.List(context.Background(), params)
	require.NoError(b.t, err)
	return messages
}

// GetUserInboxMessage retrieves a specific user inbox message directly from the database
func (b Builder) GetUserInboxMessage(messageID, userID uuid.UUID) inbox.GetRow {
	queries := b.Queries()
	message, err := queries.Get(context.Background(), inbox.GetParams{
		UserInboxMessageID: messageID,
		UserID:             userID,
	})
	require.NoError(b.t, err)
	return message
}

// UpdateUserInboxMessage updates a user inbox message directly in the database
func (b Builder) UpdateUserInboxMessage(messageID, userID uuid.UUID, filter inbox.UserInboxMessageFilter) inbox.UpdateRow {
	queries := b.Queries()
	message, err := queries.Update(context.Background(), inbox.UpdateParams{
		ID:         messageID,
		UserID:     userID,
		IsRead:     filter.IsRead,
		IsStarred:  filter.IsStarred,
		IsArchived: filter.IsArchived,
	})
	require.NoError(b.t, err)
	return message
}

// CountUserInboxMessages counts user inbox messages with optional filters
func (b Builder) CountUserInboxMessages(userID uuid.UUID, filter *inbox.FilterRequest) int64 {
	queries := b.Queries()
	params := inbox.ListCountParams{
		UserID:     userID,
		IsRead:     pgtype.Bool{Valid: false},
		IsStarred:  pgtype.Bool{Valid: false},
		IsArchived: pgtype.Bool{Valid: false},
		Search:     "",
	}

	if filter != nil {
		if filter.IsRead != nil {
			params.IsRead = pgtype.Bool{Bool: *filter.IsRead, Valid: true}
		}
		if filter.IsStarred != nil {
			params.IsStarred = pgtype.Bool{Bool: *filter.IsStarred, Valid: true}
		}
		if filter.IsArchived != nil {
			params.IsArchived = pgtype.Bool{Bool: *filter.IsArchived, Valid: true}
		}
		if filter.Search != "" {
			params.Search = filter.Search
		}
	}

	count, err := queries.ListCount(context.Background(), params)
	require.NoError(b.t, err)
	return count
}

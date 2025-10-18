package inbox

import (
	"context"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	CreateMessage(ctx context.Context, arg CreateMessageParams) (InboxMessage, error)
	CreateUserInboxBulk(ctx context.Context, arg CreateUserInboxBulkParams) ([]UserInboxMessage, error)
	List(ctx context.Context, arg ListParams) ([]ListRow, error)
	ListCount(ctx context.Context, arg ListCountParams) (int64, error)
	GetByID(ctx context.Context, arg GetByIDParams) (GetByIDRow, error)
	UpdateByID(ctx context.Context, arg UpdateByIDParams) (UpdateByIDRow, error)
}

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer
}

func NewService(logger *zap.Logger, db DBTX) *Service {
	return &Service{
		logger:  logger,
		queries: New(db),
		tracer:  otel.Tracer("inbox/service"),
	}
}

// Create registers a new inbox message and delivers it to the given set of users.
//
// The purpose of this function is to provide a single entry point for creating
// a message entity and ensuring it is visible in the inbox of all target users.
// On success, it returns the unique identifier of the created message.
func (s *Service) Create(ctx context.Context, contentType ContentType, contentID uuid.UUID, userIDs []uuid.UUID, postByUnitID uuid.UUID) (uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	message, err := s.queries.CreateMessage(traceCtx, CreateMessageParams{
		Type:      contentType,
		ContentID: contentID,
		PostedBy:  postByUnitID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create inbox message")
		span.RecordError(err)
		return uuid.Nil, err
	}

	_, err = s.queries.CreateUserInboxBulk(traceCtx, CreateUserInboxBulkParams{
		UserIds:   userIDs,
		MessageID: message.ID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create user inbox messages in bulk")
		span.RecordError(err)
		return uuid.Nil, err
	}

	logger.Info("Created inbox message",
		zap.String("message_id", message.ID.String()),
		zap.Int("recipients", len(userIDs)),
	)

	return message.ID, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, filter *InboxFilterRequest, page int, size int) ([]ListRow, error) {
	traceCtx, span := s.tracer.Start(ctx, "List")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Prepare filter parameters
	params := ListParams{
		UserID:     userID,
		IsRead:     pgtype.Bool{Valid: false},
		IsStarred:  pgtype.Bool{Valid: false},
		IsArchived: pgtype.Bool{Valid: false},
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

	// Apply pagination
	if size > 0 {
		params.PageLimit = int32(size)
	}
	if page > 0 && size > 0 {
		offset := (page - 1) * size
		params.PageOffset = int32(offset)
	}

	messages, err := s.queries.List(traceCtx, params)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list all user inbox messages")
		span.RecordError(err)
		return nil, err
	}

	if messages == nil {
		return []ListRow{}, err
	}

	return messages, err
}

func (s *Service) Count(ctx context.Context, userID uuid.UUID, filter *InboxFilterRequest) (int64, error) {
	traceCtx, span := s.tracer.Start(ctx, "Count")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	params := ListCountParams{
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

	total, err := s.queries.ListCount(traceCtx, params)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "count user inbox messages")
		span.RecordError(err)
		return 0, err
	}

	return total, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (GetByIDRow, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	message, err := s.queries.GetByID(traceCtx, GetByIDParams{
		UserInboxMessageID: id,
		UserID:             userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get the full inbox_message by id")
		span.RecordError(err)
		return GetByIDRow{}, err
	}

	return message, err
}

func (s *Service) UpdateByID(ctx context.Context, id uuid.UUID, userID uuid.UUID, arg UserInboxMessageFilter) (UpdateByIDRow, error) {
	traceCtx, span := s.tracer.Start(ctx, "UpdateByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	message, err := s.queries.UpdateByID(traceCtx, UpdateByIDParams{
		ID:         id,
		UserID:     userID,
		IsRead:     arg.IsRead,
		IsArchived: arg.IsArchived,
		IsStarred:  arg.IsStarred,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "update user_inbox_message by id")
		span.RecordError(err)
		return UpdateByIDRow{}, err
	}

	return message, err
}

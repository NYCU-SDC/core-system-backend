package internal

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type contextKey string

var (
	UserContextKey    contextKey = "user"
	OrgIDContextKey   contextKey = "org-id"
	OrgSlugContextKey contextKey = "org-slug"
	DBConnectionKey   contextKey = "database-connection"
)

type DBTX interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

// TxBeginner can start a new database transaction (e.g. *pgxpool.Pool).
type TxBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

func ParseUUID(value string) (uuid.UUID, error) {
	parsedUUID, err := uuid.Parse(value)
	if err != nil {
		return parsedUUID, fmt.Errorf("failed to parse UUID: %w", err)
	}

	return parsedUUID, nil
}

func GetDBTXFromContext(ctx context.Context) (DBTX, error) {
	conn, ok := ctx.Value(DBConnectionKey).(DBTX)
	if !ok {
		return nil, fmt.Errorf("database connection not found in context")
	}
	return conn, nil
}

func GetSlugFromContext(ctx context.Context) (string, error) {
	orgSlug, ok := ctx.Value(OrgSlugContextKey).(string)
	if !ok {
		return "", fmt.Errorf("organization slug not found in context")
	}
	return orgSlug, nil
}

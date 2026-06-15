package internal

import (
	"context"
	"errors"
	"fmt"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// WithTransaction runs fn inside a pgx transaction. If db is already a pgx.Tx, fn
// runs on that transaction without begin/commit/rollback. Otherwise db must
// implement TxBeginner.
func WithTransaction(
	ctx context.Context, db DBTX, logger *zap.Logger,
	fn func(pgx.Tx) error,
) error {
	logger = WithContext(ctx, logger)

	existingTx, ok := db.(pgx.Tx)
	if ok {
		return fn(existingTx)
	}

	beginner, ok := db.(TxBeginner)
	if !ok {
		return fmt.Errorf("%w", ErrDBTransactionNotSupported)
	}

	tx, err := beginner.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return databaseutil.WrapDBError(err, logger, "begin tx")
	}

	defer func() {
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			logger.Error("rollback failed", zap.Error(rollbackErr))
		}
	}()

	err = fn(tx)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return databaseutil.WrapDBError(err, logger, "commit tx")
	}

	return nil
}

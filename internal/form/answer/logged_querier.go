package answer

import (
	"context"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type loggedQuerier struct {
	base   Querier
	logger *zap.Logger
}

func newLoggedQuerier(base Querier, logger *zap.Logger) Querier {
	return &loggedQuerier{
		base:   base,
		logger: logger,
	}
}

func (q *loggedQuerier) ListByResponseID(ctx context.Context, responseID uuid.UUID) ([]Answer, error) {
	logger := withEvent(logutil.WithContext(ctx, q.logger), eventTypeDepDatabase)

	result, err := q.base.ListByResponseID(ctx, responseID)
	if err != nil {
		logger.Warn(
			dbFailureMessage("ListByResponseID"),
			zap.String("db.operation", "ListByResponseID"),
			zap.Any("db.parameters", map[string]string{"response_id": responseID.String()}),
			zap.Error(err),
		)
		return nil, err
	}

	logger.Debug(
		dbReadSuccessMessage("ListByResponseID", len(result)),
		zap.String("db.operation", "ListByResponseID"),
		zap.Int("db.rows_affected", len(result)),
		zap.Any("db.pk", map[string]string{"response_id": responseID.String()}),
	)

	return result, nil
}

func (q *loggedQuerier) GetByResponseIDAndQuestionID(ctx context.Context, arg GetByResponseIDAndQuestionIDParams) (Answer, error) {
	logger := withEvent(logutil.WithContext(ctx, q.logger), eventTypeDepDatabase)

	result, err := q.base.GetByResponseIDAndQuestionID(ctx, arg)
	if err != nil {
		logger.Warn(
			dbFailureMessage("GetByResponseIDAndQuestionID"),
			zap.String("db.operation", "GetByResponseIDAndQuestionID"),
			zap.Any("db.parameters", map[string]string{
				"response_id": arg.ResponseID.String(),
				"question_id": arg.QuestionID.String(),
			}),
			zap.Error(err),
		)
		return Answer{}, err
	}

	logger.Debug(
		dbReadSuccessMessage("GetByResponseIDAndQuestionID", 1),
		zap.String("db.operation", "GetByResponseIDAndQuestionID"),
		zap.Int("db.rows_affected", 1),
		zap.Any("db.pk", map[string]string{
			"id":          result.ID.String(),
			"response_id": result.ResponseID.String(),
			"question_id": result.QuestionID.String(),
		}),
	)

	return result, nil
}

func (q *loggedQuerier) BatchUpsert(ctx context.Context, arg BatchUpsertParams) ([]Answer, error) {
	logger := withEvent(logutil.WithContext(ctx, q.logger), eventTypeDepDatabase)

	result, err := q.base.BatchUpsert(ctx, arg)
	if err != nil {
		logger.Warn(
			dbFailureMessage("BatchUpsert"),
			zap.String("db.operation", "BatchUpsert"),
			zap.Any("db.parameters", map[string]int{
				"response_id_count": len(arg.ResponseIds),
				"question_id_count": len(arg.QuestionIds),
				"value_count":       len(arg.Values),
			}),
			zap.Error(err),
		)
		return nil, err
	}

	logger.Info(
		dbWriteSuccessMessage("BatchUpsert", len(result)),
		zap.String("db.operation", "BatchUpsert"),
		zap.Int("db.rows_affected", len(result)),
	)

	return result, nil
}

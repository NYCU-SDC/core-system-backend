package answer

import (
	"fmt"

	"go.uber.org/zap"
)

const (
	eventTypeHTTPPayload = "HTTP_PAYLOAD"
	eventTypeBizFlow     = "BIZ_FLOW"
	eventTypeBizLogic    = "BIZ_LOGIC"
	eventTypeDepDatabase = "DEP_DATABASE"
	eventTypeUtilCodec   = "UTIL_CODEC"
)

func withEvent(logger *zap.Logger, eventType string) *zap.Logger {
	return logger.With(zap.String("event_type", eventType))
}

func dbReadSuccessMessage(operation string, rowsAffected int) string {
	return fmt.Sprintf("DB operation %s completed: retrieved %d row(s)", operation, rowsAffected)
}

func dbWriteSuccessMessage(operation string, rowsAffected int) string {
	return fmt.Sprintf("DB operation %s completed: affected %d row(s)", operation, rowsAffected)
}

func dbFailureMessage(operation string) string {
	return fmt.Sprintf("DB operation %s failed", operation)
}

package user

import (
	"context"
	"encoding/json"
	"errors"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EmailAuthEntry is one email on a user's login profile with the OAuth providers linked to that address.
type EmailAuthEntry struct {
	Email         string   `json:"email"`
	AuthProviders []string `json:"authProviders"`
}

// GetLoginProfileEntries returns emails and linked auth providers stored as JSON for the user.
func (s *Service) GetLoginProfileEntries(ctx context.Context, userID uuid.UUID) ([]EmailAuthEntry, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetLoginProfileEntries")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	profile, err := s.queries.GetLoginProfile(traceCtx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []EmailAuthEntry{}, nil
		}
		err = databaseutil.WrapDBError(err, logger, "get login profile")
		span.RecordError(err)
		return nil, err
	}

	if len(profile) == 0 {
		return []EmailAuthEntry{}, nil
	}

	var entries []EmailAuthEntry
	err = json.Unmarshal(profile, &entries)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "unmarshal login profile")
		span.RecordError(err)
		return nil, err
	}

	if entries == nil {
		return []EmailAuthEntry{}, nil
	}

	for i := range entries {
		if entries[i].AuthProviders == nil {
			entries[i].AuthProviders = []string{}
		}
	}

	return entries, nil
}

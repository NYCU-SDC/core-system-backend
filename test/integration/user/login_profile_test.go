package user

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"NYCU-SDC/core-system-backend/test/integration"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGetLoginProfileEntries_fromView(t *testing.T) {
	resourceManager, logger, err := integration.GetOrInitResource()
	require.NoError(t, err)

	db, rollback, err := resourceManager.SetupPostgres()
	require.NoError(t, err)
	defer rollback()

	builder := userbuilder.New(t, db)
	account := builder.Create()
	emailA := fmt.Sprintf("profile-a-%s@example.com", uuid.NewString())
	emailB := fmt.Sprintf("profile-b-%s@example.com", uuid.NewString())
	builder.CreateEmail(account.ID, emailA)
	builder.CreateEmail(account.ID, emailB)
	builder.CreateAuth(account.ID, "github", uuid.NewString())
	builder.CreateAuth(account.ID, "google", uuid.NewString())

	svc := newUserService(t, db, logger)
	entries, err := svc.GetLoginProfileEntries(context.Background(), account.ID)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	byEmail := make(map[string]user.EmailAuthEntry, len(entries))
	for _, entry := range entries {
		byEmail[entry.Email] = entry
	}

	require.ElementsMatch(t, []string{"github", "google"}, byEmail[emailA].AuthProviders)
	require.ElementsMatch(t, []string{"github", "google"}, byEmail[emailB].AuthProviders)
}

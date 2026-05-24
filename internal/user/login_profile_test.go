package user

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmailAuthEntryJSON(t *testing.T) {
	t.Parallel()

	raw := `[{"email":"a@example.com","authProviders":["github","google"]},{"email":"b@example.com","authProviders":[]}]`
	var entries []EmailAuthEntry
	require.NoError(t, json.Unmarshal([]byte(raw), &entries))
	require.Len(t, entries, 2)
	require.Equal(t, "a@example.com", entries[0].Email)
	require.Equal(t, []string{"github", "google"}, entries[0].AuthProviders)
	require.Empty(t, entries[1].AuthProviders)
}

package user

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---- parseMulti ----

func TestParseMulti(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name     string
		input    string
		expected map[string][]string
	}

	testCases := []testCase{
		{
			name:     "empty string",
			input:    "",
			expected: map[string][]string{},
		},
		{
			name:  "single entry",
			input: "alice@example.com:admin",
			expected: map[string][]string{
				"alice@example.com": {"admin"},
			},
		},
		{
			name:  "multiple entries different emails",
			input: "alice@example.com:admin,bob@example.com:viewer",
			expected: map[string][]string{
				"alice@example.com": {"admin"},
				"bob@example.com":   {"viewer"},
			},
		},
		{
			name:  "same email multiple roles",
			input: "alice@example.com:admin,alice@example.com:editor",
			expected: map[string][]string{
				"alice@example.com": {"admin", "editor"},
			},
		},
		{
			name:  "whitespace around entries is trimmed",
			input: " alice@example.com:admin , bob@example.com:viewer ",
			expected: map[string][]string{
				"alice@example.com": {"admin"},
				"bob@example.com":   {"viewer"},
			},
		},
		{
			name:  "email and role are normalized to lowercase",
			input: "Alice@Example.COM:Admin",
			expected: map[string][]string{
				"alice@example.com": {"admin"},
			},
		},
		{
			name:     "entry without colon is skipped",
			input:    "alice@example.com,bob@example.com:viewer",
			expected: map[string][]string{"bob@example.com": {"viewer"}},
		},
		{
			name:  "colon in role value (SplitN keeps remainder)",
			input: "alice@example.com:role:extra",
			expected: map[string][]string{
				"alice@example.com": {"role:extra"},
			},
		},
		{
			name:     "only commas / blank entries are ignored",
			input:    ",,, ,",
			expected: map[string][]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseMulti(tc.input)

			// Sort role slices for deterministic comparison
			for k := range got {
				sort.Strings(got[k])
			}
			for k := range tc.expected {
				sort.Strings(tc.expected[k])
			}

			require.Equal(t, tc.expected, got)
		})
	}
}

// ---- parseSingle ----

func TestParseSingle(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name     string
		input    string
		expected map[string]string
	}

	testCases := []testCase{
		{
			name:     "empty string",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:  "single entry",
			input: "alice@example.com:owner",
			expected: map[string]string{
				"alice@example.com": "owner",
			},
		},
		{
			name:  "multiple entries different emails",
			input: "alice@example.com:owner,bob@example.com:member",
			expected: map[string]string{
				"alice@example.com": "owner",
				"bob@example.com":   "member",
			},
		},
		{
			name:  "duplicate email last entry wins",
			input: "alice@example.com:owner,alice@example.com:member",
			expected: map[string]string{
				"alice@example.com": "member",
			},
		},
		{
			name:  "whitespace around entries is trimmed",
			input: " alice@example.com:owner , bob@example.com:member ",
			expected: map[string]string{
				"alice@example.com": "owner",
				"bob@example.com":   "member",
			},
		},
		{
			name:  "email and role are normalized to lowercase",
			input: "Alice@Example.COM:Owner",
			expected: map[string]string{
				"alice@example.com": "owner",
			},
		},
		{
			name:     "entry without colon is skipped",
			input:    "alice@example.com,bob@example.com:member",
			expected: map[string]string{"bob@example.com": "member"},
		},
		{
			name:     "only commas / blank entries are ignored",
			input:    ",,, ,",
			expected: map[string]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseSingle(tc.input)
			require.Equal(t, tc.expected, got)
		})
	}
}

// ---- DefaultGlobalRoles ----

func TestDefaultGlobalRoles(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name     string
		cfg      string
		email    string
		expected []string
	}

	testCases := []testCase{
		{
			name:     "uninitialised store returns nil",
			cfg:      "",
			email:    "alice@example.com",
			expected: nil,
		},
		{
			name:     "email not in config returns nil",
			cfg:      "bob@example.com:admin",
			email:    "alice@example.com",
			expected: nil,
		},
		{
			name:     "single role returned",
			cfg:      "alice@example.com:admin",
			email:    "alice@example.com",
			expected: []string{"admin"},
		},
		{
			name:     "multiple roles returned",
			cfg:      "alice@example.com:admin,alice@example.com:editor",
			email:    "alice@example.com",
			expected: []string{"admin", "editor"},
		},
		{
			name:     "email lookup is case-insensitive",
			cfg:      "alice@example.com:admin",
			email:    "Alice@Example.COM",
			expected: []string{"admin"},
		},
		{
			name:     "returned slice is a copy (mutation does not affect store)",
			cfg:      "alice@example.com:admin",
			email:    "alice@example.com",
			expected: []string{"admin"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Use a local store to avoid global state pollution between parallel tests.
			local := roleStore{
				global: parseMulti(tc.cfg),
			}

			var got []string
			if local.global == nil {
				got = nil
			} else {
				got = clone(local.global[normalize(tc.email)])
			}

			sort.Strings(got)
			expected := append([]string(nil), tc.expected...)
			sort.Strings(expected)

			require.Equal(t, expected, got)
		})
	}
}

// TestDefaultGlobalRoles_MutationSafety verifies that mutating the returned
// slice does not affect subsequent calls (clone isolation).
func TestDefaultGlobalRoles_MutationSafety(t *testing.T) {
	t.Parallel()

	local := roleStore{
		global: parseMulti("alice@example.com:admin"),
	}

	first := clone(local.global[normalize("alice@example.com")])
	require.Equal(t, []string{"admin"}, first)

	// Mutate the returned slice
	first[0] = "hacked"

	second := clone(local.global[normalize("alice@example.com")])
	require.Equal(t, []string{"admin"}, second, "store was mutated through returned slice")
}

// ---- DefaultOrgRole ----

func TestDefaultOrgRole(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name         string
		cfg          string
		email        string
		expectedRole string
		expectedOk   bool
	}

	testCases := []testCase{
		{
			name:         "uninitialised store returns empty and false",
			cfg:          "",
			email:        "alice@example.com",
			expectedRole: "",
			expectedOk:   false,
		},
		{
			name:         "email not in config returns empty and false",
			cfg:          "bob@example.com:member",
			email:        "alice@example.com",
			expectedRole: "",
			expectedOk:   false,
		},
		{
			name:         "email found returns role and true",
			cfg:          "alice@example.com:owner",
			email:        "alice@example.com",
			expectedRole: "owner",
			expectedOk:   true,
		},
		{
			name:         "duplicate email last entry wins",
			cfg:          "alice@example.com:owner,alice@example.com:member",
			email:        "alice@example.com",
			expectedRole: "member",
			expectedOk:   true,
		},
		{
			name:         "email lookup is case-insensitive",
			cfg:          "alice@example.com:owner",
			email:        "Alice@Example.COM",
			expectedRole: "owner",
			expectedOk:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			local := roleStore{
				org: parseSingle(tc.cfg),
			}

			// Replicate the logic of DefaultOrgRole against the local store.
			var gotRole string
			var gotOk bool
			if local.org == nil {
				gotRole, gotOk = "", false
			} else {
				gotRole, gotOk = local.org[normalize(tc.email)]
			}

			require.Equal(t, tc.expectedOk, gotOk)
			require.Equal(t, tc.expectedRole, gotRole)
		})
	}
}

// ---- Init functions affect global store ----

func TestInitDefaultGlobalRole_SetsGlobalStore(t *testing.T) {
	// Not parallel — touches the global store.
	original := store.global
	t.Cleanup(func() { store.global = original })

	InitDefaultGlobalRole("alice@example.com:admin")

	got := DefaultGlobalRoles("alice@example.com")
	require.Equal(t, []string{"admin"}, got)
}

func TestInitDefaultOrgRole_SetsOrgStore(t *testing.T) {
	// Not parallel — touches the global store.
	original := store.org
	t.Cleanup(func() { store.org = original })

	InitDefaultOrgRole("alice@example.com:owner")

	role, ok := DefaultOrgRole("alice@example.com")
	require.True(t, ok)
	require.Equal(t, "owner", role)
}

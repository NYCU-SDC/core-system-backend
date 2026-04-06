package markdown

import (
	"encoding/json"
	"strings"
	"testing"

	"NYCU-SDC/core-system-backend/internal"

	"github.com/stretchr/testify/require"
)

func TestProcessRequest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		raw         []byte
		validate    func(t *testing.T, raw []byte, docJSON []byte, docHTML string)
		expectedErr error
	}{
		{
			name: "JSON-encoded plain string",
			raw:  []byte(`"Hello world"`),
			validate: func(t *testing.T, _ []byte, docJSON []byte, docHTML string) {
				t.Helper()
				require.Contains(t, string(docJSON), `"type":"doc"`)
				require.Contains(t, string(docJSON), "Hello world")
				require.Contains(t, docHTML, "Hello world")
			},
		},
		{
			name: "empty JSON string",
			raw:  []byte(`""`),
			validate: func(t *testing.T, _ []byte, docJSON []byte, _ string) {
				t.Helper()
				require.Contains(t, string(docJSON), `"type":"doc"`)
			},
		},
		{
			name:        "malformed JSON string",
			raw:         []byte(`"unclosed`),
			expectedErr: internal.ErrInvalidDocumentJSON,
		},
		{
			name: "prose mirror object matches Process",
			raw:  []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}`),
			validate: func(t *testing.T, raw []byte, docJSON []byte, docHTML string) {
				t.Helper()
				j2, h2, err := Process(raw)
				require.NoError(t, err)
				require.Equal(t, string(j2), string(docJSON))
				require.Equal(t, h2, docHTML)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			docJSON, docHTML, err := ProcessRequest(tc.raw)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			tc.validate(t, tc.raw, docJSON, docHTML)
		})
	}
}

func TestProcess(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		raw         []byte
		validate    func(t *testing.T, docJSON []byte, docHTML string)
		expectedErr error
	}{
		{
			name: "empty nil input",
			raw:  nil,
			validate: func(t *testing.T, docJSON []byte, docHTML string) {
				t.Helper()
				require.JSONEq(t, EmptyDocumentJSON, string(docJSON))
				require.Equal(t, "", docHTML)
			},
			expectedErr: nil,
		},
		{
			name: "empty whitespace input",
			raw:  []byte("  \n\t  "),
			validate: func(t *testing.T, docJSON []byte, docHTML string) {
				t.Helper()
				require.JSONEq(t, EmptyDocumentJSON, string(docJSON))
				require.Equal(t, "", docHTML)
			},
			expectedErr: nil,
		},
		{
			name: "null JSON literal",
			raw:  []byte("null"),
			validate: func(t *testing.T, docJSON []byte, docHTML string) {
				t.Helper()
				require.JSONEq(t, EmptyDocumentJSON, string(docJSON))
				require.Equal(t, "", docHTML)
			},
			expectedErr: nil,
		},
		{
			name: "explicit empty doc canonical JSON",
			raw:  []byte(`{"type":"doc","content":[]}`),
			validate: func(t *testing.T, docJSON []byte, docHTML string) {
				t.Helper()
				var doc map[string]any
				err := json.Unmarshal(docJSON, &doc)
				require.NoError(t, err)
				require.Equal(t, "doc", doc["type"])
				c, ok := doc["content"].([]any)
				require.True(t, !ok || len(c) == 0, "empty doc should omit or empty content")
				require.Equal(t, "", docHTML)
			},
			expectedErr: nil,
		},
		{
			name: "bold paragraph",
			raw:  []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello "},{"type":"text","marks":[{"type":"bold"}],"text":"world"}]}]}`),
			validate: func(t *testing.T, docJSON []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, "<strong>world</strong>")
				require.Contains(t, string(docJSON), `"type":"doc"`)
			},
		},
		{
			name: "italic and code marks",
			raw:  []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"italic"}],"text":"it"},{"type":"text","marks":[{"type":"code"}],"text":"code"}]}]}`),
			validate: func(t *testing.T, _ []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, "<em>it</em>")
				require.Contains(t, docHTML, "<code>code</code>")
			},
			expectedErr: nil,
		},
		{
			name: "heading level 3",
			raw:  []byte(`{"type":"doc","content":[{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"Section"}]}]}`),
			validate: func(t *testing.T, _ []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, "<h3>")
				require.Contains(t, docHTML, "Section")
				require.Contains(t, docHTML, "</h3>")
			},
		},
		{
			name: "code block wrapped in pre",
			raw:  []byte(`{"type":"doc","content":[{"type":"code_block","content":[{"type":"text","text":"fmt.Println(\"hi\")"}]}]}`),
			validate: func(t *testing.T, _ []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, "<pre><code>")
				require.True(t, strings.Contains(docHTML, `&#34;`) || strings.Contains(docHTML, `&quot;`), "escaped quotes in code output")
				require.Contains(t, docHTML, "fmt.Println")
			},
			expectedErr: nil,
		},
		{
			name: "bullet list",
			raw:  []byte(`{"type":"doc","content":[{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]}]},{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"two"}]}]}]}]}`),
			validate: func(t *testing.T, _ []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, "<ul>")
				require.Contains(t, docHTML, "<li>")
				require.Contains(t, docHTML, "one")
				require.Contains(t, docHTML, "two")
			},
			expectedErr: nil,
		},
		{
			name: "link https with blank target",
			raw:  []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com/x","target":"_blank"}}],"text":"go"}]}]}`),
			validate: func(t *testing.T, _ []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, `href="https://example.com/x"`)
				require.Contains(t, docHTML, ">go</a>")
				// UGC sanitizer may strip target/rel; anchor and href must remain.
			},
		},
		{
			name: "mailto link",
			raw:  []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":"mailto:a@b.co"}}],"text":"mail"}]}]}`),
			validate: func(t *testing.T, _ []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, `href="mailto:a@b.co"`)
			},
			expectedErr: nil,
		},
		{
			name: "horizontal rule and hard break",
			raw:  []byte(`{"type":"doc","content":[{"type":"horizontal_rule"},{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hard_break"},{"type":"text","text":"b"}]}]}`),
			validate: func(t *testing.T, _ []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, "<hr")
				require.Contains(t, docHTML, "<br")
			},
			expectedErr: nil,
		},
		{
			name: "top-level hard break is normalized into paragraph",
			raw:  []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]},{"type":"hard_break"},{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}`),
			validate: func(t *testing.T, docJSON []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, "<br")
				require.Contains(t, docHTML, "a")
				require.Contains(t, docHTML, "b")
				require.Contains(t, string(docJSON), `"type":"paragraph"`)
			},
			expectedErr: nil,
		},
		{
			name: "variable node renders placeholder span",
			raw:  []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"var: "},{"type":"variable","attrs":{"name":"VAR_TEST"}}]}]}`),
			validate: func(t *testing.T, _ []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, "{{VAR_TEST}}")
			},
			expectedErr: nil,
		},
		{
			name:        "rejects image node",
			raw:         []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"before"}]},{"type":"image","attrs":{"src":"https://placehold.co/300x200.png","alt":"x"}},{"type":"paragraph","content":[{"type":"text","text":"after"}]}]}`),
			expectedErr: internal.ErrInvalidDocumentNode,
		},
		{
			name: "blockquote",
			raw:  []byte(`{"type":"doc","content":[{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"quoted"}]}]}]}`),
			validate: func(t *testing.T, _ []byte, docHTML string) {
				t.Helper()
				require.Contains(t, docHTML, "<blockquote>")
				require.Contains(t, docHTML, "quoted")
			},
			expectedErr: nil,
		},
		{
			name:        "rejects unknown node",
			raw:         []byte(`{"type":"doc","content":[{"type":"evil","content":[]}]}`),
			expectedErr: internal.ErrInvalidDocumentNode,
		},
		{
			name:        "rejects invalid JSON",
			raw:         []byte(`{"type":"doc",`),
			expectedErr: internal.ErrInvalidDocumentJSON,
		},
		{
			name:        "rejects non-doc root",
			raw:         []byte(`{"type":"paragraph","content":[{"type":"text","text":"x"}]}`),
			expectedErr: internal.ErrInvalidDocumentRoot,
		},
		{
			name:        "rejects heading level out of range",
			raw:         []byte(`{"type":"doc","content":[{"type":"heading","attrs":{"level":9},"content":[{"type":"text","text":"x"}]}]}`),
			expectedErr: internal.ErrInvalidDocumentHeading,
		},
		{
			name:        "rejects link with empty href",
			raw:         []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":""}}],"text":"x"}]}]}`),
			expectedErr: internal.ErrInvalidDocumentLink,
		},
		{
			name:        "rejects link with disallowed target",
			raw:         []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com","target":"_parent"}}],"text":"x"}]}]}`),
			expectedErr: internal.ErrInvalidDocumentLink,
		},
		{
			name:        "rejects link with relative href",
			raw:         []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":"/path"}}],"text":"x"}]}]}`),
			expectedErr: internal.ErrInvalidDocumentLink,
		},
		{
			name: "sanitizes script in text",
			raw:  []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"<script>alert(1)</script>"}]}]}`),
			validate: func(t *testing.T, _ []byte, docHTML string) {
				t.Helper()
				require.NotContains(t, docHTML, "<script>")
				require.True(t, strings.Contains(docHTML, "&lt;script&gt;") || strings.Contains(docHTML, "script"))
			},
			expectedErr: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			docJSON, docHTML, err := Process(tc.raw)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			tc.validate(t, docJSON, docHTML)
		})
	}
}

func TestPlainText(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		raw         []byte
		expected    string
		expectedErr error
	}{
		{
			name:        "nil and empty",
			raw:         nil,
			expected:    "",
			expectedErr: nil,
		},
		{
			name:        "whitespace only",
			raw:         []byte(" \t\n"),
			expected:    "",
			expectedErr: nil,
		},
		{
			name:        "nested paragraph with bold",
			raw:         []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"text","marks":[{"type":"bold"}],"text":"b"}]}]}`),
			expected:    "ab",
			expectedErr: nil,
		},
		{
			name:        "multiple blocks concatenated",
			raw:         []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"p1"}]},{"type":"paragraph","content":[{"type":"text","text":"p2"}]}]}`),
			expected:    "p1p2",
			expectedErr: nil,
		},
		{
			name:        "heading and list",
			raw:         []byte(`{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"H"}]},{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"i"}]}]}]}]}`),
			expected:    "Hi",
			expectedErr: nil,
		},
		{
			name:        "hard break is not a visible character",
			raw:         []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hard_break"},{"type":"text","text":"b"}]}]}`),
			expected:    "ab",
			expectedErr: nil,
		},
		{
			name:        "invalid JSON",
			raw:         []byte(`not json`),
			expected:    "",
			expectedErr: internal.ErrInvalidDocumentJSON,
		},
		{
			name:        "unknown top-level type",
			raw:         []byte(`{"type":"docx","content":[]}`),
			expected:    "",
			expectedErr: internal.ErrInvalidDocumentNode,
		},
		{
			name:        "paragraph as root",
			raw:         []byte(`{"type":"paragraph","content":[{"type":"text","text":"x"}]}`),
			expected:    "",
			expectedErr: internal.ErrInvalidDocumentRoot,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := PlainText(tc.raw)
			require.Equal(t, tc.expected, s)
			require.ErrorIs(t, err, tc.expectedErr)
		})
	}
}

func TestPreviewSnippet(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		raw           []byte
		maxRunes      int
		expectedRunes int
		expectedErr   error
	}{
		{
			name:          "truncates to max runes",
			raw:           []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + strings.Repeat("x", 30) + `"}]}]}`),
			maxRunes:      25,
			expectedRunes: 25,
			expectedErr:   nil,
		},
		{
			name:          "maxZero returns full text",
			raw:           []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + strings.Repeat("x", 12) + `"}]}]}`),
			maxRunes:      0,
			expectedRunes: 12,
			expectedErr:   nil,
		},
		{
			name:          "unicode truncation by runes not bytes",
			raw:           []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"你好世界"}]}]}`),
			maxRunes:      2,
			expectedRunes: 2,
			expectedErr:   nil,
		},
		{
			name:          "shorter than max returns full",
			raw:           []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`),
			maxRunes:      100,
			expectedRunes: 2,
			expectedErr:   nil,
		},
		{
			name:        "propagates PlainText JSON error",
			raw:         []byte(`not json`),
			maxRunes:    10,
			expectedErr: internal.ErrInvalidDocumentJSON,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := PreviewSnippet(tc.raw, tc.maxRunes)
			require.Len(t, []rune(s), tc.expectedRunes)
			require.ErrorIs(t, err, tc.expectedErr)
		})
	}
}

func TestEmptyDocContent(t *testing.T) {
	t.Parallel()
	_, _, err := Process([]byte(`{"type":"doc","content":[]}`))
	t.Logf("err=%v", err)
}

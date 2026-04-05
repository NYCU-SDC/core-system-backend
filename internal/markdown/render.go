package markdown

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"NYCU-SDC/core-system-backend/internal"

	pm "github.com/karitham/prosemirror"
	pmgo "github.com/nicksrandall/prosemirror-go"
)

// collectText recursively extracts plain text from a karitham node tree.
func collectText(n pm.Node, b *strings.Builder) {
	if n.Type.Name == NodeText {
		b.WriteString(n.Text)
		return
	}

	for _, child := range n.Content.Content {
		collectText(child, b)
	}
}

// renderHTML converts a validated karitham document to HTML using nicksrandall/prosemirror-go.
func renderHTML(root pm.Node) (string, error) {
	raw, err := json.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("%w: marshal for render: %w", internal.ErrInvalidDocumentRender, err)
	}

	var doc pmgo.Content
	err = json.Unmarshal(raw, &doc)
	if err != nil {
		return "", fmt.Errorf("%w: unmarshal for render: %w", internal.ErrInvalidDocumentRender, err)
	}

	// prosemirror-go writes text verbatim; escape before generating HTML so
	// entities are consistent with the previous renderer and UGC sanitization.
	escapeAllTextNodes(&doc)

	state := &pmgo.EditorState{Doc: &doc}

	return pmgo.Render(state, htmlRenderConfig()), nil
}

func escapeAllTextNodes(c *pmgo.Content) {
	if c == nil {
		return
	}

	if c.Type == NodeText && c.Text != "" {
		c.Text = html.EscapeString(c.Text)
	}

	for _, child := range c.Content {
		escapeAllTextNodes(child)
	}
}

// htmlRenderConfig builds on the library defaults but adds Tiptap mark names,
// wraps code_block as <pre><code>, and hardens link output (escape attrs, target=_blank).
func htmlRenderConfig() *pmgo.Config {
	cfg := pmgo.NewHTMLConfig()

	cfg.NodeRenderers[NodeCodeBlock] = pmgo.SimpleOption{
		Before: "<pre><code>",
		After:  "</code></pre>",
	}

	cfg.MarkRenderers[MarkBold] = pmgo.SimpleOption{
		Before: "<strong>",
		After:  "</strong>",
	}
	cfg.MarkRenderers[MarkItalic] = pmgo.SimpleOption{
		Before: "<em>",
		After:  "</em>",
	}
	cfg.MarkRenderers[MarkUnderline] = pmgo.SimpleOption{
		Before: "<u>",
		After:  "</u>",
	}
	cfg.MarkRenderers[MarkStrike] = pmgo.SimpleOption{
		Before: "<s>",
		After:  "</s>",
	}
	cfg.MarkRenderers[MarkLink] = linkMarkOption{}

	return cfg
}

type linkMarkOption struct{}

func (linkMarkOption) RenderBefore(_ int, attrs map[string]interface{}) string {
	href := ""
	if v, ok := attrs["href"].(string); ok {
		href = html.EscapeString(v)
	}

	titleAttr := ""
	if v, ok := attrs["title"].(string); ok && v != "" {
		titleAttr = ` title="` + html.EscapeString(v) + `"`
	}

	targetAttr := ""
	if v, ok := attrs["target"].(string); ok && v == LinkTargetBlank {
		targetAttr = ` rel="noopener noreferrer" target="` + LinkTargetBlank + `"`
	}

	return fmt.Sprintf(`<a href="%s"%s%s>`, href, titleAttr, targetAttr)
}

func (linkMarkOption) RenderAfter(_ int, _ map[string]interface{}) string {
	return "</a>"
}

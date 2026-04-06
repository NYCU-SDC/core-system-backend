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

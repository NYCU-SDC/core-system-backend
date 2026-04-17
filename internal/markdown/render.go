package markdown

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"NYCU-SDC/core-system-backend/internal"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	pm "github.com/karitham/prosemirror"
	pmgo "github.com/nicksrandall/prosemirror-go"
	"go.uber.org/zap"
)

// collectText recursively extracts plain text from a karitham node tree.
//
// It is intentionally minimal but should preserve user-visible separation:
// - hard breaks become "\n"
// - block siblings are separated by "\n"
func collectText(n pm.Node, b *strings.Builder) {
	if n.Type.Name == NodeText {
		b.WriteString(n.Text)
		return
	}

	if n.Type.Name == NodeHardBreak {
		b.WriteByte('\n')
		return
	}

	if n.Type.Name == NodeVariable {
		name, _ := n.Attrs["name"].(string)
		b.WriteString("{{")
		b.WriteString(name)
		b.WriteString("}}")
		return
	}

	children := n.Content.Content
	if len(children) == 0 {
		return
	}

	// Insert newlines between block-ish siblings to avoid "p1p2" and similar joins.
	for i, child := range children {
		if i > 0 && isBlockLike(child) && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		collectText(child, b)
	}
}

func isBlockLike(n pm.Node) bool {
	switch n.Type.Name {
	case NodeParagraph, NodeHeading, NodeBlockquote, NodeListItem, NodeBulletList, NodeOrderedList, NodeCodeBlock:
		return true
	default:
		return false
	}
}

// renderHTML converts a validated karitham document to HTML using nicksrandall/prosemirror-go.
func (s *Service) renderHTML(ctx context.Context, root pm.Node) (string, error) {
	traceCtx, span := s.tracer.Start(ctx, "renderHTML")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	raw, err := json.Marshal(root)
	if err != nil {
		err := fmt.Errorf("%w: marshal for render: %w", internal.ErrInvalidDocumentRender, err)
		logger.Error("failed to marshal for render", zap.Error(err))
		span.RecordError(err)
		return "", err
	}

	var doc pmgo.Content
	err = json.Unmarshal(raw, &doc)
	if err != nil {
		err := fmt.Errorf("%w: unmarshal for render: %w", internal.ErrInvalidDocumentRender, err)
		logger.Error("failed to unmarshal for render", zap.Error(err))
		span.RecordError(err)
		return "", err
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

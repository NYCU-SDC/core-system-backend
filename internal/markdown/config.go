package markdown

import (
	"fmt"
	"html"

	pmgo "github.com/nicksrandall/prosemirror-go"
)

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

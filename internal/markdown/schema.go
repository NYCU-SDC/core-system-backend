package markdown

import (
	"fmt"
	"net/url"
	"strings"

	"NYCU-SDC/core-system-backend/internal"

	pm "github.com/karitham/prosemirror"
)

// Node type name constants — keep in sync with the admin Tiptap/StarterKit configuration
// and the CoreSystem.ProseMirror* definitions in core-system-api TypeSpec.
const (
	NodeDoc            = "doc"
	NodeParagraph      = "paragraph"
	NodeHeading        = "heading"
	NodeBulletList     = "bullet_list"
	NodeOrderedList    = "ordered_list"
	NodeListItem       = "list_item"
	NodeBlockquote     = "blockquote"
	NodeCodeBlock      = "code_block"
	NodeHorizontalRule = "horizontal_rule"
	NodeHardBreak      = "hard_break"
	NodeVariable       = "variable"
	NodeText           = "text"
)

// Mark type name constants.
const (
	MarkBold      = "bold"
	MarkItalic    = "italic"
	MarkUnderline = "underline"
	MarkStrike    = "strike"
	MarkCode      = "code"
	MarkLink      = "link"
)

const (
	schemeHTTP         = "http"
	schemeHTTPS        = "https"
	schemeMailtoPrefix = "mailto:"

	// LinkTargetBlank is the only permitted non-empty link target (new tab).
	LinkTargetBlank = "_blank"
)

// EmptyDocumentJSON is the canonical empty ProseMirror doc.
const EmptyDocumentJSON = `{"type":"` + NodeDoc + `","content":[]}`

// noMarks is a sentinel used in NodeSpec.Marks to mean "no marks allowed".
var noMarks = ""

// Schema is the compiled ProseMirror schema for this application.
// NewSchema registers it globally so that json.Unmarshal works on pm.Node.
//
//nolint:gochecknoglobals
var Schema = pm.Must(pm.NewSchema(pm.SchemaSpec{
	TopNode: NodeDoc,
	Nodes: map[pm.NodeTypeName]pm.NodeSpec{
		NodeDoc: {
			Content: "block+",
		},
		NodeParagraph: {
			Content: "inline*",
			Group:   "block",
		},
		NodeHeading: {
			Content: "inline*",
			Group:   "block",
			Attrs: map[string]pm.Attribute{
				"level": {Default: 1},
			},
		},
		NodeBulletList: {
			Content: "list_item+",
			Group:   "block",
		},
		NodeOrderedList: {
			Content: "list_item+",
			Group:   "block",
		},
		NodeListItem: {
			Content: "block+",
			Group:   "block",
		},
		NodeBlockquote: {
			Content: "block+",
			Group:   "block",
		},
		NodeCodeBlock: {
			Content: "text*",
			Group:   "block",
			Marks:   &noMarks,
		},
		NodeHorizontalRule: {
			Group: "block",
		},
		NodeHardBreak: {
			Group:  "inline",
			Inline: true,
		},
		NodeVariable: {
			Group:  "inline",
			Inline: true,
			Attrs: map[string]pm.Attribute{
				"name": {},
			},
		},
		NodeText: {
			Group: "inline",
		},
	},
	Marks: map[pm.MarkTypeName]pm.MarkSpec{
		MarkBold:      {},
		MarkItalic:    {},
		MarkUnderline: {},
		MarkStrike:    {},
		MarkCode:      {},
		MarkLink: {
			Attrs: map[string]pm.Attribute{
				"href":   {},
				"title":  {Default: ""},
				"target": {Default: ""},
			},
		},
	},
}))

// validateNode recursively validates a karitham Node against our schema rules.
// Schema-level unknown types are already caught during JSON unmarshal.
// This function enforces content expressions, heading level range, and link mark attr constraints.
func validateNode(n pm.Node) error {
	// Leaf nodes (text, hard_break, horizontal_rule, variable) carry no children —
	// their ContentMatch is the zero value with ValidEnd=false, so calling
	// CheckContent on them would always error. Validate their attrs (if any) and marks.
	if n.IsLeaf() {
		if n.Type.Name == NodeVariable {
			err := validateVariableAttrs(n.Attrs)
			if err != nil {
				return err
			}
		}
		return validateNodeMarks(n.Marks)
	}

	if err := n.Type.CheckContent(n.Content); err != nil {
		return fmt.Errorf("%w: %w", internal.ErrInvalidDocumentNode, err)
	}

	if n.Type.Name == NodeHeading {
		err := validateHeadingLevel(n.Attrs)
		if err != nil {
			return err
		}
	}

	if n.Type.Name == NodeVariable {
		err := validateVariableAttrs(n.Attrs)
		if err != nil {
			return err
		}
	}

	if err := validateNodeMarks(n.Marks); err != nil {
		return err
	}

	for _, child := range n.Content.Content {
		if err := validateNode(child); err != nil {
			return err
		}
	}

	return nil
}

func validateNodeMarks(marks []pm.Mark) error {
	for _, mark := range marks {
		if mark.Type.Name == MarkLink {
			err := validateLinkMark(mark)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func validateHeadingLevel(attrs map[string]any) error {
	v, ok := attrs["level"]
	if !ok {
		return fmt.Errorf("%w: heading requires level attr", internal.ErrInvalidDocumentHeading)
	}

	level := toInt(v)
	if level < 1 || level > 6 {
		return fmt.Errorf("%w: heading level must be 1-6, got %d", internal.ErrInvalidDocumentHeading, level)
	}

	return nil
}

func validateVariableAttrs(attrs map[string]any) error {
	name, _ := attrs["name"].(string)
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: variable requires name attr", internal.ErrInvalidDocumentNode)
	}
	return nil
}

// toInt converts a JSON-decoded number (float64 or int) to int.
func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	default:
		return 0
	}
}

func validateLinkMark(m pm.Mark) error {
	href, _ := m.Attrs["href"].(string)
	target, _ := m.Attrs["target"].(string)

	if href == "" {
		return fmt.Errorf("%w: link href required", internal.ErrInvalidDocumentLink)
	}

	if isHashHref(href) {
		// Fragment-only hrefs are useful for in-document navigation (e.g. TOC links).
		// These should not open new tabs.
		if target != "" {
			return fmt.Errorf("%w: hash links cannot set target", internal.ErrInvalidDocumentLink)
		}

		return nil
	}

	if isMailtoHref(href) {
		return validateMailtoLink(href, target)
	}

	return validateWebLink(href, target)
}

func isHashHref(href string) bool {
	return strings.HasPrefix(href, "#")
}

func isMailtoHref(href string) bool {
	return len(href) >= len(schemeMailtoPrefix) &&
		strings.EqualFold(href[:len(schemeMailtoPrefix)], schemeMailtoPrefix)
}

func validateMailtoLink(href, target string) error {
	addr := href[len(schemeMailtoPrefix):]
	if !strings.Contains(addr, "@") {
		return fmt.Errorf("%w: invalid mailto link", internal.ErrInvalidDocumentLink)
	}

	return validateLinkTarget(target)
}

func validateWebLink(href, target string) error {
	u, err := url.Parse(href)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%w: link href must be absolute URL", internal.ErrInvalidDocumentLink)
	}

	switch strings.ToLower(u.Scheme) {
	case schemeHTTP, schemeHTTPS:
	default:
		return fmt.Errorf("%w: link scheme not allowed", internal.ErrInvalidDocumentLink)
	}

	return validateLinkTarget(target)
}

func validateLinkTarget(target string) error {
	if target != "" && target != LinkTargetBlank {
		return fmt.Errorf("%w: link target not allowed", internal.ErrInvalidDocumentLink)
	}

	return nil
}

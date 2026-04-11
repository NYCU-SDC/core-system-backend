package markdown

import (
	"errors"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

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

// EmptyDocumentJSON is the canonical empty ProseMirror doc: one empty paragraph, satisfying NodeDoc's "block+"
// content rule (see Schema.Nodes[NodeDoc]) and matching DB defaults for description_json.
const EmptyDocumentJSON = `{"type":"` + NodeDoc + `","content":[{"type":"` + NodeParagraph + `"}]}`

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

type validationState struct {
	nodeCount int
	textRunes int
}

// validateDocument validates the doc tree, aggregate limits, and field rules.
func validateDocument(root pm.Node) error {
	st := &validationState{}
	return validateNode(st, root, 1)
}

// validateNode recursively validates a karitham Node against our schema rules.
// Schema-level unknown types are already caught during JSON unmarshal.
// This function enforces content expressions, heading level range, link mark attr constraints,
// and document size limits.
func validateNode(st *validationState, n pm.Node, depth int) error {
	err := validateDocumentDepthExceeded(depth)
	if err != nil {
		return err
	}

	st.nodeCount++
	err = validateDocumentNodeCountExceeded(st.nodeCount)
	if err != nil {
		return err
	}

	// Leaf nodes (text, hard_break, horizontal_rule, variable) carry no children —
	// their ContentMatch is the zero value with ValidEnd=false, so calling
	// CheckContent on them would always error. Validate their attrs (if any) and marks.
	if n.IsLeaf() {
		if n.Type.Name == NodeText {
			err = validateTextLeaf(st, n.Text)
			if err != nil {
				return err
			}
		}
		if n.Type.Name == NodeVariable {
			err = validateVariableAttrs(n.Attrs)
			if err != nil {
				return fmt.Errorf("%w: %w", internal.ErrInvalidDocumentNode, err)
			}
		}

		err = validateNodeMarks(n.Marks)
		if err != nil {
			return err
		}

		return nil
	}

	err = n.Type.CheckContent(n.Content)
	if err != nil {
		return fmt.Errorf("%w: %w", internal.ErrInvalidDocumentNode, err)
	}

	if n.Type.Name == NodeHeading {
		err = validateHeadingLevel(n.Attrs)
		if err != nil {
			return fmt.Errorf("%w: %w", internal.ErrInvalidDocumentNode, err)
		}
	}

	err = validateNodeMarks(n.Marks)
	if err != nil {
		return err
	}

	for _, child := range n.Content.Content {
		err = validateNode(st, child, depth+1)
		if err != nil {
			return wrapValidateNodeChildError(err)
		}
	}

	return nil
}

// validateTextLeaf validates a text node's UTF-8 runes against size limits.
func validateTextLeaf(st *validationState, text string) error {
	err := validateTextContainsNUL(text)
	if err != nil {
		return err
	}

	r := utf8.RuneCountInString(text)
	err = validateTextLeafTooLong(r)
	if err != nil {
		return err
	}

	err = validateTotalTextBudgetExceeded(st.textRunes, r)
	if err != nil {
		return err
	}
	st.textRunes += r

	return nil
}

// validateNodeMarks validates a node marks.
func validateNodeMarks(marks []pm.Mark) error {
	for _, mark := range marks {
		switch mark.Type.Name {
		case MarkBold, MarkItalic, MarkUnderline, MarkStrike, MarkCode, MarkLink:
		default:
			return fmt.Errorf("%w: unknown mark type %q", internal.ErrInvalidDocumentMark, mark.Type.Name)
		}
		if mark.Type.Name == MarkLink {
			err := validateLinkMark(mark)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// validateHeadingLevel validates a heading level.
func validateHeadingLevel(attrs map[string]any) error {
	v, ok := attrs["level"]
	if !ok {
		return fmt.Errorf("%w: heading level required", internal.ErrInvalidDocumentHeading)
	}

	_, err := parseHeadingLevel(v)
	return err
}

// validateVariableAttrs validates a variable attrs.
func validateVariableAttrs(attrs map[string]any) error {
	name, _ := attrs["name"].(string)
	if name == "" {
		return fmt.Errorf("%w: variable name required", internal.ErrInvalidDocumentVariableAttrs)
	}

	err := validateVariableNameContainsNUL(name)
	if err != nil {
		return err
	}

	err = validateVariableNameTooLong(name)
	if err != nil {
		return err
	}

	for _, r := range name {
		err = validateVariableNameRuneInvalid(r)
		if err != nil {
			return err
		}
	}

	return nil
}

// validateLinkMark validates a link mark.
func validateLinkMark(m pm.Mark) error {
	href, _ := m.Attrs["href"].(string)
	target, _ := m.Attrs["target"].(string)
	title, _ := m.Attrs["title"].(string)

	href = strings.TrimSpace(href)
	target = strings.TrimSpace(target)
	title = strings.TrimSpace(title)

	if href == "" {
		return fmt.Errorf("%w: link href required", internal.ErrInvalidDocumentLink)
	}

	err := validateLinkContainsNUL(href, title)
	if err != nil {
		return err
	}

	err = validateLinkHrefTooLong(href)
	if err != nil {
		return err
	}

	err = validateLinkTitleTooLong(title)
	if err != nil {
		return err
	}

	if isHashHref(href) {
		if target != "" {
			return fmt.Errorf("%w: hash links cannot set target", internal.ErrInvalidDocumentLink)
		}

		return nil
	}

	if isMailtoHref(href) {
		err = validateMailtoLink(href, target)
		if err != nil {
			return err
		}

		return nil
	}

	err = validateWebLink(href, target)
	if err != nil {
		return err
	}

	return nil
}

// isHashHref reports whether href is a fragment-only link (starts with "#") after trimming in validateLinkMark.
func isHashHref(href string) bool {
	return strings.HasPrefix(href, "#")
}

// isMailtoHref reports whether href begins with the mailto: prefix, case-insensitive.
func isMailtoHref(href string) bool {
	return len(href) >= len(schemeMailtoPrefix) &&
		strings.EqualFold(href[:len(schemeMailtoPrefix)], schemeMailtoPrefix)
}

// validateMailtoLink validates a mailto: link href and target.
func validateMailtoLink(href, target string) error {
	u, parseErr := url.Parse(href)
	if parseErr != nil || u == nil {
		return fmt.Errorf("%w: invalid mailto link", internal.ErrInvalidDocumentLink)
	}

	if !isMailtoScheme(u.Scheme) {
		return fmt.Errorf("%w: invalid mailto link", internal.ErrInvalidDocumentLink)
	}

	addrPart := u.Opaque
	if addrPart == "" {
		addrPart = strings.TrimPrefix(u.Path, "/")
	}

	idx := strings.IndexByte(addrPart, '?')
	if idx >= 0 {
		addrPart = addrPart[:idx]
	}

	// validate mailto address part is not empty
	addrPart = strings.TrimSpace(addrPart)
	if addrPart == "" {
		return fmt.Errorf("%w: invalid mailto link", internal.ErrInvalidDocumentLink)
	}

	// validate mailto address list is invalid
	_, err := mail.ParseAddressList(addrPart)
	if err != nil {
		return fmt.Errorf("%w: invalid mailto link", internal.ErrInvalidDocumentLink)
	}

	// validate link target is disallowed
	err = validateLinkTargetDisallowed(target)
	if err != nil {
		return err
	}

	return nil
}

// validateWebLink validates a web link href and target.
func validateWebLink(href, target string) error {
	u, parseErr := url.Parse(href)
	if parseErr != nil || u == nil {
		return fmt.Errorf("%w: invalid web link", internal.ErrInvalidDocumentLink)
	}

	err := validateWebLinkNotAbsoluteURL(u)
	if err != nil {
		return err
	}

	err = validateWebURLSchemeDisallowed(u.Scheme)
	if err != nil {
		return err
	}

	err = validateLinkTargetDisallowed(target)
	if err != nil {
		return err
	}

	return nil
}

// validateTextContainsNUL validates a text node contains NUL.
func validateTextContainsNUL(s string) error {
	if !strings.ContainsRune(s, '\x00') {
		return nil
	}

	return fmt.Errorf("%w: text contains NUL", internal.ErrInvalidDocumentNode)
}

// validateVariableNameContainsNUL validates a variable name contains NUL.
func validateVariableNameContainsNUL(s string) error {
	if !strings.ContainsRune(s, '\x00') {
		return nil
	}

	return fmt.Errorf("%w: variable name contains NUL", internal.ErrInvalidDocumentVariableAttrs)
}

// validateLinkContainsNUL validates a link href and title contains NUL.
func validateLinkContainsNUL(href, title string) error {
	if !strings.ContainsRune(href, '\x00') && !strings.ContainsRune(title, '\x00') {
		return nil
	}

	return fmt.Errorf("%w: link contains NUL", internal.ErrInvalidDocumentLink)
}

// validateVariableNameTooLong validates a variable name is too long.
func validateVariableNameTooLong(name string) error {
	if utf8.RuneCountInString(name) <= MaxVariableNameRunes {
		return nil
	}

	return fmt.Errorf("%w: variable name too long", internal.ErrInvalidDocumentVariableAttrs)
}

// validateLinkHrefTooLong validates a link href is too long.
func validateLinkHrefTooLong(href string) error {
	if utf8.RuneCountInString(href) <= MaxLinkHrefRunes {
		return nil
	}
	return fmt.Errorf("%w: link href too long", internal.ErrInvalidDocumentLink)
}

// validateLinkTitleTooLong validates a link title is too long.
func validateLinkTitleTooLong(title string) error {
	if utf8.RuneCountInString(title) <= MaxLinkTitleRunes {
		return nil
	}

	return fmt.Errorf("%w: link title too long", internal.ErrInvalidDocumentLink)
}

// validateWebLinkNotAbsoluteURL validates a web link is not an absolute URL.
func validateWebLinkNotAbsoluteURL(u *url.URL) error {
	if u.Scheme != "" && u.Host != "" {
		return nil
	}

	return fmt.Errorf("%w: link href must be absolute URL", internal.ErrInvalidDocumentLink)
}

// parseHeadingLevel parses attrs["level"] from JSON (float64, int, or int64) into a valid heading level 1-6.
// It rejects non-numeric types, NaN/Inf, and non-integer floats.
func parseHeadingLevel(v any) (int, error) {
	inRange := func(level int) bool { return level >= 1 && level <= 6 }

	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, fmt.Errorf("%w: heading level must be an integer 1-6", internal.ErrInvalidDocumentHeading)
		}
		if x != math.Trunc(x) {
			return 0, fmt.Errorf("%w: heading level must be an integer 1-6", internal.ErrInvalidDocumentHeading)
		}
		if !inRange(int(x)) {
			return 0, fmt.Errorf("%w: heading level must be 1-6, got %d", internal.ErrInvalidDocumentHeading, int(x))
		}
		return int(x), nil
	case int:
		if !inRange(x) {
			return 0, fmt.Errorf("%w: heading level must be 1-6, got %d", internal.ErrInvalidDocumentHeading, x)
		}
		return x, nil
	case int64:
		level := int(x)
		if level < 1 || level > 6 {
			return 0, fmt.Errorf("%w: heading level must be 1-6, got %d", internal.ErrInvalidDocumentHeading, level)
		}
		return level, nil
	default:
		return 0, fmt.Errorf("%w: heading level must be a number", internal.ErrInvalidDocumentHeading)
	}
}

// validateVariableNameRuneInvalid validates a variable name rune is invalid.
func validateVariableNameRuneInvalid(r rune) error {
	if unicode.IsControl(r) || unicode.IsSpace(r) {
		return fmt.Errorf("%w: variable name contains invalid characters", internal.ErrInvalidDocumentVariableAttrs)
	}

	return nil
}

// validateWebURLSchemeDisallowed validates a web URL scheme is disallowed.
func validateWebURLSchemeDisallowed(scheme string) error {
	s := strings.ToLower(scheme)
	if s == schemeHTTP || s == schemeHTTPS {
		return nil
	}

	return fmt.Errorf("%w: link scheme not allowed", internal.ErrInvalidDocumentLink)
}

// isMailtoScheme reports whether scheme is a non-empty mailto scheme, case-insensitive.
func isMailtoScheme(scheme string) bool {
	if scheme == "" {
		return false
	}

	return strings.EqualFold(scheme, strings.TrimSuffix(schemeMailtoPrefix, ":"))
}

// validateLinkTargetDisallowed validates a link target is disallowed.
func validateLinkTargetDisallowed(target string) error {
	if target == "" || target == LinkTargetBlank {
		return nil
	}

	return fmt.Errorf("%w: link target not allowed", internal.ErrInvalidDocumentLink)
}

// validateDocumentDepthExceeded validates a document depth is exceeded.
func validateDocumentDepthExceeded(depth int) error {
	if depth <= MaxRichTextDepth {
		return nil
	}

	return fmt.Errorf("%w: document tree too deep", internal.ErrInvalidDocumentTooLarge)
}

// validateDocumentNodeCountExceeded validates a document node count is exceeded.
func validateDocumentNodeCountExceeded(count int) error {
	if count <= MaxRichTextNodeCount {
		return nil
	}

	return fmt.Errorf("%w: too many nodes", internal.ErrInvalidDocumentTooLarge)
}

// validateTextLeafTooLong validates a text leaf is too long.
func validateTextLeafTooLong(runeCount int) error {
	if runeCount <= MaxRichTextLeafRunes {
		return nil
	}

	return fmt.Errorf("%w: text node too long", internal.ErrInvalidDocumentTooLarge)
}

// validateTotalTextBudgetExceeded returns ErrInvalidDocumentTooLarge if currentTotal+additional exceeds MaxRichTextTotalRunes; otherwise nil.
func validateTotalTextBudgetExceeded(currentTotal, additional int) error {
	if currentTotal+additional <= MaxRichTextTotalRunes {
		return nil
	}

	return fmt.Errorf("%w: total text too long", internal.ErrInvalidDocumentTooLarge)
}

// wrapValidateNodeChildError preserves ErrInvalidDocumentTooLarge from child validation; otherwise wraps with ErrInvalidDocumentNode.
func wrapValidateNodeChildError(err error) error {
	if errors.Is(err, internal.ErrInvalidDocumentTooLarge) {
		return err
	}

	return fmt.Errorf("%w: %w", internal.ErrInvalidDocumentNode, err)
}

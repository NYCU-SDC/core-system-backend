package markdown

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"NYCU-SDC/core-system-backend/internal"

	"github.com/microcosm-cc/bluemonday"

	pm "github.com/karitham/prosemirror"
)

// ProcessRequest validates rich text from HTTP APIs. It accepts canonical ProseMirror JSON
// (object root) or a JSON-encoded plain string, which is converted to a single paragraph.
func ProcessRequest(raw []byte) (canonicalJSON []byte, cleanHTML string, err error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return Process(raw)
	}

	if raw[0] == '"' {
		var plain string
		err = json.Unmarshal(raw, &plain)
		if err != nil {
			return nil, "", wrapUnmarshalErr(err)
		}

		return FromPlaintext(plain)
	}

	return Process(raw)
}

// Process validates a ProseMirror JSON document, renders HTML, sanitizes it, and returns canonical JSON.
func Process(raw []byte) (canonicalJSON []byte, cleanHTML string, err error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return []byte(EmptyDocumentJSON), "", nil
	}

	var root pm.Node
	err = json.Unmarshal(raw, &root)
	if err != nil {
		return nil, "", wrapUnmarshalErr(err)
	}

	if root.Type.Name != NodeDoc {
		return nil, "", fmt.Errorf("%w: root type must be doc", internal.ErrInvalidDocumentRoot)
	}

	root = normalizeDoc(root)

	err = validateNode(root)
	if err != nil {
		return nil, "", err
	}

	rawHTML, err := renderHTML(root)
	if err != nil {
		return nil, "", err
	}

	p := bluemonday.UGCPolicy()
	cleanHTML = p.Sanitize(rawHTML)

	canonicalJSON, err = json.Marshal(root)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", internal.ErrInvalidDocumentMarshal, err)
	}

	return canonicalJSON, cleanHTML, nil
}

// normalizeDoc coerces editor payloads into a schema-valid doc without losing block structure.
// Today we accept (but may not render) some nodes like image; we also prevent top-level inline
// nodes (hard_break, text, variable) from invalidating the document by wrapping them into paragraphs.
func normalizeDoc(root pm.Node) pm.Node {
	if root.Type.Name != NodeDoc || root.IsLeaf() {
		return root
	}

	var out []pm.Node
	var inlineBuf []pm.Node

	flushInline := func() {
		if len(inlineBuf) == 0 {
			return
		}
		p := Schema.Nodes[NodeParagraph]
		para, err := p.Create(nil, nil, inlineBuf...)
		if err == nil {
			out = append(out, para)
		}
		inlineBuf = nil
	}

	for _, child := range root.Content.Content {
		switch child.Type.Name {
		case NodeHardBreak, NodeText, NodeVariable:
			// Wrap consecutive inline nodes into a single paragraph.
			inlineBuf = append(inlineBuf, child)
			continue
		default:
			flushInline()
			out = append(out, child)
		}
	}
	flushInline()

	docType := Schema.Nodes[NodeDoc]
	normalized, err := docType.Create(nil, nil, out...)
	if err != nil {
		return root
	}
	return normalized
}

// PlainText extracts visible text from a valid ProseMirror JSON document.
func PlainText(raw []byte) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", nil
	}

	var root pm.Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", wrapUnmarshalErr(err)
	}

	if root.Type.Name != NodeDoc {
		return "", fmt.Errorf("%w: root type must be doc", internal.ErrInvalidDocumentRoot)
	}

	if err := validateNode(root); err != nil {
		return "", err
	}

	var b strings.Builder
	collectText(root, &b)

	return b.String(), nil
}

// FromPlaintext builds a minimal ProseMirror document from plain text.
func FromPlaintext(plain string) (canonicalJSON []byte, html string, err error) {
	textJSON, err := json.Marshal(plain)
	if err != nil {
		return nil, "", err
	}

	raw := fmt.Sprintf(`{"type":%q,"content":[{"type":%q,"content":[{"type":%q,"text":%s}]}]}`,
		NodeDoc, NodeParagraph, NodeText, textJSON)

	return Process([]byte(raw))
}

// PreviewSnippet returns the first maxRunes runes of plain text from a ProseMirror JSON payload.
func PreviewSnippet(raw []byte, maxRunes int) (string, error) {
	pt, err := PlainText(raw)
	if err != nil {
		return "", err
	}

	if maxRunes <= 0 || utf8.RuneCountInString(pt) <= maxRunes {
		return pt, nil
	}

	runes := []rune(pt)

	return string(runes[:maxRunes]), nil
}

// wrapUnmarshalErr maps encoding/json errors to the appropriate sentinel.
// JSON syntax errors → ErrInvalidDocumentJSON.
// Schema errors (unknown node/mark type from karitham) → ErrInvalidDocumentNode.
func wrapUnmarshalErr(err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return fmt.Errorf("%w: %w", internal.ErrInvalidDocumentJSON, err)
	}

	return fmt.Errorf("%w: %w", internal.ErrInvalidDocumentNode, err)
}

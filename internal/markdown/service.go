package markdown

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"NYCU-SDC/core-system-backend/internal"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/microcosm-cc/bluemonday"

	pm "github.com/karitham/prosemirror"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Service struct {
	logger *zap.Logger
	tracer trace.Tracer
}

const maxRichTextBytes = 64 * 1024

func NewService(logger *zap.Logger) *Service {
	return &Service{
		logger: logger,
		tracer: otel.Tracer("markdown/service"),
	}
}

func checkRichTextSize(raw []byte) error {
	if len(raw) > maxRichTextBytes {
		return internal.ErrInvalidDocumentTooLarge
	}
	return nil
}

// ProcessRequest validates rich text from HTTP APIs. It accepts canonical ProseMirror JSON
// (object root) or a JSON-encoded Markdown string, which is parsed to a ProseMirror doc before
// validation and HTML rendering.
func (s *Service) ProcessRequest(ctx context.Context, raw []byte) (canonicalJSON []byte, cleanHTML string, err error) {
	traceCtx, span := s.tracer.Start(ctx, "ProcessRequest")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return s.Process(traceCtx, raw)
	}

	err = checkRichTextSize(raw)
	if err != nil {
		logger.Warn("rich text payload too large", zap.Error(err))
		span.RecordError(err)
		return nil, "", err
	}

	if raw[0] == '"' {
		var markdown string
		err = json.Unmarshal(raw, &markdown)
		if err != nil {
			wrapped := wrapUnmarshalErr(err)
			logger.Error("invalid rich text JSON string", zap.Error(wrapped))
			span.RecordError(wrapped)
			return nil, "", wrapped
		}

		return s.FromMarkdown(traceCtx, markdown)
	}

	return s.Process(traceCtx, raw)
}

// Process validates a ProseMirror JSON document, renders HTML, sanitizes it, and returns canonical JSON.
func (s *Service) Process(ctx context.Context, raw []byte) (canonicalJSON []byte, cleanHTML string, err error) {
	traceCtx, span := s.tracer.Start(ctx, "Process")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return []byte(EmptyDocumentJSON), "", nil
	}

	err = checkRichTextSize(bytes.TrimSpace(raw))
	if err != nil {
		logger.Warn("rich text payload too large", zap.Error(err))
		span.RecordError(err)
		return nil, "", err
	}

	var root pm.Node
	err = json.Unmarshal(raw, &root)
	if err != nil {
		wrapped := wrapUnmarshalErr(err)
		logger.Warn("invalid rich text JSON", zap.Error(wrapped))
		span.RecordError(wrapped)
		return nil, "", wrapped
	}

	if root.Type.Name != NodeDoc {
		err := fmt.Errorf("%w: root type must be doc", internal.ErrInvalidDocumentRoot)
		logger.Warn("invalid rich text root", zap.Error(err))
		span.RecordError(err)
		return nil, "", err
	}

	root = s.normalizeDoc(traceCtx, root)

	err = validateNode(root)
	if err != nil {
		logger.Error("rich text schema validation failed", zap.Error(err))
		span.RecordError(err)
		return nil, "", err
	}

	canonicalJSON, err = json.Marshal(root)
	if err != nil {
		err = fmt.Errorf("%w: %w", internal.ErrInvalidDocumentMarshal, err)
		logger.Error("rich text canonicalization failed", zap.Error(err))
		span.RecordError(err)
		return nil, "", err
	}

	if isVisiblyEmptyDoc(root) {
		return canonicalJSON, "", nil
	}

	rawHTML, err := s.renderHTML(traceCtx, root)
	if err != nil {
		err := fmt.Errorf("%w: render: %w", internal.ErrInvalidDocumentRender, err)
		logger.Error("failed to render", zap.Error(err))
		span.RecordError(err)
		return nil, "", err
	}

	p := bluemonday.UGCPolicy()
	// Needed for in-document links like "#section".
	p.AllowRelativeURLs(true)
	cleanHTML = p.Sanitize(rawHTML)

	return canonicalJSON, cleanHTML, nil
}

func isVisiblyEmptyDoc(root pm.Node) bool {
	if root.Type.Name != NodeDoc {
		return false
	}
	children := root.Content.Content
	if len(children) != 1 {
		return false
	}

	p := children[0]
	if p.Type.Name != NodeParagraph {
		return false
	}
	return len(p.Content.Content) == 0
}

// normalizeDoc coerces editor payloads into a schema-valid doc without losing block structure.
// Today we accept (but may not render) some nodes like image; we also prevent top-level inline
// nodes (hard_break, text, variable) from invalidating the document by wrapping them into paragraphs.
func (s *Service) normalizeDoc(ctx context.Context, root pm.Node) pm.Node {
	traceCtx, span := s.tracer.Start(ctx, "normalizeDoc")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	if root.Type.Name != NodeDoc || root.IsLeaf() {
		return root
	}

	var out []pm.Node
	var inlineBuf []pm.Node

	flushInline := func() {
		if len(inlineBuf) == 0 {
			return
		}
		paragraphType := Schema.Nodes[NodeParagraph]
		paragraph, err := paragraphType.Create(nil, nil, inlineBuf...)
		if err != nil {
			logger.Error("failed to create paragraph", zap.Error(err))
			span.RecordError(err)
			return
		}

		out = append(out, paragraph)
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

	if len(out) == 0 {
		out = append(out, pm.Node{Type: Schema.Nodes[NodeParagraph]})
	}

	docType := Schema.Nodes[NodeDoc]
	normalized, err := docType.Create(nil, nil, out...)
	if err != nil {
		logger.Error("failed to create doc", zap.Error(err))
		span.RecordError(err)
		return root
	}
	return normalized
}

// PlainText extracts visible text from a valid ProseMirror JSON document.
func (s *Service) PlainText(ctx context.Context, raw []byte) (string, error) {
	traceCtx, span := s.tracer.Start(ctx, "PlainText")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", nil
	}

	err := checkRichTextSize(trimmed)
	if err != nil {
		logger.Warn("rich text payload too large", zap.Error(err))
		span.RecordError(err)
		return "", err
	}

	var root pm.Node
	err = json.Unmarshal(raw, &root)
	if err != nil {
		wrapped := wrapUnmarshalErr(err)
		logger.Error("invalid rich text JSON", zap.Error(wrapped))
		span.RecordError(wrapped)
		return "", wrapped
	}

	if root.Type.Name != NodeDoc {
		err := fmt.Errorf("%w: root type must be doc", internal.ErrInvalidDocumentRoot)
		logger.Error("invalid rich text root", zap.Error(err))
		span.RecordError(err)
		return "", err
	}

	root = s.normalizeDoc(traceCtx, root)

	err = validateNode(root)
	if err != nil {
		logger.Error("rich text schema validation failed", zap.Error(err))
		span.RecordError(err)
		return "", err
	}

	var b strings.Builder
	collectText(root, &b)

	return b.String(), nil
}

// FromPlaintext builds a minimal ProseMirror document from plain text.
func (s *Service) FromPlaintext(ctx context.Context, plain string) (canonicalJSON []byte, html string, err error) {
	traceCtx, span := s.tracer.Start(ctx, "FromPlaintext")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	textJSON, err := json.Marshal(plain)
	if err != nil {
		logger.Error("failed to marshal plaintext for rich text", zap.Error(err))
		span.RecordError(err)
		return nil, "", err
	}

	raw := fmt.Sprintf(`{"type":%q,"content":[{"type":%q,"content":[{"type":%q,"text":%s}]}]}`,
		NodeDoc, NodeParagraph, NodeText, textJSON)

	return s.Process(traceCtx, []byte(raw))
}

// FromMarkdown builds a minimal ProseMirror document from Markdown source.
// This is used for API payloads where the rich text field is provided as a JSON string.
func (s *Service) FromMarkdown(ctx context.Context, source string) (canonicalJSON []byte, html string, err error) {
	traceCtx, span := s.tracer.Start(ctx, "FromMarkdown")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	if strings.TrimSpace(source) == "" {
		return s.Process(traceCtx, nil)
	}

	err = checkRichTextSize([]byte(source))
	if err != nil {
		logger.Warn("rich text payload too large", zap.Error(err))
		span.RecordError(err)
		return nil, "", err
	}

	doc := docFromMarkdown(source)
	raw, err := json.Marshal(doc)
	if err != nil {
		logger.Error("failed to marshal markdown doc", zap.Error(err))
		span.RecordError(err)
		return nil, "", err
	}

	return s.Process(traceCtx, raw)
}

// PreviewSnippet returns the first maxRunes runes of plain text from a ProseMirror JSON payload.
func (s *Service) PreviewSnippet(ctx context.Context, raw []byte, maxRunes int) (string, error) {
	traceCtx, span := s.tracer.Start(ctx, "PreviewSnippet")
	defer span.End()

	pt, err := s.PlainText(traceCtx, raw)
	if err != nil {
		span.RecordError(err)
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

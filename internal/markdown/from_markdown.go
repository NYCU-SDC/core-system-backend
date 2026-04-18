package markdown

import (
	"strings"

	pm "github.com/karitham/prosemirror"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// docFromMarkdown parses Markdown into a schema-valid ProseMirror doc.
//
// This is intentionally conservative: we only map a small subset of Markdown
// constructs that round-trip reasonably in our ProseMirror schema.
func docFromMarkdown(source string) pm.Node {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")

	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	reader := text.NewReader([]byte(source))
	root := md.Parser().Parse(reader)

	var blocks []pm.Node
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		switch n.Kind() {
		case ast.KindParagraph:
			para := n.(*ast.Paragraph)
			inlines := inlineNodesFromMarkdown(para, []byte(source), nil)
			if len(inlines) == 0 {
				blocks = append(blocks, pm.Node{Type: Schema.Nodes[NodeParagraph]})
				continue
			}
			blocks = append(blocks, pm.Node{
				Type:    Schema.Nodes[NodeParagraph],
				Content: pm.Fragment{Content: inlines},
			})
		case ast.KindHeading:
			h := n.(*ast.Heading)
			inlines := inlineNodesFromMarkdown(h, []byte(source), nil)
			blocks = append(blocks, pm.Node{
				Type:  Schema.Nodes[NodeHeading],
				Attrs: map[string]any{"level": int(h.Level)},
				Content: pm.Fragment{
					Content: inlines,
				},
			})
		case ast.KindThematicBreak:
			blocks = append(blocks, pm.Node{Type: Schema.Nodes[NodeHorizontalRule]})
		case ast.KindBlockquote:
			// Convert blockquote children into blocks; if empty, drop it.
			var quoteBlocks []pm.Node
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				if c.Kind() == ast.KindParagraph {
					inlines := inlineNodesFromMarkdown(c, []byte(source), nil)
					quoteBlocks = append(quoteBlocks, pm.Node{
						Type:    Schema.Nodes[NodeParagraph],
						Content: pm.Fragment{Content: inlines},
					})
				}
			}
			if len(quoteBlocks) > 0 {
				blocks = append(blocks, pm.Node{
					Type:    Schema.Nodes[NodeBlockquote],
					Content: pm.Fragment{Content: quoteBlocks},
				})
			}
		default:
			// Unknown block -> degrade to plain text paragraph.
			txt := strings.TrimSpace(extractPlainText(n, []byte(source)))
			if txt == "" {
				continue
			}
			blocks = append(blocks, pm.Node{
				Type: Schema.Nodes[NodeParagraph],
				Content: pm.Fragment{Content: []pm.Node{
					{Type: Schema.Nodes[NodeText], Text: txt},
				}},
			})
		}
	}

	if len(blocks) == 0 {
		return pm.Node{Type: Schema.Nodes[NodeDoc], Content: pm.Fragment{Content: nil}}
	}

	return pm.Node{Type: Schema.Nodes[NodeDoc], Content: pm.Fragment{Content: blocks}}
}

func inlineNodesFromMarkdown(parent ast.Node, src []byte, marks []pm.Mark) []pm.Node {
	var out []pm.Node

	appendText := func(s string) {
		if s == "" {
			return
		}
		out = append(out, pm.Node{
			Type:  Schema.Nodes[NodeText],
			Text:  s,
			Marks: cloneMarks(marks),
		})
	}

	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		switch n.Kind() {
		case ast.KindText:
			t := n.(*ast.Text)
			seg := t.Segment
			appendText(string(seg.Value(src)))
			if t.HardLineBreak() || t.SoftLineBreak() {
				out = append(out, pm.Node{
					Type:  Schema.Nodes[NodeHardBreak],
					Marks: cloneMarks(marks),
				})
			}
		case ast.KindString:
			s := n.(*ast.String)
			appendText(string(s.Value))
		case ast.KindEmphasis:
			e := n.(*ast.Emphasis)
			nextMarks := marks
			if e.Level >= 2 {
				nextMarks = append(nextMarks, pm.Mark{Type: Schema.Marks[MarkBold]})
			} else {
				nextMarks = append(nextMarks, pm.Mark{Type: Schema.Marks[MarkItalic]})
			}
			out = append(out, inlineNodesFromMarkdown(e, src, nextMarks)...)
		case ast.KindLink:
			l := n.(*ast.Link)
			href := string(l.Destination)
			linkMark := pm.Mark{
				Type: Schema.Marks[MarkLink],
				Attrs: map[string]any{
					"href":   href,
					"title":  "",
					"target": "",
				},
			}
			nextMarks := append(marks, linkMark)
			out = append(out, inlineNodesFromMarkdown(l, src, nextMarks)...)
		case ast.KindAutoLink:
			l := n.(*ast.AutoLink)
			href := string(l.URL(src))
			linkMark := pm.Mark{
				Type: Schema.Marks[MarkLink],
				Attrs: map[string]any{
					"href":   href,
					"title":  "",
					"target": "",
				},
			}
			nextMarks := append(marks, linkMark)
			out = append(out, pm.Node{
				Type:  Schema.Nodes[NodeText],
				Text:  href,
				Marks: nextMarks,
			})
		case ast.KindCodeSpan:
			cs := n.(*ast.CodeSpan)
			var b strings.Builder
			for c := cs.FirstChild(); c != nil; c = c.NextSibling() {
				switch c.Kind() {
				case ast.KindText:
					t := c.(*ast.Text)
					b.Write(t.Value(src))
				case ast.KindString:
					s := c.(*ast.String)
					b.Write(s.Value)
				default:
					b.WriteString(extractPlainText(c, src))
				}
			}
			txt := b.String()
			nextMarks := append(marks, pm.Mark{Type: Schema.Marks[MarkCode]})
			out = append(out, pm.Node{
				Type:  Schema.Nodes[NodeText],
				Text:  txt,
				Marks: nextMarks,
			})
		default:
			// Inline we don't understand: recurse and/or degrade to plaintext.
			if n.FirstChild() != nil {
				out = append(out, inlineNodesFromMarkdown(n, src, marks)...)
				continue
			}
			appendText(extractPlainText(n, src))
		}
	}

	return out
}

func extractPlainText(n ast.Node, src []byte) string {
	var b strings.Builder
	err := ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node.Kind() {
		case ast.KindText:
			t := node.(*ast.Text)
			b.Write(t.Segment.Value(src))
			if t.HardLineBreak() || t.SoftLineBreak() {
				b.WriteByte('\n')
			}
		case ast.KindString:
			s := node.(*ast.String)
			b.Write(s.Value)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return ""
	}
	return b.String()
}

func cloneMarks(in []pm.Mark) []pm.Mark {
	if len(in) == 0 {
		return nil
	}
	out := make([]pm.Mark, len(in))
	copy(out, in)
	return out
}

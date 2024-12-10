package seabird_discord

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/seabird-chat/seabird-go"
	"github.com/seabird-chat/seabird-go/pb"
)

func maybeContainer(blocks ...*pb.Block) *pb.Block {
	if len(blocks) == 1 {
		return blocks[0]
	}

	return seabird.NewContainerBlock(blocks...)
}

func TextToBlocks(data string) (*pb.Block, error) {
	//var isAction bool

	// If the message starts and ends with an underscore, it's an "action"
	// message. This parsing is actually *more* accurate than Discord's because
	// the /me command blindly adds an _ to the start and end, but it's
	// displayed as normal italics.
	if len(data) > 2 && strings.HasPrefix(data, "_") && strings.HasSuffix(data, "_") {
		//data = strings.TrimPrefix(strings.TrimSuffix(data, "_"), "_")
		//isAction = true
	}

	src := []byte(data)

	reader := text.NewReader(src)

	// This parser roughly approximates Discord's markdown parsing.
	parser := parser.NewParser(
		parser.WithBlockParsers(
			//util.Prioritized(parser.NewSetextHeadingParser(), 100),
			//util.Prioritized(parser.NewThematicBreakParser(), 200),
			util.Prioritized(parser.NewListParser(), 300),
			util.Prioritized(parser.NewListItemParser(), 400),
			//util.Prioritized(parser.NewCodeBlockParser(), 500),
			util.Prioritized(parser.NewATXHeadingParser(
				parser.WithMaxHeadingLevel(3),
			), 600),
			util.Prioritized(parser.NewFencedCodeBlockParser(), 700),
			util.Prioritized(parser.NewBlockquoteParser(), 800),
			//util.Prioritized(NewHTMLBlockParser(), 900),
			util.Prioritized(parser.NewParagraphParser(), 1000),
		),
		parser.WithInlineParsers(
			util.Prioritized(parser.NewCodeSpanParser(), 100),
			util.Prioritized(parser.NewLinkParser(), 200),
			util.Prioritized(parser.NewAutoLinkParser(), 300),
			//util.Prioritized(parser.NewRawHTMLParser(), 400),
			util.Prioritized(parser.NewEmphasisParser(), 500),

			// Custom additions

			// NOTE: underline must be a higher priority than the emphasis
			// parser to work correctly.
			util.Prioritized(newMultiCharInlineParser('_', "Underline"), 450),

			util.Prioritized(newMultiCharInlineParser('|', "Spoiler"), 1000),
			util.Prioritized(newMultiCharInlineParser('~', "Strikethrough"), 1000),

			// We want to convert automatically linkified URLs to a format which
			// seabird understands, just in case. It's better for
			// interoperability.
			util.Prioritized(extension.NewLinkifyParser(), 1000),
		),
		parser.WithParagraphTransformers(
		//util.Prioritized(parser.LinkReferenceParagraphTransformer, 100),
		),
	)
	doc := parser.Parse(reader)

	blocks, err := nodeToBlocks(doc, src)
	if err != nil {
		return nil, err
	}

	/*
		if isAction {
			blocks = []*pb.Block{
				&pb.Block{
					Inner: &pb.Block_Action{
						Action: &pb.ActionBlock{
							Inner: blocks,
						},
					},
				},
			}
		}
	*/

	return maybeContainer(blocks...), nil
}

func nodeToBlocks(doc ast.Node, src []byte) ([]*pb.Block, error) {
	var ret []*pb.Block

	for cur := doc; cur != nil; cur = cur.NextSibling() {
		switch node := cur.(type) {
		// case *ast.Image: // XXX: not supported, send as plain text or error
		// case *ast.ThematicBreak: // XXX: not supported (properly) by Discord
		// case *ast.CodeBlock: // XXX: not supported, send as plain text or error
		// case *ast.HTMLBlock: // XXX: not supported, send as plain text or error
		// case *ast.RawHTML: // XXX: not supported, send as plain text or error

		// Note that Text, String, and TextBlock are all separate entities and
		// they all react differently.

		case *ast.Document:
			nodes, err := nodeToBlocks(cur.FirstChild(), src)
			if err != nil {
				return nil, err
			}
			ret = append(ret, maybeContainer(nodes...))
		case *ast.Paragraph:
			nodes, err := nodeToBlocks(cur.FirstChild(), src)
			if err != nil {
				return nil, err
			}
			ret = append(ret, maybeContainer(nodes...))
		case *ast.Text:
			ret = append(ret, seabird.NewTextBlock(string(node.Value(src))))
		case *ast.String:
			ret = append(ret, seabird.NewTextBlock(string(node.Value)))
		case *ast.AutoLink:
			ret = append(ret, seabird.NewLinkBlock(
				string(node.URL(src)),
				seabird.NewTextBlock(string(node.Label(src))),
			))
		case *ast.Heading:
			nodes, err := nodeToBlocks(cur.FirstChild(), src)
			if err != nil {
				return nil, err
			}

			ret = append(ret, seabird.NewHeadingBlock(node.Level, nodes...))
		case *ast.CodeSpan:
			var buf bytes.Buffer

			for c := cur.FirstChild(); c != nil; c = c.NextSibling() {
				text, ok := c.(*ast.Text)
				if !ok {
					return nil, fmt.Errorf("CodeSpan contained non-text node")
				}

				segment := text.Segment
				value := segment.Value(src)

				// If the value ends in a newline, we remove it, add a space and
				// continue
				if bytes.HasSuffix(value, []byte("\n")) {
					buf.Write(value[:len(value)-1])
					buf.Write([]byte(" "))
				} else {
					buf.Write(value)
				}
			}

			ret = append(ret, seabird.NewInlineCodeBlock(buf.String()))
		case *ast.FencedCodeBlock:
			var buf bytes.Buffer

			lang := string(node.Language(src))

			l := node.Lines().Len()
			for i := 0; i < l; i++ {
				line := cur.Lines().At(i)
				buf.Write(line.Value(src))
			}

			// Trim any trailing newlines to make it a little cleaner
			data := strings.TrimSuffix(buf.String(), "\n")

			ret = append(ret, seabird.NewFencedCodeBlock(lang, data))
		case *ast.Blockquote:
			nodes, err := nodeToBlocks(cur.FirstChild(), src)
			if err != nil {
				return nil, err
			}
			ret = append(ret, seabird.NewBlockquoteBlock(nodes...))
		case *ast.Link:
			nodes, err := nodeToBlocks(cur.FirstChild(), src)
			if err != nil {
				return nil, err
			}
			ret = append(ret, seabird.NewLinkBlock(string(node.Destination), nodes...))
		case *ast.List:
			nodes, err := nodeToBlocks(cur.FirstChild(), src)
			if err != nil {
				return nil, err
			}
			ret = append(ret, seabird.NewListBlock(nodes...))
		case *ast.ListItem:
			nodes, err := nodeToBlocks(cur.FirstChild(), src)
			if err != nil {
				return nil, err
			}
			ret = append(ret, maybeContainer(nodes...))
		case *ast.TextBlock:
			nodes, err := nodeToBlocks(cur.FirstChild(), src)
			if err != nil {
				return nil, err
			}
			ret = append(ret, maybeContainer(nodes...))
		case *ast.Emphasis:
			nodes, err := nodeToBlocks(cur.FirstChild(), src)
			if err != nil {
				return nil, err
			}

			if node.Level == 2 {
				ret = append(ret, seabird.NewBoldBlock(nodes...))
			} else {
				ret = append(ret, seabird.NewItalicsBlock(nodes...))
			}
		case *multiCharDelimiterNode:
			nodes, err := nodeToBlocks(cur.FirstChild(), src)
			if err != nil {
				return nil, err
			}

			if node.BaseChar == '~' {
				ret = append(ret, seabird.NewStrikethroughBlock(nodes...))
			} else if node.BaseChar == '|' {
				ret = append(ret, seabird.NewSpoilerBlock(nodes...))
			} else if node.BaseChar == '_' {
				ret = append(ret, seabird.NewUnderlineBlock(nodes...))
			} else {
				return nil, fmt.Errorf("unknown delimiter: %c", node.BaseChar)
			}
		default:
			return nil, fmt.Errorf("unknown or unsupported node type: %T", node)
		}
	}

	return ret, nil
}

// ScanDelimiter scans a multi-character delimiter by given DelimiterProcessor.
// This was originally based off parser.ScanDelimiter, but has been simplified
// and tweaked to work better with how spoiler and strikethrough blocks work in
// Discord to the point that it now no longer resembles the original.
func scanMultiCharDelimiter(line []byte, targetLen int, processor parser.DelimiterProcessor) *parser.Delimiter {
	if len(line) < targetLen {
		return nil
	}

	c := line[0]

	if !processor.IsDelimiter(c) {
		return nil
	}

	for _, c2 := range line[1:targetLen] {
		if c != c2 {
			return nil
		}
	}

	return parser.NewDelimiter(true, true, targetLen, c, processor)
}

type multiCharInlineParser struct {
	baseChar  byte
	processor *multiCharDelimiterProcessor
}

// newMultiCharInlineParser is sort of a port of
// extension.NewStrikethroughParser, but generalized so it can work with
// multiple types of characters, allowing for support of underline,
// strikethrough, and spoiler tags with the same code.
func newMultiCharInlineParser(baseChar byte, kind string) parser.InlineParser {
	return &multiCharInlineParser{
		baseChar: baseChar,
		processor: &multiCharDelimiterProcessor{
			baseChar: baseChar,
			kind:     ast.NewNodeKind(kind),
		},
	}
}

func (p *multiCharInlineParser) Trigger() []byte {
	return []byte{p.baseChar, p.baseChar}
}

func (p *multiCharInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, segment := block.PeekLine()

	// If the last delimiter ended where we're starting and matches the char we
	// care about, bail.
	lastDelim := pc.LastDelimiter()
	if lastDelim != nil && lastDelim.Char == p.baseChar {
		_, curSeg := block.Position()
		if curSeg.Start == lastDelim.Segment.Stop {
			return nil
		}
	}

	node := scanMultiCharDelimiter(line, 2, p.processor)
	if node == nil {
		return nil
	}

	node.Segment = segment.WithStop(segment.Start + node.OriginalLength)
	block.Advance(node.OriginalLength)
	pc.PushDelimiter(node)

	return node
}

type multiCharDelimiterProcessor struct {
	baseChar byte
	kind     ast.NodeKind
}

func (p *multiCharDelimiterProcessor) IsDelimiter(b byte) bool {
	return b == p.baseChar
}

func (p *multiCharDelimiterProcessor) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	return opener.Char == closer.Char && opener.Length == closer.Length
}

func (p *multiCharDelimiterProcessor) OnMatch(consumes int) ast.Node {
	return newMultiCharDelimiterNode(consumes, p.baseChar, p.kind)
}

type multiCharDelimiterNode struct {
	ast.BaseInline
	Level    int
	BaseChar byte
	kind     ast.NodeKind
}

func newMultiCharDelimiterNode(level int, baseChar byte, kind ast.NodeKind) ast.Node {
	return &multiCharDelimiterNode{
		Level:    level,
		BaseChar: baseChar,
		kind:     kind,
	}
}

// Dump implements Node.Dump.
func (n *multiCharDelimiterNode) Dump(source []byte, level int) {
	m := map[string]string{
		"Level":    fmt.Sprintf("%v", n.Level),
		"BaseChar": string(n.BaseChar),
	}
	ast.DumpHelper(n, source, level, m, nil)
}

// Kind implements Node.Kind.
func (n *multiCharDelimiterNode) Kind() ast.NodeKind {
	return n.kind
}

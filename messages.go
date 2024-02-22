package seabird_discord

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/seabird-chat/seabird-go/pb"
)

func TextToBlocks(data string) []*pb.Block {
	var isAction bool

	// If the message starts and ends with an underscore, it's an "action"
	// message. This parsing is actually *more* accurate than Discord's because
	// the /me command blindly adds an _ to the start and end, but it's
	// displayed as normal italics.
	if len(data) > 2 && strings.HasPrefix(data, "_") && strings.HasSuffix(data, "_") {
		data = strings.TrimPrefix(strings.TrimSuffix(data, "_"), "_")
		isAction = true
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
			util.Prioritized(parser.NewCodeBlockParser(), 500),
			util.Prioritized(parser.NewATXHeadingParser(), 600),
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
			util.Prioritized(newMultiCharInlineParser('|', "Spoiler"), 1000),
			util.Prioritized(newMultiCharInlineParser('~', "Strikethrough"), 1000),
		),
		parser.WithParagraphTransformers(
			util.Prioritized(parser.LinkReferenceParagraphTransformer, 100),
		),
	)
	doc := parser.Parse(reader)

	blocks := nodeToBlocks(doc.FirstChild())

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

	return blocks
}

func nodeToBlocks(doc ast.Node) []*pb.Block {
	var ret []*pb.Block

	fmt.Println(doc.Type())

	return ret
}

// ScanDelimiter scans a multi-character delimiter by given DelimiterProcessor.
// This was originally based off parser.ScanDelimiter, but has been simplified
// and tweaked to work better with how spoiler and strikethrough blocks work in
// Discord to the point that it now no longer resembles the original.
func ScanMultiCharDelimiter(line []byte, targetLen int, processor parser.DelimiterProcessor) *parser.Delimiter {
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

	node := ScanMultiCharDelimiter(line, 2, p.processor)
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
	return newMultiCharDelimiterNode(consumes, p.kind)
}

type multiCharDelimiterNode struct {
	ast.BaseInline
	Level int
	kind  ast.NodeKind
}

func newMultiCharDelimiterNode(level int, kind ast.NodeKind) ast.Node {
	return &multiCharDelimiterNode{
		Level: level,
		kind:  kind,
	}
}

// Dump implements Node.Dump.
func (n *multiCharDelimiterNode) Dump(source []byte, level int) {
	m := map[string]string{
		"Level": fmt.Sprintf("%v", n.Level),
	}
	ast.DumpHelper(n, source, level, m, nil)
}

// Kind implements Node.Kind.
func (n *multiCharDelimiterNode) Kind() ast.NodeKind {
	return n.kind
}

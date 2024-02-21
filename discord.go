package seabird_discord

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog"
	"github.com/seabird-chat/seabird-go/pb"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func TextToBlocks(data string) []*pb.Block {
	src := []byte(data)

	// TODO:
	// - Strikethrough
	// - Spoiler

	reader := text.NewReader(src)
	parser := parser.NewParser(
		parser.WithBlockParsers(
			//util.Prioritized(NewSetextHeadingParser(), 100),
			//util.Prioritized(parser.NewThematicBreakParser(), 200),
			util.Prioritized(parser.NewListParser(), 300),
			//util.Prioritized(NewListItemParser(), 400),
			util.Prioritized(parser.NewCodeBlockParser(), 500),
			//util.Prioritized(NewATXHeadingParser(), 600),
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
		),
		parser.WithParagraphTransformers(
			util.Prioritized(parser.LinkReferenceParagraphTransformer, 100),
		),
	)
	doc := parser.Parse(reader)

	/// TODO: remove debug text
	doc.Dump(src, 0)

	return nodeToBlocks(doc)
}

func nodeToBlocks(doc ast.Node) []*pb.Block {
	return nil
}

// ComesFromDM returns true if a message comes from a DM channel
func ComesFromDM(s *discordgo.Session, m *discordgo.MessageCreate) (bool, error) {
	channel, err := s.State.Channel(m.ChannelID)
	if err != nil {
		if channel, err = s.Channel(m.ChannelID); err != nil {
			return false, err
		}
	}

	return channel.Type == discordgo.ChannelTypeDM, nil
}

// ActionText tries to guess if a message was an action. It returns the action
// text without formatting and if it thinks the text was an action.
func ActionText(rawText string) (string, bool) {
	text := strings.TrimPrefix(strings.TrimSuffix(rawText, "_"), "_")

	if len(text) != len(rawText)-2 {
		return text, false
	}

	return text, !strings.Contains(text, "_")
}

func ReplaceMentions(l zerolog.Logger, s *discordgo.Session, m *discordgo.Message) string {
	rawText, err := m.ContentWithMoreMentionsReplaced(s)
	if err != nil {
		l.Warn().Err(err).Msg("failed to replace mentions, falling back to less agressive mentions")
		rawText = m.ContentWithMentionsReplaced()
	}

	return rawText
}

package seabird_discord

import (
	"strings"
	"testing"
	"time"

	"github.com/seabird-chat/seabird-go"
	"github.com/seabird-chat/seabird-go/pb"
	"github.com/stretchr/testify/assert"
)

func TestRenderMessage(t *testing.T) {
	testCases := []struct {
		name     string
		text     string
		block    *pb.Block
		expected string
	}{
		{
			name:     "no-block",
			text:     "*raw* text",
			expected: "*raw* text",
		},
		{
			name:     "bare-text-block-is-raw",
			text:     "**raw** text",
			block:    seabird.NewTextBlock("**raw** text"),
			expected: "**raw** text",
		},
		{
			name: "archipelago-item-send",
			block: seabird.NewContainerBlock(
				seabird.NewBoldBlock(seabird.NewTextBlock("Alice")),
				seabird.NewTextBlock(" sent "),
				seabird.NewItalicsBlock(seabird.NewTextBlock("Master Sword")),
				seabird.NewTextBlock(" to "),
				seabird.NewBoldBlock(seabird.NewTextBlock("Bob")),
			),
			expected: "**Alice** sent *Master Sword* to **Bob**",
		},
		{
			name: "formatting",
			block: seabird.NewContainerBlock(
				seabird.NewUnderlineBlock(seabird.NewTextBlock("u")),
				seabird.NewStrikethroughBlock(seabird.NewTextBlock("s")),
				seabird.NewSpoilerBlock(seabird.NewTextBlock("p")),
				seabird.NewInlineCodeBlock("c"),
			),
			expected: "__u__~~s~~||p||`c`",
		},
		{
			name: "nested",
			block: seabird.NewBoldBlock(
				seabird.NewItalicsBlock(seabird.NewTextBlock("hi")),
			),
			expected: "***hi***",
		},
		{
			name: "edge-whitespace-moves-outside",
			block: seabird.NewContainerBlock(
				seabird.NewTextBlock("a"),
				seabird.NewItalicsBlock(seabird.NewTextBlock(" b ")),
				seabird.NewTextBlock("c"),
			),
			expected: "a *b* c",
		},
		{
			name:     "empty-formatting",
			block:    seabird.NewContainerBlock(seabird.NewBoldBlock(), seabird.NewTextBlock("x")),
			expected: "x",
		},
		{
			name: "escapes-text",
			block: seabird.NewItalicsBlock(
				seabird.NewTextBlock(`*a_b~c|d` + "`" + `e\f>g#h[i]`),
			),
			expected: `*\*a\_b\~c\|d` + "\\`" + `e\\f>g#h\[i\]*`,
		},
		{
			name: "escapes-line-start",
			block: seabird.NewContainerBlock(
				seabird.NewTextBlock("> not a quote\n# not a heading"),
				seabird.NewItalicsBlock(seabird.NewTextBlock("x")),
			),
			expected: "\\> not a quote\n\\# not a heading*x*",
		},
		{
			name: "urls-not-escaped",
			block: seabird.NewContainerBlock(
				seabird.NewBoldBlock(seabird.NewTextBlock("see")),
				seabird.NewTextBlock(" https://x.com/a_b*c and_this"),
			),
			expected: `**see** https://x.com/a_b*c and\_this`,
		},
		{
			name:     "inline-code-with-backticks",
			block:    seabird.NewContainerBlock(seabird.NewInlineCodeBlock("a``b"), seabird.NewInlineCodeBlock("`x")),
			expected: "```a``b```" + "`` `x ``",
		},
		{
			name: "fenced-code",
			block: seabird.NewContainerBlock(
				seabird.NewTextBlock("code:"),
				seabird.NewFencedCodeBlock("go", "x := *y"),
			),
			expected: "code:\n```go\nx := *y\n```",
		},
		{
			name: "links",
			block: seabird.NewContainerBlock(
				seabird.NewLinkBlock("https://seabird.chat", seabird.NewTextBlock("seabird")),
				seabird.NewTextBlock(" "),
				seabird.NewLinkBlock("https://seabird.chat", seabird.NewTextBlock("https://seabird.chat")),
			),
			expected: "[seabird](<https://seabird.chat>) https://seabird.chat",
		},
		{
			name: "block-level",
			block: seabird.NewContainerBlock(
				seabird.NewHeadingBlock(2, seabird.NewTextBlock("Title")),
				seabird.NewTextBlock("intro"),
				seabird.NewListBlock(seabird.NewTextBlock("one"), seabird.NewTextBlock("two")),
				seabird.NewBlockquoteBlock(seabird.NewTextBlock("quoted")),
			),
			expected: "## Title\nintro\n- one\n- two\n> quoted",
		},
		{
			name: "multiline-blockquote",
			block: seabird.NewBlockquoteBlock(
				seabird.NewTextBlock("hello\n"),
				seabird.NewTextBlock("world"),
			),
			expected: "> hello\n> world",
		},
		{
			name:     "timestamp",
			block:    seabird.NewTimestampBlock(time.Unix(1700000000, 0)),
			expected: "<t:1700000000:f>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, renderMessage(tc.text, tc.block, nil))
		})
	}
}

func TestRenderMessageReplacesBeforeEscaping(t *testing.T) {
	replace := strings.NewReplacer("@foo_bar", "<@123>").Replace

	block := seabird.NewContainerBlock(
		seabird.NewTextBlock("hi @foo_bar and @baz_qux "),
		seabird.NewItalicsBlock(seabird.NewTextBlock("item")),
	)
	assert.Equal(t, `hi <@123> and @baz\_qux *item*`, renderMessage("", block, replace))
	assert.Equal(t, "hi <@123>", renderMessage("hi @foo_bar", nil, replace))
}

func TestRoundTrip(t *testing.T) {
	for _, input := range []string{
		"**Alice** sent *Master Sword* to **Bob**",
		"__u__ ~~s~~ ||p|| `c`",
		"[seabird](https://seabird.chat) and - *ab*",
		"> hello\n> world",
		"hello\nworld",
	} {
		block, _, err := TextToBlock(input)
		assert.NoError(t, err)

		out := renderMessage(block.Plain, block, nil)
		again, _, err := TextToBlock(out)
		assert.NoError(t, err)
		assert.Equal(t, block.Plain, again.Plain, "input %q rendered as %q", input, out)
	}
}

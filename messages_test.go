package seabird_discord

import (
	"testing"

	"github.com/seabird-chat/seabird-go"
	"github.com/seabird-chat/seabird-go/pb"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestTextToBlocks(t *testing.T) {
	var testCases = []struct {
		name     string
		input    string
		expected *pb.Block
		isAction bool
	}{
		// Simple Cases
		{
			name:  "simple-text",
			input: "hello world",
			expected: seabird.NewContainerBlock(
				seabird.NewTextBlock("hello"),
				seabird.NewTextBlock(" world"),
			),
		},
		{
			name:  "italics-simple",
			input: "*hello world*",
			expected: seabird.NewItalicsBlock(
				seabird.NewTextBlock("hello world"),
			),
		},
		{
			name:  "link-simple",
			input: "[hello](world)",
			expected: seabird.NewLinkBlock("world",
				seabird.NewTextBlock("hello"),
			),
		},
		{
			name:  "linkify-simple",
			input: "hello https://seabird.chat",
			expected: seabird.NewContainerBlock(
				seabird.NewTextBlock("hello "),
				seabird.NewLinkBlock(
					"https://seabird.chat",
					seabird.NewTextBlock("https://seabird.chat"),
				),
			),
		},
		{
			name:     "inline-code-simple",
			input:    "`hello world`",
			expected: seabird.NewInlineCodeBlock("hello world"),
		},
		{
			name:  "strikethrough-simple",
			input: "~~hello world~~",
			expected: seabird.NewStrikethroughBlock(
				seabird.NewTextBlock("hello world"),
			),
		},
		{
			name:  "spoiler-simple",
			input: "||hello world||",
			expected: seabird.NewSpoilerBlock(
				seabird.NewTextBlock("hello world"),
			),
		},
		{
			// NOTE: we can't start and end the underline test with a _ because
			// then it would count as an action.
			name:  "underline-simple",
			input: "start __hello world__ end",
			expected: seabird.NewContainerBlock(
				seabird.NewTextBlock("start "),
				seabird.NewUnderlineBlock(
					seabird.NewTextBlock("hello world"),
				),
				seabird.NewTextBlock(" end"),
			),
		},
		{
			name:  "list-simple",
			input: "* hello\n* world",
			expected: seabird.NewListBlock(
				seabird.NewTextBlock("hello"),
				seabird.NewTextBlock("world"),
			),
		},
		{
			name:  "fenced-code-simple",
			input: "```python\nprint('hello world')\n```",
			expected: seabird.NewFencedCodeBlock(
				"python",
				"print('hello world')",
			),
		},
		{
			name:  "blockquote-simple",
			input: "> hello world",
			expected: seabird.NewBlockquoteBlock(
				seabird.NewContainerBlock(
					// TODO: the container here is a side effect of the Linkify
					// extension, but ideally it shouldn't exist. Maybe we can
					// re-merge the text blocks.
					seabird.NewTextBlock("hello"),
					seabird.NewTextBlock(" world"),
				),
			),
		},
		{
			name:     "heading-simple",
			input:    "# 1",
			expected: seabird.NewHeadingBlock(1, seabird.NewTextBlock("1")),
		},

		// Complex Cases
		{
			name:  "heading-all",
			input: "# 1\n## 2\n### 3\n#### 4",
			expected: seabird.NewContainerBlock(
				seabird.NewHeadingBlock(1, seabird.NewTextBlock("1")),
				seabird.NewHeadingBlock(2, seabird.NewTextBlock("2")),
				seabird.NewHeadingBlock(3, seabird.NewTextBlock("3")),

				// TODO: the container here is a side effect of the Linkify
				// extension, but ideally it shouldn't exist. Maybe we can
				// re-merge the text blocks.
				seabird.NewContainerBlock(
					seabird.NewTextBlock("####"),
					seabird.NewTextBlock(" 4"),
				),
			),
		},

		{
			name:  "blockquote-newlines",
			input: "> hello world\n>\n> post-blank",
			expected: seabird.NewBlockquoteBlock(
				// TODO: the container here is a side effect of the Linkify
				// extension, but ideally it shouldn't exist. Maybe we can
				// re-merge the text blocks.
				seabird.NewContainerBlock(
					seabird.NewTextBlock("hello"),
					seabird.NewTextBlock(" world"),
				),
				seabird.NewTextBlock("post-blank"),
			),
		},
		{
			name:  "list-nested",
			input: "* hello\n  * world\n    1. ordered\n    2. list",
			expected: seabird.NewListBlock(
				seabird.NewContainerBlock(
					seabird.NewTextBlock("hello"),
					seabird.NewListBlock(
						seabird.NewContainerBlock(
							seabird.NewTextBlock("world"),
							seabird.NewListBlock(
								seabird.NewTextBlock("ordered"),
								seabird.NewTextBlock("list"),
							),
						),
					),
				),
			),
		},
		{
			name:  "strikethrough-complex",
			input: "~a~ ~hello~ ~~~world~~~ ~~~~~asdf~~~~~",
			expected: seabird.NewContainerBlock(
				seabird.NewTextBlock("~a~ ~hello~ "),
				seabird.NewStrikethroughBlock(
					seabird.NewTextBlock("~world"),
				),
				seabird.NewTextBlock("~ "),
				seabird.NewStrikethroughBlock(
					seabird.NewTextBlock("~"),
				),
				seabird.NewTextBlock("asdf"),
				seabird.NewStrikethroughBlock(
					seabird.NewTextBlock("~"),
				),
			),
		},
		{
			name:  "spoiler-complex",
			input: "|a| |hello| |||world||| |||||asdf|||||",
			expected: seabird.NewContainerBlock(
				seabird.NewTextBlock("|a| |hello| "),
				seabird.NewSpoilerBlock(
					seabird.NewTextBlock("|world"),
				),
				seabird.NewTextBlock("| "),
				seabird.NewSpoilerBlock(
					seabird.NewTextBlock("|"),
				),
				seabird.NewTextBlock("asdf"),
				seabird.NewSpoilerBlock(
					seabird.NewTextBlock("|"),
				),
			),
		},
		{
			name:  "bold-and-italics-complex",
			input: "*a* *hello* ***world*** *****asdf*****",
			expected: seabird.NewContainerBlock(
				seabird.NewItalicsBlock(
					seabird.NewTextBlock("a"),
				),
				seabird.NewTextBlock(" "),
				seabird.NewItalicsBlock(
					seabird.NewTextBlock("hello"),
				),
				seabird.NewTextBlock(" "),
				seabird.NewItalicsBlock(
					seabird.NewBoldBlock(
						seabird.NewTextBlock("world"),
					),
				),
				seabird.NewTextBlock(" "),
				seabird.NewItalicsBlock(
					seabird.NewBoldBlock(
						seabird.NewBoldBlock(
							seabird.NewTextBlock("asdf"),
						),
					),
				),
			),
		},
		{
			name:  "why-we-cant-have-nice-things",
			input: "~~strike **bold** _italic__under___~~ in ||spoiled **bold**||",
			expected: seabird.NewContainerBlock(
				seabird.NewStrikethroughBlock(
					seabird.NewTextBlock("strike "),
					seabird.NewBoldBlock(
						seabird.NewTextBlock("bold"),
					),
					seabird.NewTextBlock(" "),
					seabird.NewItalicsBlock(
						seabird.NewTextBlock("italic"),
						seabird.NewUnderlineBlock(
							seabird.NewTextBlock("under"),
						),
					),
				),
				seabird.NewTextBlock(" in "),
				seabird.NewSpoilerBlock(
					seabird.NewTextBlock("spoiled "),
					seabird.NewBoldBlock(
						seabird.NewTextBlock("bold"),
					),
				),
			),
		},
		{
			name:  "action-simple",
			input: "_hello world_",

			// TODO: the container here is a side effect of the Linkify
			// extension, but ideally it shouldn't exist. Maybe we can
			// re-merge the text blocks.
			expected: seabird.NewContainerBlock(
				seabird.NewTextBlock("hello"),
				seabird.NewTextBlock(" world"),
			),
			isAction: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			blocks, isAction, err := TextToBlocks(testCase.input)
			assert.NoError(t, err)
			assert.Equal(t, isAction, testCase.isAction)
			expected, err := protojson.Marshal(testCase.expected)
			assert.NoError(t, err)
			blockJson, err := protojson.Marshal(blocks)
			assert.NoError(t, err)
			assert.NotNil(t, blocks)
			assert.JSONEq(t, string(expected), string(blockJson))
		})
	}
}

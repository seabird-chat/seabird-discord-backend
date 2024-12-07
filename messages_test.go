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
	}{
		// Simple Cases
		{
			name:     "simple-text",
			input:    "hello world",
			expected: seabird.NewTextBlock("hello world"),
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
			name:  "underline-simple",
			input: "__hello world__",
			expected: seabird.NewUnderlineBlock(
				seabird.NewTextBlock("hello world"),
			),
		},

		// Complex Cases
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
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			blocks := TextToBlocks(testCase.input)
			expected, err := protojson.Marshal(testCase.expected)
			assert.NoError(t, err)
			blockJson, err := protojson.Marshal(blocks)
			assert.NoError(t, err)
			assert.NotNil(t, blocks)
			assert.JSONEq(t, string(expected), string(blockJson))
		})
	}
}

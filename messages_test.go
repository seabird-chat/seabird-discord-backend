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
		input    string
		expected *pb.Block
	}{
		// Simple Cases
		{
			input: "hello world",
			expected: seabird.NewContainerBlock(
				seabird.NewContainerBlock(
					seabird.NewContainerBlock(
						seabird.NewTextBlock("hello world"),
					),
				),
			),
		},
		{
			input: "*hello world*",
			expected: seabird.NewContainerBlock(
				seabird.NewContainerBlock(
					seabird.NewContainerBlock(
						seabird.NewItalicsBlock(
							seabird.NewTextBlock("hello world"),
						),
					),
				),
			),
		},
		{
			input: "[hello](world)",
			expected: seabird.NewContainerBlock(
				seabird.NewContainerBlock(
					seabird.NewContainerBlock(
						seabird.NewLinkBlock("world",
							seabird.NewTextBlock("hello"),
						),
					),
				),
			),
		},
		/*
			{
				input: "__hello world__",
				expected: seabird.NewContainerBlock(
					seabird.NewContainerBlock(
						seabird.NewContainerBlock(
							seabird.NewUnderlineBlock(
								seabird.NewTextBlock("hello world"),
							),
						),
					),
				),
			},
		*/

		// Complex Cases
		{
			input: "~a~ ~hello~ ~~~world~~~ ~~~~~asdf~~~~~",
			expected: seabird.NewContainerBlock(
				seabird.NewContainerBlock(
					seabird.NewContainerBlock(
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
				),
			),
		},
		{
			input: "|a| |hello| |||world||| |||||asdf|||||",
			expected: seabird.NewContainerBlock(
				seabird.NewContainerBlock(
					seabird.NewContainerBlock(
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
				),
			),
		},
		{
			input: "*a* *hello* ***world*** *****asdf*****",
			expected: seabird.NewContainerBlock(
				seabird.NewContainerBlock(
					seabird.NewContainerBlock(
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
				),
			),
		},
	}

	for _, testCase := range testCases {
		blocks := TextToBlocks(testCase.input)
		expected, err := protojson.Marshal(testCase.expected)
		assert.NoError(t, err)
		blockJson, err := protojson.Marshal(blocks)
		assert.NoError(t, err)
		assert.NotNil(t, blocks)
		assert.JSONEq(t, string(expected), string(blockJson))
	}
}

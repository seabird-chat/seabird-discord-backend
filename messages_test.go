package seabird_discord

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTextToBlocks(t *testing.T) {
	var testCases = []struct {
		input string
	}{
		//{
		//	input: "asd ~~~~~ asd",
		//},
		{
			input: "~a~ ~hello~ ~~~world~~~ ~~~~~asdf~~~~~",
		},
		//{
		//	input: "*a* *hello* ***world*** *****asdf*****",
		//},
	}

	for _, testCase := range testCases {
		assert.NotNil(t, TextToBlocks(testCase.input))
	}
}

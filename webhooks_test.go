package seabird_discord

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/seabird-chat/seabird-go"
	"github.com/seabird-chat/seabird-go/pb"
	"github.com/stretchr/testify/assert"
)

func TestAuthorLabel(t *testing.T) {
	alice := &pb.User{DisplayName: "alice"}

	assert.Equal(t, "alice (IRC)", authorLabel(alice, &pb.Backend{Type: "irc", DisplayName: "IRC"}))
	assert.Equal(t, "alice (minecraft)", authorLabel(alice, &pb.Backend{Type: "minecraft"}))
	assert.Equal(t, "alice", authorLabel(alice, nil))
}

func TestWithAuthor(t *testing.T) {
	testCases := []struct {
		name     string
		text     string
		block    *pb.Block
		action   bool
		expected string
	}{
		{
			name:     "text",
			text:     "hi *there*",
			expected: `**alice (IRC)**: hi \*there\*`,
		},
		{
			name:     "block",
			block:    seabird.NewContainerBlock(seabird.NewTextBlock("hi "), seabird.NewItalicsBlock(seabird.NewTextBlock("there"))),
			expected: "**alice (IRC)**: hi *there*",
		},
		{
			name:     "action",
			text:     "waves",
			block:    seabird.NewTextBlock("waves"),
			action:   true,
			expected: "\\* **alice (IRC)** waves",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			block := withAuthor("alice (IRC)", tc.text, tc.block, tc.action)
			assert.Equal(t, tc.expected, renderMessage("", block, nil))
		})
	}
}

func TestIsUnknownWebhook(t *testing.T) {
	restErr := func(status, code int) error {
		err := &discordgo.RESTError{Response: &http.Response{StatusCode: status}}
		if code != 0 {
			err.Message = &discordgo.APIErrorMessage{Code: code}
		}
		return fmt.Errorf("failed to post: %w", err)
	}

	assert.True(t, isUnknownWebhook(restErr(http.StatusNotFound, discordgo.ErrCodeUnknownWebhook)))
	assert.True(t, isUnknownWebhook(restErr(http.StatusNotFound, 0)))
	assert.False(t, isUnknownWebhook(restErr(http.StatusBadRequest, discordgo.ErrCodeInvalidFormBody)))
	assert.False(t, isUnknownWebhook(fmt.Errorf("connection reset")))
}

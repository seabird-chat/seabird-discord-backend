package seabird_discord

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog"
)

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
		// rawText = m.ContentWithMentionsReplaced()
	}

	return rawText
}

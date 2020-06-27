package seabird_discord

import (
	"strings"

	"github.com/bwmarrin/discordgo"
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

func ActionText(rawText string) (string, bool) {
	text := strings.TrimPrefix(strings.TrimSuffix(rawText, "_"), "_")

	if len(text) != len(rawText)-2 {
		return text, false
	}

	return text, !strings.Contains(text, "_")
}

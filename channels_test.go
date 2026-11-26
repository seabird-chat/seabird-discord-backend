package seabird_discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/seabird-chat/seabird-go/pb"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestChannelEvents(t *testing.T) {
	join := func(id, name, topic string) *pb.ChatEvent {
		return &pb.ChatEvent{Inner: &pb.ChatEvent_JoinChannel{JoinChannel: &pb.JoinChannelChatEvent{
			ChannelId: id, DisplayName: name, Topic: topic,
		}}}
	}
	change := func(id, name, topic string) *pb.ChatEvent {
		return &pb.ChatEvent{Inner: &pb.ChatEvent_ChangeChannel{ChangeChannel: &pb.ChangeChannelChatEvent{
			ChannelId: id, DisplayName: name, Topic: topic,
		}}}
	}
	leave := func(id string) *pb.ChatEvent {
		return &pb.ChatEvent{Inner: &pb.ChatEvent_LeaveChannel{LeaveChannel: &pb.LeaveChannelChatEvent{
			ChannelId: id,
		}}}
	}

	var testCases = []struct {
		name    string
		sent    map[string]channelInfo
		current map[string]channelInfo
		want    []*pb.ChatEvent
	}{
		{
			name:    "new stream sends every channel",
			current: map[string]channelInfo{"2": {Name: "b"}, "1": {Name: "a", Topic: "hi"}},
			want:    []*pb.ChatEvent{join("1", "a", "hi"), join("2", "b", "")},
		},
		{
			name:    "nothing changed",
			sent:    map[string]channelInfo{"1": {Name: "a"}},
			current: map[string]channelInfo{"1": {Name: "a"}},
		},
		{
			name:    "renamed, created and deleted",
			sent:    map[string]channelInfo{"1": {Name: "a"}, "2": {Name: "b"}},
			current: map[string]channelInfo{"1": {Name: "renamed"}, "3": {Name: "c"}},
			want:    []*pb.ChatEvent{change("1", "renamed", ""), join("3", "c", ""), leave("2")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := channelEvents(tc.sent, tc.current)
			assert.Equal(t, len(tc.want), len(got), "%v", got)
			for i := range min(len(tc.want), len(got)) {
				assert.True(t, proto.Equal(tc.want[i], got[i]), "event %d: want %v, got %v", i, tc.want[i], got[i])
			}
		})
	}
}

func TestCurrentChannelsSkipsHidden(t *testing.T) {
	const guildID, botID, roleID = "g", "bot", "role"

	deny := func(id string) []*discordgo.PermissionOverwrite {
		return []*discordgo.PermissionOverwrite{
			{ID: guildID, Type: discordgo.PermissionOverwriteTypeRole, Deny: discordgo.PermissionViewChannel},
			{ID: id, Type: discordgo.PermissionOverwriteTypeRole, Allow: discordgo.PermissionViewChannel},
		}
	}

	state := discordgo.NewState()
	state.User = &discordgo.User{ID: botID}
	err := state.GuildAdd(&discordgo.Guild{
		ID: guildID,
		Roles: []*discordgo.Role{
			// @everyone shares the guild's ID.
			{ID: guildID, Permissions: discordgo.PermissionViewChannel},
			{ID: roleID},
		},
		Channels: []*discordgo.Channel{
			{ID: "visible", GuildID: guildID, Name: "general", Type: discordgo.ChannelTypeGuildText},
			{ID: "hidden", GuildID: guildID, Name: "___hidden___", Type: discordgo.ChannelTypeGuildText,
				PermissionOverwrites: deny("someone-else")},
			{ID: "role-only", GuildID: guildID, Name: "staff", Type: discordgo.ChannelTypeGuildText,
				PermissionOverwrites: deny(roleID)},
			{ID: "voice", GuildID: guildID, Name: "voice", Type: discordgo.ChannelTypeGuildVoice},
		},
		Members: []*discordgo.Member{{GuildID: guildID, User: &discordgo.User{ID: botID}, Roles: []string{roleID}}},
	})
	assert.NoError(t, err)

	b := &Backend{discord: &discordgo.Session{State: state}}
	assert.Equal(t, map[string]channelInfo{
		"visible":   {Name: "general"},
		"role-only": {Name: "staff"},
	}, b.currentChannels())
}

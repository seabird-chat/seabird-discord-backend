package seabird_discord

import (
	"testing"

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

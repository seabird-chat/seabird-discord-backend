package seabird_discord

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/seabird-chat/seabird-discord-backend/pb"
)

type DiscordConfig struct {
	Logger        zerolog.Logger
	CommandPrefix string
	DiscordToken  string
	SeabirdID     string
	SeabirdHost   string
	SeabirdToken  string
}

type Backend struct {
	id             string
	cmdPrefix      string
	logger         zerolog.Logger
	discord        *discordgo.Session
	grpc           pb.ChatIngestClient
	ingestSendLock sync.Mutex
	ingestStream   pb.ChatIngest_IngestEventsClient
}

func New(config DiscordConfig) (*Backend, error) {
	var err error

	b := &Backend{
		id:        config.SeabirdID,
		logger:    config.Logger,
		cmdPrefix: config.CommandPrefix,
	}

	b.grpc, err = newGRPCClient(config.SeabirdHost, config.SeabirdToken)
	if err != nil {
		return nil, err
	}

	b.discord, err = discordgo.New(config.DiscordToken)
	if err != nil {
		return nil, err
	}

	b.discord.AddHandler(b.handleMessageCreate)
	b.discord.AddHandler(b.handleGuildCreate)
	b.discord.AddHandler(b.handleGuildDelete)
	//b.discord.AddHandler(b.handleChannelEdit)
	b.discord.AddHandler(b.handleDiscordLog)

	return b, nil
}

func (b *Backend) handleGuildCreate(s *discordgo.Session, m *discordgo.GuildCreate) {
	for _, channel := range m.Channels {
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}

		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_JoinChannel{JoinChannel: &pb.JoinChannelChatEvent{
			ChannelId:   channel.ID,
			DisplayName: channel.Name,
			Topic:       channel.Topic,
		}}})
	}
}

func (b *Backend) handleGuildDelete(s *discordgo.Session, m *discordgo.GuildDelete) {
	for _, channel := range m.Channels {
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}

		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_LeaveChannel{LeaveChannel: &pb.LeaveChannelChatEvent{
			ChannelId: channel.ID,
		}}})
	}
}

func (b *Backend) handleChannelEdit(s *discordgo.Session, m *discordgo.ChannelUpdate) {
	fmt.Printf("%+v\n", m)

	/*
		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_ChangeChannel{ChangeChannel: &pb.ChangeChannelChatEvent{
			ChannelId: m.ID,
		}}})
	*/
}

func (b *Backend) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself This isn't required but
	// it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	rawText := m.ContentWithMentionsReplaced()

	ok, err := ComesFromDM(s, m)
	if err != nil {
		b.logger.Warn().Err(err).Msg("failed to determine if message is private")
		return
	}

	if ok {
		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_PrivateMessage{PrivateMessage: &pb.PrivateMessageEvent{
			Source: &pb.User{
				Id:          m.ChannelID,
				DisplayName: m.Author.Username,
			},
			Text: rawText,
		}}})
		return
	}

	source := &pb.ChannelSource{
		ChannelId: m.ChannelID,
		User: &pb.User{
			Id:          m.Author.ID,
			DisplayName: m.Author.Username,
		},
	}

	if strings.HasPrefix(rawText, b.cmdPrefix) {
		msgParts := strings.SplitN(rawText, " ", 2)
		if len(msgParts) < 2 {
			msgParts = append(msgParts, "")
		}

		command := strings.TrimPrefix(msgParts[0], b.cmdPrefix)
		arg := msgParts[1]

		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_Command{Command: &pb.CommandEvent{
			Source:  source,
			Command: command,
			Arg:     arg,
		}}})
		return
	}

	mentionPrefix := fmt.Sprintf("<@!%s> ", s.State.User.ID)
	if strings.HasPrefix(m.Content, mentionPrefix) {
		msg := *m.Message
		msg.Content = strings.TrimPrefix(m.Content, mentionPrefix)

		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_Mention{Mention: &pb.MentionEvent{
			Source: source,
			Text:   msg.ContentWithMentionsReplaced(),
		}}})
		return
	}

	b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_Message{Message: &pb.MessageEvent{
		Source: source,
		Text:   rawText,
	}}})
}

func (b *Backend) handleDiscordLog(s *discordgo.Session, m interface{}) {
	if _, ok := m.(*discordgo.Event); ok {
		return
	}

	b.logger.Debug().Str("msg_type", fmt.Sprintf("%+T", m)).Msgf("%+v", m)
}

func (b *Backend) writeSuccess(id string) {
	b.writeEvent(&pb.ChatEvent{
		Id:    id,
		Inner: &pb.ChatEvent_Success{Success: &pb.SuccessChatEvent{}},
	})
}

func (b *Backend) writeFailure(id string, reason string) {
	b.writeEvent(&pb.ChatEvent{
		Id:    id,
		Inner: &pb.ChatEvent_Failed{Failed: &pb.FailedChatEvent{Reason: reason}},
	})
}

func (b *Backend) writeEvent(e *pb.ChatEvent) {
	b.ingestSendLock.Lock()
	defer b.ingestSendLock.Unlock()

	err := b.ingestStream.Send(e)
	if err != nil {
		b.logger.Warn().Err(err).Msg("failed to send event")
	}
}

func (b *Backend) handleIngest(ctx context.Context) error {
	for {
		msg, err := b.ingestStream.Recv()
		if err != nil {
			return err
		}

		switch v := msg.Inner.(type) {
		case *pb.ChatRequest_SendMessage:
			_, err = b.discord.ChannelMessageSend(v.SendMessage.ChannelId, v.SendMessage.Text)
		case *pb.ChatRequest_SendPrivateMessage:
			// TODO: this might not work
			_, err = b.discord.ChannelMessageSend(v.SendPrivateMessage.UserId, v.SendPrivateMessage.Text)
		case *pb.ChatRequest_JoinChannel:
			err = errors.New("unimplemented for discord")
		case *pb.ChatRequest_LeaveChannel:
			err = errors.New("unimplemented for discord")
		case *pb.ChatRequest_UpdateChannelInfo:
			_, err = b.discord.ChannelEditComplex(v.UpdateChannelInfo.ChannelId, &discordgo.ChannelEdit{
				Topic: v.UpdateChannelInfo.Topic,
			})
		default:
			b.logger.Warn().Msgf("unknown msg type: %T", msg.Inner)
		}

		if msg.Id != "" {
			if err != nil {
				b.writeFailure(msg.Id, err.Error())
			} else {
				b.writeSuccess(msg.Id)
			}
		}
	}
}

func (b *Backend) Run() error {
	var err error
	errGroup, ctx := errgroup.WithContext(context.Background())

	// TODO: this is an ugly place for this. It also means that calling Run
	// multiple times will break and cause race conditions. This shouldn't
	// happen in practice, but it's good to remember.
	b.ingestStream, err = b.grpc.IngestEvents(ctx)
	if err != nil {
		return err
	}

	err = b.ingestStream.Send(&pb.ChatEvent{
		Inner: &pb.ChatEvent_Hello{
			Hello: &pb.HelloChatEvent{
				BackendInfo: &pb.Backend{
					Type: "discord",
					Id:   b.id,
				},
			},
		},
	})
	if err != nil {
		return err
	}

	errGroup.Go(func() error { return b.handleIngest(ctx) })
	errGroup.Go(func() error {
		err := b.discord.Open()
		defer b.discord.Close()
		if err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	})

	return errGroup.Wait()
}

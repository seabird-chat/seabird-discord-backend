package seabird_discord

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	seabird "github.com/seabird-chat/seabird-go"
	"github.com/seabird-chat/seabird-go/pb"
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
	id           string
	cmdPrefix    string
	logger       zerolog.Logger
	discord      *discordgo.Session
	grpc         *seabird.ChatIngestClient
	outputStream chan *pb.ChatEvent
}

func New(config DiscordConfig) (*Backend, error) {
	var err error

	client, err := seabird.NewChatIngestClient(config.SeabirdHost, config.SeabirdToken)

	b := &Backend{
		id:           config.SeabirdID,
		logger:       config.Logger,
		cmdPrefix:    config.CommandPrefix,
		grpc:         client,
		outputStream: make(chan *pb.ChatEvent, 10),
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
	// Ignore all messages created by the bot itself. This is a requirement from
	// the chat ingest API.
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
		if text, ok := ActionText(rawText); ok {
			b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_PrivateAction{PrivateAction: &pb.PrivateActionEvent{
				Source: &pb.User{
					Id:          m.ChannelID,
					DisplayName: m.Author.Username,
				},
				Text: text,
			}}})
		} else {
			b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_PrivateMessage{PrivateMessage: &pb.PrivateMessageEvent{
				Source: &pb.User{
					Id:          m.ChannelID,
					DisplayName: m.Author.Username,
				},
				Text: rawText,
			}}})
		}
		return
	}

	source := &pb.ChannelSource{
		ChannelId: m.ChannelID,
		User: &pb.User{
			Id:          m.Author.ID,
			DisplayName: m.Author.Username,
		},
	}

	if text, ok := ActionText(rawText); ok {
		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_Action{Action: &pb.ActionEvent{
			Source: source,
			Text:   text,
		}}})
		return
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
	// Note that we need to allow events to be dropped so we don't lose the
	// connection when the gRPC service is down.
	select {
	case b.outputStream <- e:
	default:
	}
}

func (b *Backend) handleIngest(ctx context.Context) {
	ingestStream, err := b.grpc.IngestEvents("discord", b.id)
	if err != nil {
		b.logger.Warn().Err(err).Msg("got error while calling ingest events")
		return
	}

	for {
		select {
		case event := <-b.outputStream:
			b.logger.Debug().Msgf("Sending event: %+v", event)

			err := ingestStream.Send(event)
			if err != nil {
				b.logger.Warn().Err(err).Msgf("got error while sending event: %+v", event)
				return
			}

		case msg, ok := <-ingestStream.C:
			if !ok {
				b.logger.Warn().Err(errors.New("ingest stream ended")).Msg("unexpected end of ingest stream")
				return
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
}

func (b *Backend) runGrpc(ctx context.Context) error {
	for {
		b.handleIngest(ctx)

		// If the context exited, we're shutting down
		err := ctx.Err()
		if err != nil {
			return err
		}

		time.Sleep(5 * time.Second)
	}
}

func (b *Backend) Run() error {
	errGroup, ctx := errgroup.WithContext(context.Background())

	errGroup.Go(func() error { return b.runGrpc(ctx) })
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

package seabird_discord

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	seabird "github.com/seabird-chat/seabird-go"
	"github.com/seabird-chat/seabird-go/pb"
)

type DiscordConfig struct {
	Logger                zerolog.Logger
	CommandPrefix         string
	DiscordToken          string
	SeabirdID             string
	SeabirdHost           string
	SeabirdToken          string
	DiscordChannelMapping string
}

type Backend struct {
	id                    string
	cmdPrefix             string
	logger                zerolog.Logger
	discord               *discordgo.Session
	grpc                  *seabird.ChatIngestClient
	seabird               *seabird.Client
	outputStream          chan *pb.ChatEvent
	guildMentionCacheLock sync.Mutex
	guildMentionCache     map[string]*strings.Replacer

	channelMap   map[string]string
	userMapping  map[string]string
	channelCount map[string]int
}

func New(config DiscordConfig) (*Backend, error) {
	var err error

	ciClient, err := seabird.NewChatIngestClient(config.SeabirdHost, config.SeabirdToken)
	if err != nil {
		return nil, err
	}

	sbClient, err := seabird.NewClient(config.SeabirdHost, config.SeabirdToken)
	if err != nil {
		return nil, err
	}

	b := &Backend{
		id:                config.SeabirdID,
		logger:            config.Logger,
		cmdPrefix:         config.CommandPrefix,
		grpc:              ciClient,
		seabird:           sbClient,
		outputStream:      make(chan *pb.ChatEvent, 10),
		guildMentionCache: make(map[string]*strings.Replacer),
		channelMap:        make(map[string]string),
		userMapping:       make(map[string]string),
		channelCount:      make(map[string]int),
	}

	// Convert the channel mapping into a useful format
	if config.DiscordChannelMapping != "" {
		for _, item := range strings.Split(config.DiscordChannelMapping, ",") {
			split := strings.SplitN(item, ":", 2)
			if len(split) != 2 {
				return nil, errors.New("invalid channel mapping")
			}

			b.channelMap[split[0]] = split[1]
		}
	}

	b.discord, err = discordgo.New(config.DiscordToken)
	if err != nil {
		return nil, err
	}

	// Ideally we wouldn't need any additional intents, but in order to see all
	// users for the mention cache, we need to have the GuildMembers and
	// GuildPresences intents. The first makes it so we can see users, the
	// second sends them with the GuildCreateEvent.
	b.discord.Identify.Intents = discordgo.MakeIntent(
		discordgo.IntentsAllWithoutPrivileged |
			discordgo.IntentsGuildMembers |
			discordgo.IntentsGuildPresences)

	b.discord.AddHandler(b.handleMessageCreate)
	b.discord.AddHandler(b.handleGuildCreate)
	b.discord.AddHandler(b.handleGuildDelete)
	//b.discord.AddHandler(b.handleChannelEdit)
	b.discord.AddHandler(b.handleVoiceStateUpdate)
	b.discord.AddHandler(b.handleDiscordLog)

	b.discord.AddHandler(b.handleGuildMemberAdd)
	b.discord.AddHandler(b.handleGuildMemberUpdate)
	b.discord.AddHandler(b.handleGuildMemberRemove)
	b.discord.AddHandler(b.handleGuildMembersChunk)

	return b, nil
}

func (b *Backend) markGuildMentionCacheStale(guildId string) {
	b.guildMentionCacheLock.Lock()
	defer b.guildMentionCacheLock.Unlock()

	delete(b.guildMentionCache, guildId)
}

func (b *Backend) handleGuildMemberAdd(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	b.markGuildMentionCacheStale(m.GuildID)
}

func (b *Backend) handleGuildMemberUpdate(s *discordgo.Session, m *discordgo.GuildMemberUpdate) {
	b.markGuildMentionCacheStale(m.GuildID)
}

func (b *Backend) handleGuildMemberRemove(s *discordgo.Session, m *discordgo.GuildMemberRemove) {
	b.markGuildMentionCacheStale(m.GuildID)
}

func (b *Backend) handleGuildMembersChunk(s *discordgo.Session, m *discordgo.GuildMembersChunk) {
	b.markGuildMentionCacheStale(m.GuildID)
}

func (b *Backend) getReplacer(guildId string) *strings.Replacer {
	b.guildMentionCacheLock.Lock()
	defer b.guildMentionCacheLock.Unlock()

	if _, ok := b.guildMentionCache[guildId]; !ok {
		var candidates []string
		g, err := b.discord.State.Guild(guildId)
		if err != nil {
			return strings.NewReplacer()
		}

		for _, m := range g.Members {
			candidates = append(candidates, "@"+m.User.Username, m.User.Mention())
		}

		b.guildMentionCache[guildId] = strings.NewReplacer(candidates...)
	}

	return b.guildMentionCache[guildId]
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

	b.handleMessageCreateImpl(s, m)

	// All attachments count as regular message events
	source := &pb.ChannelSource{
		ChannelId: m.ChannelID,
		User: &pb.User{
			Id:          m.Author.ID,
			DisplayName: m.Author.Username,
		},
	}

	for _, a := range m.Attachments {
		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_Message{Message: &pb.MessageEvent{
			Source: source,
			Text:   fmt.Sprintf("%s: %s", a.Filename, a.URL),
		}}})
	}
}

func (b *Backend) handleMessageCreateImpl(s *discordgo.Session, m *discordgo.MessageCreate) {
	ok, err := ComesFromDM(s, m)
	if err != nil {
		b.logger.Warn().Err(err).Msg("failed to determine if message is private")
		return
	}

	rawText := ReplaceMentions(b.logger, s, m.Message)
	if rawText == "" {
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

	mentionPrefix := fmt.Sprintf("<@%s>", s.State.User.ID)
	if strings.HasPrefix(m.Content, mentionPrefix) {
		msg := *m.Message
		msg.Content = strings.TrimSpace(strings.TrimPrefix(m.Content, mentionPrefix))

		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_Mention{Mention: &pb.MentionEvent{
			Source: source,
			Text:   ReplaceMentions(b.logger, s, &msg),
		}}})
		return
	}

	b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_Message{Message: &pb.MessageEvent{
		Source: source,
		Text:   rawText,
	}}})
}

func (b *Backend) sendJoinNotification(s *discordgo.Session, guildID, userID, channelID string, count int) {
	if count != 1 {
		return
	}

	userInfo, err := s.State.Member(guildID, userID)
	if err != nil {
		fmt.Println(err)
		return
	}
	userName := userInfo.Nick
	if userName == "" {
		if userInfo.User != nil {
			userName = userInfo.User.Username
		} else {
			userName = "Someone"
		}
	}

	channelInfo, err := s.State.Channel(channelID)
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	_, err = b.seabird.Inner.SendMessage(ctx, &pb.SendMessageRequest{
		ChannelId: b.channelMap[channelID],
		Text:      fmt.Sprintf("%s has joined voice channel %s", userName, channelInfo.Mention()),
	})
	if err != nil {
		fmt.Println(err)
		return
	}
}

func (b *Backend) handleVoiceStateUpdate(s *discordgo.Session, m *discordgo.VoiceStateUpdate) {
	prevChannel := b.userMapping[m.UserID]
	targetChannel := m.ChannelID

	if targetChannel == "" {
		delete(b.userMapping, m.UserID)
	} else {
		b.userMapping[m.UserID] = targetChannel
	}

	// If the user changed channels
	if prevChannel != targetChannel {
		if prevChannel != "" {
			b.channelCount[prevChannel] -= 1

			if b.channelCount[prevChannel] == 0 {
				delete(b.channelCount, prevChannel)
			}
		}

		if targetChannel != "" && b.channelMap[targetChannel] != "" {
			b.channelCount[targetChannel] += 1

			b.sendJoinNotification(s, m.GuildID, m.UserID, targetChannel, b.channelCount[targetChannel])
		}
	}
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
				msgText := v.SendMessage.Text
				if c, err := b.discord.State.Channel(v.SendMessage.ChannelId); err == nil {
					msgText = b.getReplacer(c.GuildID).Replace(msgText)
				} else {
					b.logger.Warn().Err(err).Msg("Tried to send message to unknown channel")
				}
				_, err = b.discord.ChannelMessageSendComplex(v.SendMessage.ChannelId, &discordgo.MessageSend{
					Content: msgText,
					Flags:   discordgo.MessageFlagsSuppressEmbeds,
				})
			case *pb.ChatRequest_SendPrivateMessage:
				// TODO: this might not work
				_, err = b.discord.ChannelMessageSendComplex(v.SendPrivateMessage.UserId, &discordgo.MessageSend{
					Content: v.SendPrivateMessage.Text,
					Flags:   discordgo.MessageFlagsSuppressEmbeds,
				})
			case *pb.ChatRequest_PerformAction:
				msgText := v.PerformAction.Text
				if c, err := b.discord.State.Channel(v.PerformAction.ChannelId); err == nil {
					msgText = b.getReplacer(c.GuildID).Replace(msgText)
				} else {
					b.logger.Warn().Err(err).Msg("Tried to send message to unknown channel")
				}
				_, err = b.discord.ChannelMessageSendComplex(v.PerformAction.ChannelId, &discordgo.MessageSend{
					Content: "_" + msgText + "_",
					Flags:   discordgo.MessageFlagsSuppressEmbeds,
				})
			case *pb.ChatRequest_PerformPrivateAction:
				// TODO: this might not work
				_, err = b.discord.ChannelMessageSendComplex(v.PerformPrivateAction.UserId, &discordgo.MessageSend{
					Content: "_" + v.PerformPrivateAction.Text + "_",
					Flags:   discordgo.MessageFlagsSuppressEmbeds,
				})
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
		b.logger.Warn().Msg("Ingest exited")

		// If the context exited, we're shutting down
		err := ctx.Err()
		if err != nil {
			b.logger.Info().Msg("Bot is shutting down, exiting runGrpc")
			return err
		}

		b.logger.Info().Msg("Sleeping 5 seconds before trying ingest again")
		time.Sleep(5 * time.Second)
	}
}

func (b *Backend) Run() error {
	errGroup, ctx := errgroup.WithContext(context.Background())

	errGroup.Go(func() error {
		return b.runGrpc(ctx)
	})
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

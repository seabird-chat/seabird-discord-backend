package seabird_discord

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
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

	// channelsChanged tells the ingest stream that the channels in
	// discord.State changed. Changes are compared against what was last
	// sent, so any number of them collapse into one signal.
	channelsChanged chan struct{}

	guildMentionCacheLock sync.Mutex
	guildMentionCache     map[string]*strings.Replacer

	// webhooks are what messages on someone's behalf are posted through, by
	// channel ID. ownWebhooks is every webhook ID they've used, so their posts
	// aren't sent back to core.
	webhookLock     sync.Mutex
	webhooks        map[string]*discordgo.Webhook
	webhookFailures map[string]time.Time
	ownWebhooks     map[string]bool
}

func New(config DiscordConfig) (*Backend, error) {
	var err error

	ciClient, err := seabird.NewChatIngestClient(config.SeabirdHost, config.SeabirdToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create new chat ingest client: %w", err)
	}

	b := &Backend{
		id:                config.SeabirdID,
		logger:            config.Logger,
		cmdPrefix:         config.CommandPrefix,
		grpc:              ciClient,
		outputStream:      make(chan *pb.ChatEvent, 10),
		channelsChanged:   make(chan struct{}, 1),
		guildMentionCache: make(map[string]*strings.Replacer),
		webhooks:          make(map[string]*discordgo.Webhook),
		webhookFailures:   make(map[string]time.Time),
		ownWebhooks:       make(map[string]bool),
	}

	b.discord, err = discordgo.New(config.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create discord client: %w", err)
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
	// discordgo updates its state before calling handlers, so these only need
	// to say that something changed.
	b.discord.AddHandler(func(*discordgo.Session, *discordgo.GuildCreate) { b.markChannelsChanged() })
	b.discord.AddHandler(func(*discordgo.Session, *discordgo.GuildDelete) { b.markChannelsChanged() })
	b.discord.AddHandler(func(*discordgo.Session, *discordgo.ChannelCreate) { b.markChannelsChanged() })
	b.discord.AddHandler(func(*discordgo.Session, *discordgo.ChannelUpdate) { b.markChannelsChanged() })
	b.discord.AddHandler(func(*discordgo.Session, *discordgo.ChannelDelete) { b.markChannelsChanged() })

	// Which channels the bot can view depends on its roles too.
	b.discord.AddHandler(func(*discordgo.Session, *discordgo.GuildRoleCreate) { b.markChannelsChanged() })
	b.discord.AddHandler(func(*discordgo.Session, *discordgo.GuildRoleUpdate) { b.markChannelsChanged() })
	b.discord.AddHandler(func(*discordgo.Session, *discordgo.GuildRoleDelete) { b.markChannelsChanged() })
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

	// The bot's own roles decide which channels it can view.
	if m.User != nil && s.State.User != nil && m.User.ID == s.State.User.ID {
		b.markChannelsChanged()
	}
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

// channelReplacer returns a func that converts @username to mentions for the
// channel's guild.
func (b *Backend) channelReplacer(channelID string) func(string) string {
	c, err := b.discord.State.Channel(channelID)
	if err != nil {
		b.logger.Warn().Err(err).Msg("Tried to send message to unknown channel")
		return nil
	}
	return b.getReplacer(c.GuildID).Replace
}

// markChannelsChanged tells the ingest stream to send core any channel changes.
func (b *Backend) markChannelsChanged() {
	select {
	case b.channelsChanged <- struct{}{}:
	default:
		// A change is already waiting to be sent.
	}
}

// channelInfo is what core is told about a channel.
type channelInfo struct {
	Name  string
	Topic string
}

// currentChannels returns every text channel the bot can see, by ID.
// Channels the bot can't view are left out: discord sends them with a
// placeholder name and nothing can be read or sent there.
func (b *Backend) currentChannels() map[string]channelInfo {
	state := b.discord.State

	state.RLock()
	var botID string
	if state.User != nil {
		botID = state.User.ID
	}
	channels := make(map[string]channelInfo)
	for _, guild := range state.Guilds {
		for _, channel := range guild.Channels {
			if channel.Type == discordgo.ChannelTypeGuildText {
				channels[channel.ID] = channelInfo{Name: channel.Name, Topic: channel.Topic}
			}
		}
	}
	state.RUnlock()

	// Checking permissions takes the state lock, so it waits until the
	// channels have been read.
	for id := range channels {
		if !canView(state, botID, id) {
			delete(channels, id)
		}
	}
	return channels
}

// canView reports whether userID can see a channel. Without enough state to
// tell, it assumes they can.
func canView(state *discordgo.State, userID, channelID string) bool {
	permissions, err := state.UserChannelPermissions(userID, channelID)
	if err != nil {
		return true
	}
	return permissions&discordgo.PermissionViewChannel != 0
}

// channelEvents returns the events that take core from the channels in sent
// to the ones in current.
func channelEvents(sent, current map[string]channelInfo) []*pb.ChatEvent {
	var events []*pb.ChatEvent
	for _, id := range slices.Sorted(maps.Keys(current)) {
		channel := current[id]
		old, known := sent[id]
		switch {
		case !known:
			events = append(events, &pb.ChatEvent{Inner: &pb.ChatEvent_JoinChannel{JoinChannel: &pb.JoinChannelChatEvent{
				ChannelId:   id,
				DisplayName: channel.Name,
				Topic:       channel.Topic,
			}}})
		case old != channel:
			events = append(events, &pb.ChatEvent{Inner: &pb.ChatEvent_ChangeChannel{ChangeChannel: &pb.ChangeChannelChatEvent{
				ChannelId:   id,
				DisplayName: channel.Name,
				Topic:       channel.Topic,
			}}})
		}
	}
	for _, id := range slices.Sorted(maps.Keys(sent)) {
		if _, ok := current[id]; !ok {
			events = append(events, &pb.ChatEvent{Inner: &pb.ChatEvent_LeaveChannel{LeaveChannel: &pb.LeaveChannelChatEvent{
				ChannelId: id,
			}}})
		}
	}
	return events
}

func (b *Backend) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself. This is a requirement from
	// the chat ingest API.
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Messages posted through the backend's webhooks came from core, so they
	// are ignored too.
	if m.WebhookID != "" && b.isOwnWebhook(m.WebhookID) {
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
	fromDM, err := ComesFromDM(s, m)
	if err != nil {
		b.logger.Warn().Err(err).Msg("failed to determine if message is private")
		return
	}

	rawText := ReplaceMentions(b.logger, s, m.Message)
	if rawText == "" {
		return
	}

	if fromDM {
		rootBlock, isAction, err := TextToBlock(rawText)
		if err != nil {
			b.logger.Warn().Err(err).Msg("failed to convert message to blocks")
			return
		}

		if isAction {
			b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_PrivateAction{PrivateAction: &pb.PrivateActionEvent{
				Source: &pb.User{
					Id:          m.ChannelID,
					DisplayName: m.Author.Username,
				},
				RootBlock: rootBlock,
			}}})
		} else {
			b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_PrivateMessage{PrivateMessage: &pb.PrivateMessageEvent{
				Source: &pb.User{
					Id:          m.ChannelID,
					DisplayName: m.Author.Username,
				},
				RootBlock: rootBlock,
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

	// Special case - if the message started with the command prefix, we do much
	// less parsing and processing on it.
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

	// Special case - if the original message started with the bot's user ID,
	// make sure we trim that off before processing as a mention event.
	mentionPrefix := fmt.Sprintf("<@%s>", s.State.User.ID)
	if strings.HasPrefix(m.Content, mentionPrefix) {
		msg := *m.Message
		msg.Content = strings.TrimSpace(strings.TrimPrefix(m.Content, mentionPrefix))

		rootBlock, _, err := TextToBlock(ReplaceMentions(b.logger, s, &msg))
		if err != nil {
			b.logger.Warn().Err(err).Msg("failed to convert message to blocks")
			return
		}

		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_Mention{Mention: &pb.MentionEvent{
			Source:    source,
			RootBlock: rootBlock,
		}}})
		return
	}

	rootBlock, isAction, err := TextToBlock(rawText)
	if err != nil {
		b.logger.Warn().Err(err).Msg("failed to convert message to blocks")
		return
	}

	if isAction {
		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_Action{Action: &pb.ActionEvent{
			Source:    source,
			RootBlock: rootBlock,
		}}})
	} else {
		b.writeEvent(&pb.ChatEvent{Inner: &pb.ChatEvent_Message{Message: &pb.MessageEvent{
			Source:    source,
			RootBlock: rootBlock,
		}}})
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
	ingestStream, err := b.grpc.IngestEventsWithHello(&pb.HelloChatEvent{
		BackendInfo: &pb.HelloChatEvent_Backend{Type: "discord", Id: b.id, DisplayName: "Discord"},
		Features:    []pb.BackendFeature{pb.BackendFeature_BACKEND_FEATURE_AUTHOR},
	})
	if err != nil {
		b.logger.Warn().Err(err).Msg("got error while calling ingest events")
		return
	}

	// Core forgets a backend's channels when its stream ends, so each stream
	// starts from nothing and is sent every channel, then whatever changes.
	var sent map[string]channelInfo
	syncChannels := func() error {
		current := b.currentChannels()
		for _, event := range channelEvents(sent, current) {
			if err := ingestStream.Send(event); err != nil {
				return err
			}
		}
		sent = current
		return nil
	}
	if err := syncChannels(); err != nil {
		b.logger.Warn().Err(err).Msg("got error while sending channels")
		return
	}

	for {
		select {
		case <-b.channelsChanged:
			if err := syncChannels(); err != nil {
				b.logger.Warn().Err(err).Msg("got error while sending channels")
				return
			}

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
				req := v.SendMessage
				err = b.sendToChannel(req.ChannelId, req.Text, req.RootBlock, false, req.Author, req.AuthorBackend)
			case *pb.ChatRequest_SendPrivateMessage:
				// TODO: this might not work
				_, err = b.discord.ChannelMessageSendComplex(v.SendPrivateMessage.UserId, &discordgo.MessageSend{
					Content: renderMessage(v.SendPrivateMessage.Text, v.SendPrivateMessage.RootBlock, nil),
					Flags:   discordgo.MessageFlagsSuppressEmbeds,
				})
			case *pb.ChatRequest_PerformAction:
				req := v.PerformAction
				err = b.sendToChannel(req.ChannelId, req.Text, req.RootBlock, true, req.Author, req.AuthorBackend)
			case *pb.ChatRequest_PerformPrivateAction:
				// TODO: this might not work
				_, err = b.discord.ChannelMessageSendComplex(v.PerformPrivateAction.UserId, &discordgo.MessageSend{
					Content: "_" + renderMessage(v.PerformPrivateAction.Text, v.PerformPrivateAction.RootBlock, nil) + "_",
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

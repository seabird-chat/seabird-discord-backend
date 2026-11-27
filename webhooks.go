package seabird_discord

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/bwmarrin/discordgo"

	seabird "github.com/seabird-chat/seabird-go"
	"github.com/seabird-chat/seabird-go/pb"
)

// webhookName is the name given to webhooks the backend creates. Existing
// ones are found by who created them, so renaming them is harmless.
const webhookName = "seabird"

// webhookRetryDelay is how long a channel whose webhook couldn't be set up
// posts as the bot before setting one up is tried again.
const webhookRetryDelay = 10 * time.Minute

// authorMentions lets messages sent on someone's behalf ping users, but not
// roles or everyone.
var authorMentions = &discordgo.MessageAllowedMentions{
	Parse: []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeUsers},
}

// sendToChannel posts a message or action to a channel. One sent on behalf of
// an author is posted under their name through the channel's webhook, or as
// the bot with their name in the text if that isn't possible.
func (b *Backend) sendToChannel(channelID, text string, block *pb.Block, action bool, author *pb.User, authorBackend *pb.Backend) error {
	replace := b.channelReplacer(channelID)
	render := func(text string, block *pb.Block, action bool) string {
		content := renderMessage(text, block, replace)
		if action {
			content = "_" + content + "_"
		}
		return content
	}

	if author == nil {
		_, err := b.discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Content: render(text, block, action),
			Flags:   discordgo.MessageFlagsSuppressEmbeds,
		})
		return err
	}

	label := authorLabel(author, authorBackend)
	if hook := b.channelWebhook(channelID); hook != nil {
		_, err := b.discord.WebhookExecute(hook.ID, hook.Token, true, &discordgo.WebhookParams{
			Content:         render(text, block, action),
			Username:        label,
			AllowedMentions: authorMentions,
			Flags:           discordgo.MessageFlagsSuppressEmbeds,
		})
		if err == nil {
			return nil
		}
		b.logger.Warn().Err(err).Str("channel_id", channelID).Msg("failed to post through webhook, posting as the bot")
		if isUnknownWebhook(err) {
			b.forgetWebhook(channelID)
		}
	}

	_, err := b.discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         render("", withAuthor(label, text, block, action), false),
		AllowedMentions: authorMentions,
		Flags:           discordgo.MessageFlagsSuppressEmbeds,
	})
	return err
}

// authorLabel is how an author is named, as "alice (IRC)".
func authorLabel(author *pb.User, authorBackend *pb.Backend) string {
	backendName := cmp.Or(authorBackend.GetDisplayName(), authorBackend.GetType())
	if backendName == "" {
		return author.GetDisplayName()
	}
	return fmt.Sprintf("%s (%s)", author.GetDisplayName(), backendName)
}

// withAuthor puts an author's label ahead of a message, as "**alice (IRC)**:
// hi", or "* **alice (IRC)** waves" for actions. It matches how core names
// authors for backends that can't post under their name.
func withAuthor(label, text string, block *pb.Block, action bool) *pb.Block {
	if block == nil {
		block = seabird.NewTextBlock(text)
	}
	name := seabird.NewBoldBlock(seabird.NewTextBlock(label))
	if action {
		return seabird.NewContainerBlock(seabird.NewTextBlock("* "), name, seabird.NewTextBlock(" "), block)
	}
	return seabird.NewContainerBlock(name, seabird.NewTextBlock(": "), block)
}

// channelWebhook returns the webhook to post through in a channel, setting one
// up if needed. It returns nil if the channel has none and one can't be set up.
func (b *Backend) channelWebhook(channelID string) *discordgo.Webhook {
	b.webhookLock.Lock()
	hook, ok := b.webhooks[channelID]
	failedAt, failed := b.webhookFailures[channelID]
	b.webhookLock.Unlock()

	if ok {
		return hook
	}
	if failed && time.Since(failedAt) < webhookRetryDelay {
		return nil
	}

	state := b.discord.State
	if state.User == nil {
		return nil
	}
	permissions, err := state.UserChannelPermissions(state.User.ID, channelID)
	if err != nil || permissions&discordgo.PermissionManageWebhooks == 0 {
		return nil
	}

	hook, err = b.setUpWebhook(channelID, state.User.ID)

	b.webhookLock.Lock()
	defer b.webhookLock.Unlock()

	if err != nil {
		b.logger.Warn().Err(err).Str("channel_id", channelID).Msg("failed to set up webhook, posting as the bot")
		b.webhookFailures[channelID] = time.Now()
		return nil
	}
	delete(b.webhookFailures, channelID)
	b.webhooks[channelID] = hook
	b.ownWebhooks[hook.ID] = true
	return hook
}

// setUpWebhook returns the bot's webhook in a channel, creating it if there
// isn't one.
func (b *Backend) setUpWebhook(channelID, botID string) (*discordgo.Webhook, error) {
	hooks, err := b.discord.ChannelWebhooks(channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}
	for _, hook := range hooks {
		if hook.Type == discordgo.WebhookTypeIncoming && hook.Token != "" && hook.User != nil && hook.User.ID == botID {
			return hook, nil
		}
	}

	hook, err := b.discord.WebhookCreate(channelID, webhookName, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook: %w", err)
	}
	return hook, nil
}

// forgetWebhook drops a channel's webhook, so the next post sets one up again.
// Its ID stays known, so its earlier posts are still recognized.
func (b *Backend) forgetWebhook(channelID string) {
	b.webhookLock.Lock()
	defer b.webhookLock.Unlock()

	delete(b.webhooks, channelID)
}

// isOwnWebhook reports whether a webhook is one the backend posts through.
func (b *Backend) isOwnWebhook(webhookID string) bool {
	b.webhookLock.Lock()
	defer b.webhookLock.Unlock()

	return b.ownWebhooks[webhookID]
}

// isUnknownWebhook reports whether err means the webhook no longer exists.
func isUnknownWebhook(err error) bool {
	var restErr *discordgo.RESTError
	if !errors.As(err, &restErr) {
		return false
	}
	if restErr.Message != nil && restErr.Message.Code == discordgo.ErrCodeUnknownWebhook {
		return true
	}
	return restErr.Response != nil && restErr.Response.StatusCode == http.StatusNotFound
}

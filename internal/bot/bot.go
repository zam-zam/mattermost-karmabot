// Package bot implements the karma bot: it routes Mattermost WebSocket
// events to channel and direct-message commands.
package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"karmabot/internal/storage"
)

const (
	// Daily karma budgets, per channel, per UTC calendar day.
	dailyTotalLimit     = 5
	dailyPerTargetLimit = 2
	topLimit            = 10
)

// Client is the Mattermost surface the bot needs; implemented by
// mmclient.Client and by fakes in tests.
type Client interface {
	Me() *model.User
	GetChannel(ctx context.Context, channelID string) (*model.Channel, error)
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	CreatePost(ctx context.Context, channelID, message string) error
}

// Bot turns "posted" events into karma commands.
type Bot struct {
	client      Client
	store       *storage.Store
	log         *slog.Logger
	msg         *Messages
	botUsername string
	botID       string
	now         func() time.Time
}

// New creates the bot, taking its identity from the client so it can
// filter out its own posts and recognize its mentions. Replies are
// rendered by msg in the configured language.
func New(
	client Client,
	store *storage.Store,
	log *slog.Logger,
	msg *Messages,
) (*Bot, error) {
	me := client.Me()
	if me == nil || me.Id == "" {
		return nil, fmt.Errorf("client returned no bot identity")
	}
	return &Bot{
		client:      client,
		store:       store,
		log:         log,
		msg:         msg,
		botUsername: me.Username,
		botID:       me.Id,
		now:         time.Now,
	}, nil
}

// HandleEvent processes one WebSocket event. A panic in a handler is
// logged instead of crashing the bot.
func (b *Bot) HandleEvent(ctx context.Context, event *model.WebSocketEvent) {
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("panic while handling event", "panic", r)
		}
	}()

	if event == nil || event.EventType() != model.WebsocketEventPosted {
		return
	}

	post := postFromEvent(event)
	if post == nil || post.Message == "" || post.UserId == b.botID {
		return
	}

	channel, err := b.client.GetChannel(ctx, post.ChannelId)
	if err != nil {
		b.log.Error("fetching channel", "err", err, "channel_id", post.ChannelId)
		return
	}

	if channel.Type == model.ChannelTypeDirect {
		b.handleDirect(ctx, post)
		return
	}
	b.handleChannel(ctx, channel, post)
}

// postFromEvent decodes the post attached to a "posted" WebSocket event.
func postFromEvent(event *model.WebSocketEvent) *model.Post {
	raw, ok := event.GetData()["post"].(string)
	if !ok {
		return nil
	}
	var post model.Post
	if err := json.Unmarshal([]byte(raw), &post); err != nil {
		return nil
	}
	return &post
}

// reply posts a message to the channel, logging failures.
func (b *Bot) reply(ctx context.Context, channelID, message string) {
	if err := b.client.CreatePost(ctx, channelID, message); err != nil {
		b.log.Error("sending reply", "err", err, "channel_id", channelID)
	}
}

// internal reports an infrastructure failure to the log and the user.
func (b *Bot) internal(ctx context.Context, channelID string, err error, what string) {
	b.log.Error(what, "err", err, "channel_id", channelID)
	b.reply(ctx, channelID, b.msg.internalError())
}

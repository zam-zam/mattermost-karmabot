// Package mmclient wraps the Mattermost REST and WebSocket APIs behind a
// small surface the bot and scheduler consume, including automatic
// WebSocket reconnection.
package mmclient

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

const (
	reconnectBaseDelay = 5 * time.Second
	reconnectMaxDelay  = time.Minute
)

// Client talks to one Mattermost server as a bot account authenticated by
// a personal access token.
type Client struct {
	api   *model.Client4
	token string
	wsURL string
	log   *slog.Logger

	me *model.User

	mu       sync.Mutex
	channels map[string]*model.Channel
}

// New creates the client and verifies the token by fetching the bot's own
// user. The Mattermost URL must start with http:// or https://.
func New(siteURL, token string, log *slog.Logger) (*Client, error) {
	base := strings.TrimRight(siteURL, "/")
	api := model.NewAPIv4Client(base)
	api.SetOAuthToken(token)
	// Client4 ships a plain http.Client; swap in one whose transport retries
	// transient failures, covering every REST call the wrapper makes.
	api.HTTPClient = &http.Client{Transport: newRetryTransport(nil, log)}

	me, _, err := api.GetMe(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("verifying bot token with GetMe: %w", err)
	}

	wsURL, err := websocketURL(base)
	if err != nil {
		return nil, err
	}

	log.Info("mattermost api ready", "url", base, "bot", me.Username)
	return &Client{
		api:      api,
		token:    token,
		wsURL:    wsURL,
		log:      log,
		me:       me,
		channels: map[string]*model.Channel{},
	}, nil
}

func websocketURL(base string) (string, error) {
	switch {
	case strings.HasPrefix(base, "https://"):
		return "wss://" + strings.TrimPrefix(base, "https://"), nil
	case strings.HasPrefix(base, "http://"):
		return "ws://" + strings.TrimPrefix(base, "http://"), nil
	default:
		return "", fmt.Errorf("mattermost url %q must start with http:// or https://", base)
	}
}

// Me returns the bot's own user, fetched at startup.
func (c *Client) Me() *model.User {
	return c.me
}

// GetChannel fetches the channel, caching results for later lookups.
func (c *Client) GetChannel(ctx context.Context, channelID string) (*model.Channel, error) {
	c.mu.Lock()
	ch, ok := c.channels[channelID]
	c.mu.Unlock()
	if ok {
		return ch, nil
	}

	ch, _, err := c.api.GetChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("fetching channel %s: %w", channelID, err)
	}

	c.mu.Lock()
	c.channels[channelID] = ch
	c.mu.Unlock()
	return ch, nil
}

// GetUserByUsername resolves a username to a user.
func (c *Client) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	user, _, err := c.api.GetUserByUsername(ctx, username, "")
	if err != nil {
		return nil, fmt.Errorf("fetching user %s: %w", username, err)
	}
	return user, nil
}

// CreatePost posts a plain message, threading it under rootID when it is
// non-empty.
func (c *Client) CreatePost(ctx context.Context, channelID, rootID, message string) error {
	post := &model.Post{
		ChannelId: channelID,
		RootId:    rootID,
		Message:   message,
	}
	if _, _, err := c.api.CreatePost(ctx, post); err != nil {
		return fmt.Errorf("posting to channel %s: %w", channelID, err)
	}
	return nil
}

// Events streams server events over a WebSocket connection, reconnecting
// with capped exponential backoff until ctx is cancelled.
func (c *Client) Events(ctx context.Context) <-chan *model.WebSocketEvent {
	events := make(chan *model.WebSocketEvent)
	go c.pumpEvents(ctx, events)
	return events
}

func (c *Client) pumpEvents(ctx context.Context, out chan<- *model.WebSocketEvent) {
	defer close(out)

	delay := reconnectBaseDelay
	for ctx.Err() == nil {
		ws, err := model.NewWebSocketClient4(c.wsURL, c.token)
		if err != nil {
			c.log.Warn("websocket connect failed", "err", err, "retry_in", delay)
			if !sleep(ctx, delay) {
				return
			}
			delay = min(2*delay, reconnectMaxDelay)
			continue
		}

		c.log.Info("websocket connected")
		ws.Listen()

		// The ping watchdog writes to PingTimeoutChannel on stale
		// connections; keep draining it until this connection is done.
		drained := make(chan struct{})
		go drainPingTimeouts(ws, drained)

		for event := range ws.EventChannel {
			select {
			case out <- event:
			case <-ctx.Done():
				close(drained)
				ws.Close()
				return
			}
		}
		// EventChannel closed: the connection broke, reconnect below.
		close(drained)
		ws.Close()

		c.log.Warn("websocket disconnected, reconnecting", "retry_in", delay)
		if !sleep(ctx, delay) {
			return
		}
		delay = min(2*delay, reconnectMaxDelay)
	}
}

func drainPingTimeouts(ws *model.WebSocketClient, done <-chan struct{}) {
	for {
		select {
		case <-ws.PingTimeoutChannel:
		case <-done:
			return
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

package slack

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Notifier provides a thread-aware, concurrency-safe API for sending Slack
// notifications. It wraps Client and manages session threading automatically.
//
// All methods are no-ops when the config is not configured, so callers never
// need nil checks.
type Notifier struct {
	client    *Client
	cfg       Config
	mu        sync.Mutex
	threadTS  string
	started   bool
}

// NewNotifier creates a new Notifier. Always returns non-nil. All methods
// return nil immediately if the config is not configured.
func NewNotifier(cfg Config, opts ...ClientOption) *Notifier {
	var client *Client
	if cfg.IsConfigured() {
		client = NewClient(cfg, opts...)
	}
	return &Notifier{
		client: client,
		cfg:    cfg,
	}
}

// IsEnabled returns whether the notifier is ready to send messages.
func (n *Notifier) IsEnabled() bool {
	return n.cfg.IsConfigured() && n.client != nil
}

// StartSession posts the initial top-level message for a session. Subsequent
// calls to Send and EndSession thread under this message. If only a webhook
// is configured (no bot token), the message is sent via webhook without
// threading support.
func (n *Notifier) StartSession(ctx context.Context, text string, blocks ...Block) error {
	if !n.IsEnabled() {
		return nil
	}

	n.mu.Lock()
	n.started = true
	n.mu.Unlock()

	// Bot token mode: use chat.postMessage to capture thread TS.
	if n.cfg.BotToken != "" && n.cfg.Channel != "" {
		req := &PostMessageRequest{
			Channel: n.cfg.Channel,
			Text:    text,
			Blocks:  blocks,
		}

		var ts string
		err := n.client.PostWithRetry(ctx, func() error {
			resp, err := n.client.PostMessage(ctx, req)
			if err != nil {
				return err
			}
			ts = resp.TS
			return nil
		})
		if err != nil {
			return fmt.Errorf("start session: %w", err)
		}

		n.mu.Lock()
		n.threadTS = ts
		n.mu.Unlock()
		return nil
	}

	// Webhook fallback: no threading.
	return n.client.PostWithRetry(ctx, func() error {
		return n.client.PostWebhook(ctx, text, blocks...)
	})
}

// Send posts a message. If a session has been started with a bot token, the
// message threads under it. Otherwise falls back to webhook.
func (n *Notifier) Send(ctx context.Context, text string, blocks ...Block) error {
	if !n.IsEnabled() {
		return nil
	}

	n.mu.Lock()
	ts := n.threadTS
	n.mu.Unlock()

	// Thread reply via bot API if we have a thread TS.
	if ts != "" && n.cfg.BotToken != "" {
		req := &PostMessageRequest{
			Channel:  n.cfg.Channel,
			Text:     text,
			Blocks:   blocks,
			ThreadTS: ts,
		}
		return n.client.PostWithRetry(ctx, func() error {
			_, err := n.client.PostMessage(ctx, req)
			return err
		})
	}

	// Webhook fallback.
	if n.client.webhookURL != "" {
		return n.client.PostWithRetry(ctx, func() error {
			return n.client.PostWebhook(ctx, text, blocks...)
		})
	}

	// Bot API without thread (no session started).
	if n.cfg.BotToken != "" {
		req := &PostMessageRequest{
			Channel: n.cfg.Channel,
			Text:    text,
			Blocks:  blocks,
		}
		return n.client.PostWithRetry(ctx, func() error {
			_, err := n.client.PostMessage(ctx, req)
			return err
		})
	}

	return nil
}

// SendWithMentions posts a message with @mentions for the given user IDs.
func (n *Notifier) SendWithMentions(ctx context.Context, text string, userIDs []string, blocks ...Block) error {
	if len(userIDs) > 0 {
		mentions := formatMentions(userIDs)
		text = text + "\n" + mentions
	}
	return n.Send(ctx, text, blocks...)
}

// EndSession posts a final message for the session, including @mentions for
// users configured in Config.NotifyUsers.
func (n *Notifier) EndSession(ctx context.Context, text string, blocks ...Block) error {
	return n.SendWithMentions(ctx, text, n.cfg.NotifyUsers, blocks...)
}

// ThreadTS returns the current thread timestamp, or empty string if no
// session has been started or only webhook mode is in use.
func (n *Notifier) ThreadTS() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.threadTS
}

// formatMentions builds a Slack mention string from user IDs.
func formatMentions(userIDs []string) string {
	mentions := make([]string, len(userIDs))
	for i, id := range userIDs {
		mentions[i] = fmt.Sprintf("<@%s>", id)
	}
	return strings.Join(mentions, " ")
}

package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Block represents a Slack Block Kit block.
type Block struct {
	Type   string      `json:"type"`
	Text   *BlockText  `json:"text,omitempty"`
	Fields []BlockText `json:"fields,omitempty"`
}

// BlockText represents text within a block.
type BlockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Attachment represents a Slack message attachment.
type Attachment struct {
	Color string `json:"color,omitempty"`
	Text  string `json:"text,omitempty"`
}

// PostMessageRequest represents a chat.postMessage API request.
type PostMessageRequest struct {
	Channel  string  `json:"channel"`
	Text     string  `json:"text,omitempty"`
	Blocks   []Block `json:"blocks,omitempty"`
	ThreadTS string  `json:"thread_ts,omitempty"`
}

// PostMessageResponse represents a chat.postMessage API response.
type PostMessageResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	TS      string `json:"ts,omitempty"`
	Channel string `json:"channel,omitempty"`
}

// RetryConfig controls retry behavior for PostWithRetry.
type RetryConfig struct {
	Backoff []time.Duration
}

// DefaultRetryConfig returns the default retry configuration: 3 attempts with
// exponential backoff [1s, 2s, 4s].
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		Backoff: []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second},
	}
}

// ClientOption configures a Client.
type ClientOption func(*clientConfig)

type clientConfig struct {
	apiBaseURL  string
	retryConfig RetryConfig
	httpClient  *http.Client
}

// WithAPIBaseURL overrides the Slack API base URL. Useful for testing with httptest.
func WithAPIBaseURL(url string) ClientOption {
	return func(cfg *clientConfig) { cfg.apiBaseURL = url }
}

// WithRetryConfig sets custom retry behavior.
func WithRetryConfig(rc RetryConfig) ClientOption {
	return func(cfg *clientConfig) { cfg.retryConfig = rc }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(cfg *clientConfig) { cfg.httpClient = c }
}

// Client handles raw Slack API interactions via webhook and chat.postMessage.
type Client struct {
	webhookURL  string
	botToken    string
	channel     string
	httpClient  *http.Client
	apiBaseURL  string
	retryConfig RetryConfig
}

// NewClient creates a new Slack API client from the given Config.
func NewClient(cfg Config, opts ...ClientOption) *Client {
	cc := clientConfig{
		retryConfig: DefaultRetryConfig(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(&cc)
	}

	return &Client{
		webhookURL:  cfg.WebhookURL,
		botToken:    cfg.BotToken,
		channel:     cfg.Channel,
		httpClient:  cc.httpClient,
		apiBaseURL:  cc.apiBaseURL,
		retryConfig: cc.retryConfig,
	}
}

// webhookMessage is the JSON body sent to a Slack webhook URL.
type webhookMessage struct {
	Text        string       `json:"text,omitempty"`
	Channel     string       `json:"channel,omitempty"`
	Blocks      []Block      `json:"blocks,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// PostWebhook sends a message via an incoming webhook URL.
func (c *Client) PostWebhook(ctx context.Context, text string, blocks ...Block) error {
	if c.webhookURL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	msg := webhookMessage{
		Text:   text,
		Blocks: blocks,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// PostMessage sends a message via the chat.postMessage API. Requires a bot token.
func (c *Client) PostMessage(ctx context.Context, req *PostMessageRequest) (*PostMessageResponse, error) {
	if c.botToken == "" {
		return nil, fmt.Errorf("bot token not configured")
	}

	if req.Channel == "" {
		req.Channel = c.channel
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	apiURL := "https://slack.com/api/chat.postMessage"
	if c.apiBaseURL != "" {
		apiURL = c.apiBaseURL + "/chat.postMessage"
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.botToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	var result PostMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("slack api error: %s", result.Error)
	}

	return &result, nil
}

// PostWithRetry executes fn with exponential backoff retry on failure.
func (c *Client) PostWithRetry(ctx context.Context, fn func() error) error {
	backoff := c.retryConfig.Backoff

	var lastErr error
	for i := 0; i <= len(backoff); i++ {
		if err := fn(); err != nil {
			lastErr = err
			if i < len(backoff) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff[i]):
					continue
				}
			}
		} else {
			return nil
		}
	}

	return fmt.Errorf("after %d retries: %w", len(backoff), lastErr)
}

// IsConfigured returns true if the client has minimum configuration to send messages.
func (c *Client) IsConfigured() bool {
	return c.webhookURL != "" || (c.botToken != "" && c.channel != "")
}

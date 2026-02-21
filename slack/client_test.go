package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostWebhook(t *testing.T) {
	var received webhookMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(Config{
		Enabled:    true,
		WebhookURL: srv.URL,
	})

	err := client.PostWebhook(context.Background(), "hello", Block{
		Type: "section",
		Text: &BlockText{Type: "mrkdwn", Text: "world"},
	})
	if err != nil {
		t.Fatalf("PostWebhook: %v", err)
	}
	if received.Text != "hello" {
		t.Errorf("expected text 'hello', got %q", received.Text)
	}
	if len(received.Blocks) != 1 || received.Blocks[0].Type != "section" {
		t.Errorf("unexpected blocks: %+v", received.Blocks)
	}
}

func TestPostWebhookError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	client := NewClient(Config{Enabled: true, WebhookURL: srv.URL})
	err := client.PostWebhook(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestPostWebhookNoURL(t *testing.T) {
	client := NewClient(Config{Enabled: true})
	err := client.PostWebhook(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error when webhook URL not configured")
	}
}

func TestPostMessage(t *testing.T) {
	var received PostMessageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer xoxb-test" {
			t.Errorf("expected Bearer xoxb-test, got %s", auth)
		}
		json.NewDecoder(r.Body).Decode(&received)
		json.NewEncoder(w).Encode(PostMessageResponse{
			OK:      true,
			TS:      "1234567890.123456",
			Channel: "C123",
		})
	}))
	defer srv.Close()

	client := NewClient(Config{
		Enabled:  true,
		BotToken: "xoxb-test",
		Channel:  "C123",
	}, WithAPIBaseURL(srv.URL))

	resp, err := client.PostMessage(context.Background(), &PostMessageRequest{
		Text:     "hello",
		ThreadTS: "1234567890.000000",
	})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if resp.TS != "1234567890.123456" {
		t.Errorf("expected TS 1234567890.123456, got %s", resp.TS)
	}
	if received.Channel != "C123" {
		t.Errorf("expected channel C123, got %s", received.Channel)
	}
	if received.ThreadTS != "1234567890.000000" {
		t.Errorf("expected thread_ts, got %s", received.ThreadTS)
	}
}

func TestPostMessageNoBotToken(t *testing.T) {
	client := NewClient(Config{Enabled: true, Channel: "C123"})
	_, err := client.PostMessage(context.Background(), &PostMessageRequest{Text: "test"})
	if err == nil {
		t.Fatal("expected error when bot token not configured")
	}
}

func TestPostMessageAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(PostMessageResponse{OK: false, Error: "channel_not_found"})
	}))
	defer srv.Close()

	client := NewClient(Config{
		Enabled:  true,
		BotToken: "xoxb-test",
		Channel:  "C123",
	}, WithAPIBaseURL(srv.URL))

	_, err := client.PostMessage(context.Background(), &PostMessageRequest{Text: "test"})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestPostWithRetry(t *testing.T) {
	var attempts int32
	client := NewClient(Config{Enabled: true}, WithRetryConfig(RetryConfig{
		Backoff: []time.Duration{10 * time.Millisecond, 20 * time.Millisecond},
	}))

	err := client.PostWithRetry(context.Background(), func() error {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			return fmt.Errorf("attempt %d failed", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("PostWithRetry: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestPostWithRetryExhausted(t *testing.T) {
	client := NewClient(Config{Enabled: true}, WithRetryConfig(RetryConfig{
		Backoff: []time.Duration{10 * time.Millisecond},
	}))

	err := client.PostWithRetry(context.Background(), func() error {
		return fmt.Errorf("always fails")
	})
	if err == nil {
		t.Fatal("expected error when retries exhausted")
	}
}

func TestPostWithRetryContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := NewClient(Config{Enabled: true}, WithRetryConfig(RetryConfig{
		Backoff: []time.Duration{10 * time.Second},
	}))

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := client.PostWithRetry(ctx, func() error {
		return fmt.Errorf("fail")
	})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestConfigIsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"webhook only", Config{Enabled: true, WebhookURL: "https://hooks.slack.com/x"}, true},
		{"bot+channel", Config{Enabled: true, BotToken: "xoxb-x", Channel: "C1"}, true},
		{"disabled", Config{Enabled: false, WebhookURL: "https://hooks.slack.com/x"}, false},
		{"empty", Config{Enabled: true}, false},
		{"bot without channel", Config{Enabled: true, BotToken: "xoxb-x"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsConfigured(); got != tt.want {
				t.Errorf("Config.IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClientIsConfigured(t *testing.T) {
	// Client.IsConfigured checks transport readiness only (not Enabled flag).
	client := NewClient(Config{Enabled: true, WebhookURL: "https://hooks.slack.com/x"})
	if !client.IsConfigured() {
		t.Error("client with webhook should be configured")
	}
	empty := NewClient(Config{Enabled: true})
	if empty.IsConfigured() {
		t.Error("client with no URLs should not be configured")
	}
}

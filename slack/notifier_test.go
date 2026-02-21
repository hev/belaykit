package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func botConfig(srv *httptest.Server) (Config, []ClientOption) {
	return Config{
		Enabled:     true,
		BotToken:    "xoxb-test",
		Channel:     "C123",
		NotifyUsers: []string{"U111", "U222"},
	}, []ClientOption{
		WithAPIBaseURL(srv.URL),
		WithRetryConfig(RetryConfig{Backoff: []time.Duration{10 * time.Millisecond}}),
	}
}

func TestNotifierDisabled(t *testing.T) {
	n := NewNotifier(Config{Enabled: false})
	if n == nil {
		t.Fatal("NewNotifier should never return nil")
	}
	if n.IsEnabled() {
		t.Fatal("should not be enabled")
	}
	// All methods should no-op without error.
	ctx := context.Background()
	if err := n.StartSession(ctx, "test"); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if err := n.Send(ctx, "test"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := n.EndSession(ctx, "test"); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
}

func TestNotifierThreading(t *testing.T) {
	var mu sync.Mutex
	var requests []PostMessageRequest

	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req PostMessageRequest
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()

		// Return thread TS on first call (StartSession).
		json.NewEncoder(w).Encode(PostMessageResponse{
			OK: true,
			TS: "1700000000.000001",
		})
	}))

	cfg, opts := botConfig(srv)
	n := NewNotifier(cfg, opts...)

	ctx := context.Background()

	// StartSession should capture thread TS.
	if err := n.StartSession(ctx, "Session started"); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if ts := n.ThreadTS(); ts != "1700000000.000001" {
		t.Fatalf("expected thread TS 1700000000.000001, got %q", ts)
	}

	// Send should thread under the session.
	if err := n.Send(ctx, "Progress update"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// EndSession should thread and include mentions.
	if err := n.EndSession(ctx, "Done"); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(requests))
	}

	// First request: no thread_ts (top-level).
	if requests[0].ThreadTS != "" {
		t.Errorf("StartSession should not have thread_ts, got %q", requests[0].ThreadTS)
	}

	// Second request: threaded.
	if requests[1].ThreadTS != "1700000000.000001" {
		t.Errorf("Send should have thread_ts, got %q", requests[1].ThreadTS)
	}

	// Third request: threaded with mentions.
	if requests[2].ThreadTS != "1700000000.000001" {
		t.Errorf("EndSession should have thread_ts, got %q", requests[2].ThreadTS)
	}
	if !strings.Contains(requests[2].Text, "<@U111>") || !strings.Contains(requests[2].Text, "<@U222>") {
		t.Errorf("EndSession should mention users, got %q", requests[2].Text)
	}
}

func TestNotifierWebhookFallback(t *testing.T) {
	var called int32
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	}))

	cfg := Config{
		Enabled:    true,
		WebhookURL: srv.URL,
	}
	n := NewNotifier(cfg, WithRetryConfig(RetryConfig{Backoff: []time.Duration{10 * time.Millisecond}}))

	ctx := context.Background()
	if err := n.StartSession(ctx, "hello"); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if err := n.Send(ctx, "update"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if n := atomic.LoadInt32(&called); n != 2 {
		t.Errorf("expected 2 webhook calls, got %d", n)
	}
	if ts := n.ThreadTS(); ts != "" {
		t.Errorf("webhook mode should not have thread TS, got %q", ts)
	}
}

func TestNotifierConcurrency(t *testing.T) {
	var calls int32
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(PostMessageResponse{OK: true, TS: "1700000000.000001"})
	}))

	cfg, opts := botConfig(srv)
	n := NewNotifier(cfg, opts...)

	ctx := context.Background()
	if err := n.StartSession(ctx, "start"); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Fire 10 concurrent Sends.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.Send(ctx, "concurrent update")
		}()
	}
	wg.Wait()

	total := atomic.LoadInt32(&calls)
	// 1 StartSession + 10 Sends = 11
	if total != 11 {
		t.Errorf("expected 11 API calls, got %d", total)
	}
}

func TestFormatMentions(t *testing.T) {
	got := formatMentions([]string{"U111", "U222"})
	if got != "<@U111> <@U222>" {
		t.Errorf("formatMentions = %q", got)
	}
}

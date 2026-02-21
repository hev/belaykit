package slack

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go-rack"
)

func TestNewEventHandler(t *testing.T) {
	var mu sync.Mutex
	var requests []PostMessageRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req PostMessageRequest
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		json.NewEncoder(w).Encode(PostMessageResponse{OK: true, TS: "1700000000.000001"})
	}))
	defer srv.Close()

	cfg := Config{
		Enabled:  true,
		BotToken: "xoxb-test",
		Channel:  "C123",
		Events: EventConfig{
			OnStart:  true,
			OnError:  true,
			OnResult: true,
		},
	}
	opts := []ClientOption{
		WithAPIBaseURL(srv.URL),
		WithRetryConfig(RetryConfig{Backoff: []time.Duration{10 * time.Millisecond}}),
	}
	notifier := NewNotifier(cfg, opts...)
	handler := NewEventHandler(notifier, WithHandlerAgentName("test-agent"))

	// Init event should trigger StartSession.
	handler(rack.Event{
		Type:      rack.EventSystem,
		Subtype:   "init",
		SessionID: "sess-123",
	})
	// Give goroutine time to complete.
	time.Sleep(100 * time.Millisecond)

	// Error event should trigger Send.
	handler(rack.Event{
		Type: rack.EventResultError,
		Text: "something broke",
	})
	time.Sleep(100 * time.Millisecond)

	// Result event should trigger EndSession.
	handler(rack.Event{
		Type:     rack.EventResult,
		NumTurns: 5,
		Duration: 3000,
		CostUSD:  0.05,
	})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(requests) < 3 {
		t.Fatalf("expected at least 3 requests, got %d", len(requests))
	}

	// StartSession: no thread_ts.
	if requests[0].ThreadTS != "" {
		t.Errorf("StartSession should not have thread_ts")
	}
	if requests[0].Text == "" {
		t.Error("StartSession text should not be empty")
	}

	// Error: threaded.
	if requests[1].ThreadTS != "1700000000.000001" {
		t.Errorf("error should be threaded, got thread_ts=%q", requests[1].ThreadTS)
	}

	// Result: threaded.
	if requests[2].ThreadTS != "1700000000.000001" {
		t.Errorf("result should be threaded, got thread_ts=%q", requests[2].ThreadTS)
	}
}

func TestNewEventHandlerOnlyConfiguredEvents(t *testing.T) {
	var mu sync.Mutex
	var calls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		json.NewEncoder(w).Encode(PostMessageResponse{OK: true, TS: "1700000000.000001"})
	}))
	defer srv.Close()

	cfg := Config{
		Enabled:  true,
		BotToken: "xoxb-test",
		Channel:  "C123",
		Events: EventConfig{
			OnStart:   false,
			OnError:   false,
			OnResult:  false,
			OnToolUse: false,
		},
	}
	notifier := NewNotifier(cfg,
		WithAPIBaseURL(srv.URL),
		WithRetryConfig(RetryConfig{Backoff: []time.Duration{10 * time.Millisecond}}),
	)
	handler := NewEventHandler(notifier)

	handler(rack.Event{Type: rack.EventSystem, Subtype: "init"})
	handler(rack.Event{Type: rack.EventResultError, Text: "err"})
	handler(rack.Event{Type: rack.EventResult, NumTurns: 1})
	handler(rack.Event{Type: rack.EventToolUse, ToolName: "bash"})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Errorf("expected 0 calls with all events disabled, got %d", calls)
	}
}

func TestNewEventHandlerToolUse(t *testing.T) {
	var mu sync.Mutex
	var requests []PostMessageRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req PostMessageRequest
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		json.NewEncoder(w).Encode(PostMessageResponse{OK: true, TS: "1700000000.000001"})
	}))
	defer srv.Close()

	cfg := Config{
		Enabled:  true,
		BotToken: "xoxb-test",
		Channel:  "C123",
		Events: EventConfig{
			OnStart:   true,
			OnToolUse: true,
		},
	}
	notifier := NewNotifier(cfg,
		WithAPIBaseURL(srv.URL),
		WithRetryConfig(RetryConfig{Backoff: []time.Duration{10 * time.Millisecond}}),
	)
	handler := NewEventHandler(notifier, WithHandlerAgentName("myagent"))

	handler(rack.Event{Type: rack.EventSystem, Subtype: "init"})
	time.Sleep(100 * time.Millisecond)

	handler(rack.Event{Type: rack.EventToolUse, ToolName: "Read"})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(requests) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requests))
	}

	// Tool use should be threaded and contain tool name.
	if requests[1].ThreadTS != "1700000000.000001" {
		t.Errorf("tool use should be threaded")
	}
	if requests[1].Text != "[myagent] Tool: Read" {
		t.Errorf("unexpected tool use text: %q", requests[1].Text)
	}
}

func TestNewEventHandlerCustomFormatters(t *testing.T) {
	var mu sync.Mutex
	var requests []PostMessageRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req PostMessageRequest
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		json.NewEncoder(w).Encode(PostMessageResponse{OK: true, TS: "1700000000.000001"})
	}))
	defer srv.Close()

	cfg := Config{
		Enabled:  true,
		BotToken: "xoxb-test",
		Channel:  "C123",
		Events:   EventConfig{OnError: true, OnStart: true},
	}
	notifier := NewNotifier(cfg,
		WithAPIBaseURL(srv.URL),
		WithRetryConfig(RetryConfig{Backoff: []time.Duration{10 * time.Millisecond}}),
	)

	handler := NewEventHandler(notifier,
		WithErrorFormatter(func(e rack.Event) (string, []Block) {
			return "CUSTOM ERROR: " + e.Text, nil
		}),
	)

	handler(rack.Event{Type: rack.EventSystem, Subtype: "init"})
	time.Sleep(100 * time.Millisecond)

	handler(rack.Event{Type: rack.EventResultError, Text: "boom"})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(requests) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requests))
	}
	if requests[1].Text != "CUSTOM ERROR: boom" {
		t.Errorf("expected custom error format, got %q", requests[1].Text)
	}
}

func TestNewEventHandlerDisabled(t *testing.T) {
	notifier := NewNotifier(Config{Enabled: false})
	handler := NewEventHandler(notifier)

	// Should not panic.
	handler(rack.Event{Type: rack.EventSystem, Subtype: "init"})
	handler(rack.Event{Type: rack.EventResultError, Text: "err"})
	handler(rack.Event{Type: rack.EventResult, NumTurns: 1})
}

func TestEventHandlerComposable(t *testing.T) {
	notifier := NewNotifier(Config{Enabled: false})
	slackH := NewEventHandler(notifier)

	var logCalled bool
	logH := func(e rack.Event) { logCalled = true }

	combined := func(e rack.Event) { logH(e); slackH(e) }
	combined(rack.Event{Type: rack.EventResult})

	if !logCalled {
		t.Error("log handler should have been called")
	}
}

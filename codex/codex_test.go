package codex

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	rack "go-rack"
)

func TestNewClientDefaults(t *testing.T) {
	c := NewClient()
	if c.executable != "codex" {
		t.Errorf("executable = %q, want %q", c.executable, "codex")
	}
	if c.defaultModel != "" {
		t.Errorf("defaultModel = %q, want empty", c.defaultModel)
	}
	if c.eventHandler != nil {
		t.Error("eventHandler should be nil by default")
	}
}

func TestNewClientWithOptions(t *testing.T) {
	var handlerCalled bool
	handler := func(rack.Event) { handlerCalled = true }

	c := NewClient(
		WithExecutable("/usr/local/bin/codex"),
		WithDefaultModel("gpt-5-codex"),
		WithDefaultEventHandler(handler),
	)

	if c.executable != "/usr/local/bin/codex" {
		t.Errorf("executable = %q, want %q", c.executable, "/usr/local/bin/codex")
	}
	if c.defaultModel != "gpt-5-codex" {
		t.Errorf("defaultModel = %q, want %q", c.defaultModel, "gpt-5-codex")
	}
	if c.eventHandler == nil {
		t.Error("eventHandler should not be nil")
	}

	c.eventHandler(rack.Event{})
	if !handlerCalled {
		t.Error("event handler was not called")
	}
}

func TestRunErrCLINotFound(t *testing.T) {
	c := NewClient(WithExecutable("nonexistent-codex-cli"))
	_, err := c.Run(t.Context(), "hello")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !errors.Is(err, ErrCLINotFound) {
		t.Fatalf("expected ErrCLINotFound, got %v", err)
	}
}

func TestRunUnsupportedOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []rack.RunOption
		want string
	}{
		{name: "max turns", opts: []rack.RunOption{rack.WithMaxTurns(2)}, want: "WithMaxTurns"},
		{name: "max output", opts: []rack.RunOption{rack.WithMaxOutputTokens(100)}, want: "WithMaxOutputTokens"},
		{name: "allowed tools", opts: []rack.RunOption{rack.WithAllowedTools("Bash(*)")}, want: "WithAllowedTools"},
		{name: "disallowed tools", opts: []rack.RunOption{rack.WithDisallowedTools("Write(*)")}, want: "WithDisallowedTools"},
	}

	c := NewClient(WithExecutable("true"))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.Run(t.Context(), "hello", tt.opts...)
			if err == nil {
				t.Fatal("expected error")
			}
			var uerr *UnsupportedOptionError
			if !errors.As(err, &uerr) {
				t.Fatalf("expected UnsupportedOptionError, got %v", err)
			}
			if uerr.Option != tt.want {
				t.Fatalf("option = %q, want %q", uerr.Option, tt.want)
			}
		})
	}
}

func TestRunSuccessFromFakeExecutable(t *testing.T) {
	exe := writeScript(t, "codex-success.sh", `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
echo '{"type":"thread.started","thread_id":"thread-123"}' 1>&2
echo '{"type":"turn.started"}' 1>&2
echo '{"type":"assistant.message.delta","delta":"hello "}' 1>&2
echo '{"type":"assistant.message.delta","delta":"world"}' 1>&2
echo '{"type":"turn.completed","duration_ms":12,"cost_usd":0.001}' 1>&2
printf 'hello world' > "$out"
`)

	c := NewClient(WithExecutable(exe), WithDefaultModel("gpt-5-codex"))

	var gotEvents []rack.EventType
	var streamOut bytes.Buffer
	var assistantFromEvents stringsBuilder
	handler := func(ev rack.Event) {
		gotEvents = append(gotEvents, ev.Type)
		if ev.Type == rack.EventAssistant {
			assistantFromEvents.WriteString(ev.Text)
		}
	}

	res, err := c.Run(
		t.Context(),
		"say hi",
		rack.WithEventHandler(handler),
		rack.WithOutputStream(&streamOut),
	)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.Text != "hello world" {
		t.Fatalf("result text = %q, want %q", res.Text, "hello world")
	}
	if streamOut.String() != "hello world" {
		t.Fatalf("stream output = %q, want %q", streamOut.String(), "hello world")
	}
	if assistantFromEvents.String() != "hello world" {
		t.Fatalf("assistant event text = %q, want %q", assistantFromEvents.String(), "hello world")
	}

	mustContainEvent(t, gotEvents, rack.EventSystem)
	mustContainEvent(t, gotEvents, rack.EventAssistantStart)
	mustContainEvent(t, gotEvents, rack.EventAssistant)
	mustContainEvent(t, gotEvents, rack.EventResult)
}

func TestRunFailureFromFakeExecutable(t *testing.T) {
	exe := writeScript(t, "codex-fail.sh", `#!/bin/sh
echo '{"type":"thread.started","thread_id":"thread-123"}' 1>&2
echo '{"type":"turn.started"}' 1>&2
echo '{"type":"turn.failed","error":{"message":"boom"}}' 1>&2
exit 1
`)

	c := NewClient(WithExecutable(exe))
	var gotErrorEvent bool
	handler := func(ev rack.Event) {
		if ev.Type == rack.EventResultError && ev.Text == "boom" {
			gotErrorEvent = true
		}
	}

	_, err := c.Run(t.Context(), "hello", rack.WithEventHandler(handler))
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if !gotErrorEvent {
		t.Fatal("expected result error event")
	}
}

type stringsBuilder struct {
	b bytes.Buffer
}

func (s *stringsBuilder) WriteString(v string) {
	s.b.WriteString(v)
}

func (s *stringsBuilder) String() string {
	return s.b.String()
}

func mustContainEvent(t *testing.T, got []rack.EventType, want rack.EventType) {
	t.Helper()
	for _, ev := range got {
		if ev == want {
			return
		}
	}
	t.Fatalf("expected event %q in %+v", want, got)
}

func writeScript(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

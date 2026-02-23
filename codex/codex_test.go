package codex

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"belaykit"
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
	handler := func(belaykit.Event) { handlerCalled = true }

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

	c.eventHandler(belaykit.Event{})
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
		opts []belaykit.RunOption
		want string
	}{
		{name: "max turns", opts: []belaykit.RunOption{belaykit.WithMaxTurns(2)}, want: "WithMaxTurns"},
		{name: "max output", opts: []belaykit.RunOption{belaykit.WithMaxOutputTokens(100)}, want: "WithMaxOutputTokens"},
		{name: "allowed tools", opts: []belaykit.RunOption{belaykit.WithAllowedTools("Bash(*)")}, want: "WithAllowedTools"},
		{name: "disallowed tools", opts: []belaykit.RunOption{belaykit.WithDisallowedTools("Write(*)")}, want: "WithDisallowedTools"},
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

	var gotEvents []belaykit.EventType
	var streamOut bytes.Buffer
	var assistantFromEvents stringsBuilder
	handler := func(ev belaykit.Event) {
		gotEvents = append(gotEvents, ev.Type)
		if ev.Type == belaykit.EventAssistant {
			assistantFromEvents.WriteString(ev.Text)
		}
	}

	res, err := c.Run(
		t.Context(),
		"say hi",
		belaykit.WithEventHandler(handler),
		belaykit.WithOutputStream(&streamOut),
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

	mustContainEvent(t, gotEvents, belaykit.EventSystem)
	mustContainEvent(t, gotEvents, belaykit.EventAssistantStart)
	mustContainEvent(t, gotEvents, belaykit.EventAssistant)
	mustContainEvent(t, gotEvents, belaykit.EventResult)
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
	handler := func(ev belaykit.Event) {
		if ev.Type == belaykit.EventResultError && ev.Text == "boom" {
			gotErrorEvent = true
		}
	}

	_, err := c.Run(t.Context(), "hello", belaykit.WithEventHandler(handler))
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

func mustContainEvent(t *testing.T, got []belaykit.EventType, want belaykit.EventType) {
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

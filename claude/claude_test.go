package claude

import (
	"testing"

	rack "go-rack"
)

func TestNewClientDefaults(t *testing.T) {
	c := NewClient()
	if c.executable != "claude" {
		t.Errorf("executable = %q, want %q", c.executable, "claude")
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
		WithExecutable("/usr/local/bin/claude"),
		WithDefaultModel("opus"),
		WithDefaultEventHandler(handler),
	)

	if c.executable != "/usr/local/bin/claude" {
		t.Errorf("executable = %q, want %q", c.executable, "/usr/local/bin/claude")
	}
	if c.defaultModel != "opus" {
		t.Errorf("defaultModel = %q, want %q", c.defaultModel, "opus")
	}
	if c.eventHandler == nil {
		t.Error("eventHandler should not be nil")
	}

	// Verify the handler is the one we set
	c.eventHandler(rack.Event{})
	if !handlerCalled {
		t.Error("event handler was not called")
	}
}

func TestRunOptions(t *testing.T) {
	var cfg rack.RunConfig

	rack.WithModel("sonnet")(&cfg)
	if cfg.Model != "sonnet" {
		t.Errorf("model = %q, want %q", cfg.Model, "sonnet")
	}

	rack.WithMaxTurns(10)(&cfg)
	if cfg.MaxTurns != 10 {
		t.Errorf("maxTurns = %d, want %d", cfg.MaxTurns, 10)
	}

	rack.WithAllowedTools("Bash(*)", "Write(*)")(&cfg)
	if len(cfg.AllowedTools) != 2 {
		t.Errorf("allowedTools length = %d, want 2", len(cfg.AllowedTools))
	}
	if cfg.AllowedTools[0] != "Bash(*)" {
		t.Errorf("allowedTools[0] = %q, want %q", cfg.AllowedTools[0], "Bash(*)")
	}

	rack.WithSystemPrompt("be helpful")(&cfg)
	if cfg.SystemPrompt != "be helpful" {
		t.Errorf("systemPrompt = %q, want %q", cfg.SystemPrompt, "be helpful")
	}
}

func TestRunErrCLINotFound(t *testing.T) {
	c := NewClient(WithExecutable("nonexistent-binary-that-does-not-exist"))
	_, err := c.Run(t.Context(), "hello")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	// Should get either ErrCLINotFound or an ExitError
	// depending on the OS
}

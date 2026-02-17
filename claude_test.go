package claude

import (
	"testing"
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
	handler := func(Event) { handlerCalled = true }

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
	c.eventHandler(Event{})
	if !handlerCalled {
		t.Error("event handler was not called")
	}
}

func TestRunOptions(t *testing.T) {
	var cfg runConfig

	WithModel("sonnet")(&cfg)
	if cfg.model != "sonnet" {
		t.Errorf("model = %q, want %q", cfg.model, "sonnet")
	}

	WithMaxTurns(10)(&cfg)
	if cfg.maxTurns != 10 {
		t.Errorf("maxTurns = %d, want %d", cfg.maxTurns, 10)
	}

	WithAllowedTools("Bash(*)", "Write(*)")(&cfg)
	if len(cfg.allowedTools) != 2 {
		t.Errorf("allowedTools length = %d, want 2", len(cfg.allowedTools))
	}
	if cfg.allowedTools[0] != "Bash(*)" {
		t.Errorf("allowedTools[0] = %q, want %q", cfg.allowedTools[0], "Bash(*)")
	}

	WithSystemPrompt("be helpful")(&cfg)
	if cfg.systemPrompt != "be helpful" {
		t.Errorf("systemPrompt = %q, want %q", cfg.systemPrompt, "be helpful")
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

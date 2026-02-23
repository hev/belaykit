package belaykit

import "io"

// RunConfig holds per-run configuration. Exported so sub-packages (agent
// implementations) can read the resolved options.
type RunConfig struct {
	Model           string
	MaxTurns        int
	MaxOutputTokens int
	AllowedTools    []string
	DisallowedTools []string
	OutputStream    io.Writer
	EventHandler    EventHandler
	SystemPrompt    string
	TraceID         string
}

// RunOption configures a single Run invocation.
type RunOption func(*RunConfig)

// NewRunConfig resolves a set of RunOptions into a RunConfig.
func NewRunConfig(opts ...RunOption) RunConfig {
	var cfg RunConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithModel sets the model for this run, overriding the client default.
func WithModel(model string) RunOption {
	return func(cfg *RunConfig) {
		cfg.Model = model
	}
}

// WithMaxTurns sets the maximum number of agentic turns.
func WithMaxTurns(n int) RunOption {
	return func(cfg *RunConfig) {
		cfg.MaxTurns = n
	}
}

// WithMaxOutputTokens sets the maximum number of output tokens for the response.
func WithMaxOutputTokens(n int) RunOption {
	return func(cfg *RunConfig) {
		cfg.MaxOutputTokens = n
	}
}

// WithAllowedTools sets the tools the model is allowed to use.
func WithAllowedTools(tools ...string) RunOption {
	return func(cfg *RunConfig) {
		cfg.AllowedTools = tools
	}
}

// WithDisallowedTools sets tools the model is NOT allowed to use.
func WithDisallowedTools(tools ...string) RunOption {
	return func(cfg *RunConfig) {
		cfg.DisallowedTools = tools
	}
}

// WithOutputStream directs streaming assistant text to the given writer.
// This is a convenience alternative to WithEventHandler for callers that
// just want to see the text stream.
func WithOutputStream(w io.Writer) RunOption {
	return func(cfg *RunConfig) {
		cfg.OutputStream = w
	}
}

// WithEventHandler sets the event handler for this run, overriding the
// client-level default set via WithDefaultEventHandler.
func WithEventHandler(h EventHandler) RunOption {
	return func(cfg *RunConfig) {
		cfg.EventHandler = h
	}
}

// WithSystemPrompt sets the system prompt for this run.
func WithSystemPrompt(s string) RunOption {
	return func(cfg *RunConfig) {
		cfg.SystemPrompt = s
	}
}

// WithTraceID associates this run with an observability trace.
// The trace ID is included in the CompletionRecord sent to the
// ObservabilityProvider. Use ObservabilityProvider.StartTrace to
// create a trace and obtain the ID.
func WithTraceID(id string) RunOption {
	return func(cfg *RunConfig) {
		cfg.TraceID = id
	}
}

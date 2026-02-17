package claude

import "io"

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithExecutable sets the path to the claude CLI binary.
func WithExecutable(path string) ClientOption {
	return func(c *Client) {
		c.executable = path
	}
}

// WithDefaultModel sets the default model for all runs.
func WithDefaultModel(model string) ClientOption {
	return func(c *Client) {
		c.defaultModel = model
	}
}

// WithDefaultEventHandler sets the default event handler for all runs.
func WithDefaultEventHandler(h EventHandler) ClientOption {
	return func(c *Client) {
		c.eventHandler = h
	}
}

// WithObservability sets the observability provider for tracing and completion
// recording. When set, Run automatically calls RecordCompletion on the provider
// with data from the result event. Use WithTraceID to associate runs with traces.
func WithObservability(provider ObservabilityProvider) ClientOption {
	return func(c *Client) {
		c.observability = provider
	}
}

// runConfig holds per-run configuration.
type runConfig struct {
	model           string
	maxTurns        int
	maxOutputTokens int
	allowedTools    []string
	outputStream    io.Writer
	eventHandler    EventHandler
	systemPrompt    string
	traceID         string
}

// RunOption configures a single Run invocation.
type RunOption func(*runConfig)

// WithModel sets the model for this run, overriding the client default.
func WithModel(model string) RunOption {
	return func(cfg *runConfig) {
		cfg.model = model
	}
}

// WithMaxTurns sets the maximum number of agentic turns.
func WithMaxTurns(n int) RunOption {
	return func(cfg *runConfig) {
		cfg.maxTurns = n
	}
}

// WithMaxOutputTokens sets the maximum number of output tokens for the response.
func WithMaxOutputTokens(n int) RunOption {
	return func(cfg *runConfig) {
		cfg.maxOutputTokens = n
	}
}

// WithAllowedTools sets the tools the model is allowed to use.
func WithAllowedTools(tools ...string) RunOption {
	return func(cfg *runConfig) {
		cfg.allowedTools = tools
	}
}

// WithOutputStream directs streaming assistant text to the given writer.
// This is a convenience alternative to WithEventHandler for callers that
// just want to see the text stream.
func WithOutputStream(w io.Writer) RunOption {
	return func(cfg *runConfig) {
		cfg.outputStream = w
	}
}

// WithEventHandler sets the event handler for this run, overriding the
// client-level default set via WithDefaultEventHandler.
func WithEventHandler(h EventHandler) RunOption {
	return func(cfg *runConfig) {
		cfg.eventHandler = h
	}
}

// WithSystemPrompt sets the system prompt for this run.
func WithSystemPrompt(s string) RunOption {
	return func(cfg *runConfig) {
		cfg.systemPrompt = s
	}
}

// WithTraceID associates this run with an observability trace.
// The trace ID is included in the CompletionRecord sent to the
// ObservabilityProvider. Use ObservabilityProvider.StartTrace to
// create a trace and obtain the ID.
func WithTraceID(id string) RunOption {
	return func(cfg *runConfig) {
		cfg.traceID = id
	}
}

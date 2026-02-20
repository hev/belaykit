package codex

import rack "go-rack"

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithExecutable sets the path to the codex CLI binary.
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
func WithDefaultEventHandler(h rack.EventHandler) ClientOption {
	return func(c *Client) {
		c.eventHandler = h
	}
}

// WithObservability sets the observability provider for tracing and completion
// recording. When set, Run automatically calls RecordCompletion on result.
func WithObservability(provider rack.ObservabilityProvider) ClientOption {
	return func(c *Client) {
		c.observability = provider
	}
}

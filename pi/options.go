package pi

import "belaykit"

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithExecutable sets the path to the pi CLI binary.
func WithExecutable(path string) ClientOption {
	return func(c *Client) {
		c.executable = path
	}
}

// WithDefaultModel sets the default model for all runs. Supports pi's
// provider/id format (e.g. "anthropic/claude-sonnet-4-20250514") and
// optional thinking suffix (e.g. "sonnet:high").
func WithDefaultModel(model string) ClientOption {
	return func(c *Client) {
		c.defaultModel = model
	}
}

// WithDefaultProvider sets the default provider for all runs (e.g. "anthropic",
// "openai", "google"). Can be omitted if the model string uses provider/id format.
func WithDefaultProvider(provider string) ClientOption {
	return func(c *Client) {
		c.defaultProvider = provider
	}
}

// WithDefaultThinking sets the default thinking level for all runs.
// Valid levels: "off", "minimal", "low", "medium", "high", "xhigh".
func WithDefaultThinking(level string) ClientOption {
	return func(c *Client) {
		c.defaultThinking = level
	}
}

// WithDefaultEventHandler sets the default event handler for all runs.
func WithDefaultEventHandler(h belaykit.EventHandler) ClientOption {
	return func(c *Client) {
		c.eventHandler = h
	}
}

// WithObservability sets the observability provider for tracing and completion
// recording. When set, Run automatically calls RecordCompletion on the provider
// with data extracted from agent_end messages.
func WithObservability(provider belaykit.ObservabilityProvider) ClientOption {
	return func(c *Client) {
		c.observability = provider
	}
}

// WithTools sets the built-in tools to enable (e.g. "read", "bash", "edit", "write").
// By default pi enables read, bash, edit, write.
func WithTools(tools ...string) ClientOption {
	return func(c *Client) {
		c.tools = tools
	}
}

// WithNoTools disables all built-in tools. Extension tools still work.
func WithNoTools() ClientOption {
	return func(c *Client) {
		c.noTools = true
	}
}

// WithExtension adds an extension to load (path, npm, or git source).
func WithExtension(source string) ClientOption {
	return func(c *Client) {
		c.extensions = append(c.extensions, source)
	}
}

// WithWorkDir sets the working directory for the pi process.
func WithWorkDir(dir string) ClientOption {
	return func(c *Client) {
		c.workDir = dir
	}
}

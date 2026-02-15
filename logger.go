package claude

import (
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	colorReset   = "\033[0m"
	colorGray    = "\033[90m"
	colorGreen   = "\033[32m"
	colorCyan    = "\033[36m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorBoldRed = "\033[1;31m"
	colorYellow  = "\033[33m"
)

const (
	maxToolInputLen  = 200
	maxToolResultLen = 500
)

// LoggerOption configures a Logger.
type LoggerOption func(*loggerConfig)

type loggerConfig struct {
	system        bool
	assistant     bool
	toolUse       bool
	toolResult    bool
	result        bool
	tokens        bool
	content       bool
	contextWindow int
	agentName     string
	modelName     string
	pricing       ModelPricing
	clock         func() time.Time // for testing
}

// LogSystem toggles logging of system init events.
func LogSystem(on bool) LoggerOption {
	return func(cfg *loggerConfig) { cfg.system = on }
}

// LogAssistant toggles logging of assistant text events.
func LogAssistant(on bool) LoggerOption {
	return func(cfg *loggerConfig) { cfg.assistant = on }
}

// LogToolUse toggles logging of tool invocation events.
func LogToolUse(on bool) LoggerOption {
	return func(cfg *loggerConfig) { cfg.toolUse = on }
}

// LogToolResult toggles logging of tool result events.
func LogToolResult(on bool) LoggerOption {
	return func(cfg *loggerConfig) { cfg.toolResult = on }
}

// LogResult toggles logging of result and error result events.
func LogResult(on bool) LoggerOption {
	return func(cfg *loggerConfig) { cfg.result = on }
}

// LogTokens enables estimated token usage and context window tracking on each
// log line. Use WithContextWindow to set the context window size; otherwise
// the default of 200,000 tokens is used.
func LogTokens(on bool) LoggerOption {
	return func(cfg *loggerConfig) { cfg.tokens = on }
}

// LogContent toggles logging of event body text (assistant text, tool
// inputs/results, etc.). When disabled, only the event prefix and stats
// block are shown. Useful for monitoring token usage without verbose output.
func LogContent(on bool) LoggerOption {
	return func(cfg *loggerConfig) { cfg.content = on }
}

// WithContextWindow sets the context window size in tokens for percentage
// calculations. Has no effect unless LogTokens is enabled. You can use
// ContextWindowForModel to look up the size by model name.
func WithContextWindow(tokens int) LoggerOption {
	return func(cfg *loggerConfig) { cfg.contextWindow = tokens }
}

// WithAgentName sets the agent name displayed as a prefix in token tracking stats.
// Has no effect unless LogTokens is enabled.
func WithAgentName(name string) LoggerOption {
	return func(cfg *loggerConfig) { cfg.agentName = name }
}

// WithModelName sets the model name displayed in token tracking stats.
// Also sets pricing and context window for the model automatically.
// Has no effect unless LogTokens is enabled.
func WithModelName(name string) LoggerOption {
	return func(cfg *loggerConfig) {
		cfg.modelName = name
		cfg.pricing = PricingForModel(name)
		cfg.contextWindow = ContextWindowForModel(name)
	}
}

// NewLogger returns an EventHandler that writes color-coded log lines to w.
// All event types are enabled by default; use LoggerOption functions to disable specific types.
func NewLogger(w io.Writer, opts ...LoggerOption) EventHandler {
	cfg := loggerConfig{
		system:        true,
		assistant:     true,
		toolUse:       true,
		toolResult:    true,
		result:        true,
		content:       true,
		contextWindow: 200_000,
		pricing:       PricingForModel(""),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	now := time.Now
	if cfg.clock != nil {
		now = cfg.clock
	}

	var mu sync.Mutex
	sessionStart := now()
	var inputTokens, outputTokens int

	return func(e Event) {
		// Reset per-session counters when a new session starts
		if e.Type == EventSystem && e.Subtype == "init" {
			mu.Lock()
			inputTokens = 0
			outputTokens = 0
			sessionStart = now()
			mu.Unlock()
		}

		var color, prefix, body string

		switch e.Type {
		case EventSystem:
			if !cfg.system {
				return
			}
			color = colorGray
			prefix = "[system]"
			body = e.Subtype
			if e.SessionID != "" {
				body += " session=" + e.SessionID
			}
		case EventAssistant:
			if !cfg.assistant {
				return
			}
			color = colorGreen
			prefix = "[assistant]"
			body = e.Text
		case EventToolUse:
			if !cfg.toolUse {
				return
			}
			color = colorCyan
			prefix = "[tool_use]"
			body = e.ToolName
			if len(e.ToolInput) > 0 {
				input := string(e.ToolInput)
				body += " " + truncate(input, maxToolInputLen)
			}
		case EventToolResult:
			if !cfg.toolResult {
				return
			}
			color = colorBlue
			prefix = "[tool_result]"
			body = truncate(e.Text, maxToolResultLen)
		case EventResult:
			if !cfg.result {
				return
			}
			color = colorMagenta
			prefix = "[result]"
			body = fmt.Sprintf("turns=%d duration=%dms", e.NumTurns, e.Duration)
		case EventResultError:
			if !cfg.result {
				return
			}
			color = colorBoldRed
			prefix = "[error]"
			body = e.Text
		default:
			return
		}

		var line string
		if cfg.content {
			line = fmt.Sprintf("%s%s%s %s", color, prefix, colorReset, body)
		} else {
			line = fmt.Sprintf("%s%s%s", color, prefix, colorReset)
		}

		if cfg.tokens {
			// Estimate tokens and classify as input or output
			in, out := classifyEventTokens(e)

			mu.Lock()
			inputTokens += in
			outputTokens += out
			totalTokens := inputTokens + outputTokens
			elapsed := now().Sub(sessionStart)
			pct := float64(totalTokens) / float64(cfg.contextWindow) * 100
			cost := cfg.pricing.Cost(inputTokens, outputTokens)

			var labelPrefix string
			switch {
			case cfg.agentName != "" && cfg.modelName != "":
				labelPrefix = cfg.agentName + " | " + cfg.modelName + " | "
			case cfg.agentName != "":
				labelPrefix = cfg.agentName + " | "
			case cfg.modelName != "":
				labelPrefix = cfg.modelName + " | "
			}

			var stats string
			if labelPrefix != "" {
				stats = fmt.Sprintf(" %s[%s~%s in + ~%s out / %s (%.1f%%) | $%.4f | %s]%s",
					colorYellow,
					labelPrefix,
					formatTokenCount(inputTokens),
					formatTokenCount(outputTokens),
					formatTokenCount(cfg.contextWindow),
					pct,
					cost,
					formatDuration(elapsed),
					colorReset,
				)
			} else {
				stats = fmt.Sprintf(" %s[~%s in + ~%s out / %s (%.1f%%) | $%.4f | %s]%s",
					colorYellow,
					formatTokenCount(inputTokens),
					formatTokenCount(outputTokens),
					formatTokenCount(cfg.contextWindow),
					pct,
					cost,
					formatDuration(elapsed),
					colorReset,
				)
			}
			line += stats
			mu.Unlock()

			line += "\n"
			mu.Lock()
			w.Write([]byte(line))
			mu.Unlock()
		} else {
			line += "\n"
			mu.Lock()
			w.Write([]byte(line))
			mu.Unlock()
		}
	}
}

// classifyEventTokens estimates token counts for an event, split into
// input tokens (context fed to the model) and output tokens (model-generated).
func classifyEventTokens(e Event) (input, output int) {
	switch e.Type {
	case EventAssistant:
		// Model-generated text
		return 0, EstimateTokens(e.Text)
	case EventToolUse:
		// Model-generated tool invocation
		return 0, EstimateTokens(e.ToolName) + EstimateTokens(string(e.ToolInput))
	case EventToolResult:
		// Tool output fed back as input
		return EstimateTokens(e.Text), 0
	case EventSystem:
		// System prompt / init overhead
		return EstimateTokens(e.Subtype) + EstimateTokens(e.SessionID), 0
	default:
		return EstimateTokens(e.Text), 0
	}
}

// formatTokenCount formats a token count with K suffix for readability.
func formatTokenCount(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// formatDuration formats a duration as a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

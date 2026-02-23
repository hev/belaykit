package belaykit

import (
	"fmt"
	"io"
	"strings"
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
	colorDim     = "\033[2m"
)

const (
	maxToolInputLen  = 200
	maxToolResultLen = 500
	thermobarWidth   = 20
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
// calculations. Has no effect unless LogTokens is enabled.
func WithContextWindow(tokens int) LoggerOption {
	return func(cfg *loggerConfig) { cfg.contextWindow = tokens }
}

// WithAgentName sets the agent name displayed in the event prefix tag.
func WithAgentName(name string) LoggerOption {
	return func(cfg *loggerConfig) { cfg.agentName = name }
}

// WithModelName sets the model name displayed in the event prefix tag.
// This only sets the display name. Use WithPricing and WithContextWindow
// to configure pricing and context window data.
func WithModelName(name string) LoggerOption {
	return func(cfg *loggerConfig) {
		cfg.modelName = name
	}
}

// WithPricing sets the model pricing used for cost calculations in token
// tracking. Has no effect unless LogTokens is enabled.
func WithPricing(pricing ModelPricing) LoggerOption {
	return func(cfg *loggerConfig) {
		cfg.pricing = pricing
	}
}

// NewLogger returns an EventHandler that writes color-coded log lines to w.
// All event types are enabled by default; use LoggerOption functions to disable specific types.
//
// Output format uses a hierarchical layout where tool use and results are
// indented under assistant turn headers. The assistant prefix includes model
// and agent names (e.g. [assistant:opus:researcher]) while the stats block
// contains only metrics. A thermobar visualizes context window usage.
func NewLogger(w io.Writer, opts ...LoggerOption) EventHandler {
	cfg := loggerConfig{
		system:        true,
		assistant:     true,
		toolUse:       true,
		toolResult:    true,
		result:        true,
		tokens:        true,
		content:       true,
		contextWindow: 200_000,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	now := time.Now
	if cfg.clock != nil {
		now = cfg.clock
	}

	// Build the assistant prefix tag with model/agent names.
	// e.g. [assistant], [assistant:opus], [assistant:opus:researcher]
	assistantPrefix := buildTagPrefix("assistant", cfg.modelName, cfg.agentName)

	var mu sync.Mutex
	sessionStart := now()
	var inputTokens, outputTokens int
	var inTurn bool

	return func(e Event) {
		mu.Lock()
		defer mu.Unlock()

		// Reset per-session counters when a new session starts.
		if e.Type == EventSystem && e.Subtype == "init" {
			inputTokens = 0
			outputTokens = 0
			sessionStart = now()
			inTurn = false
		}

		// Always count tokens when tracking is enabled.
		if cfg.tokens {
			in, out := classifyEventTokens(e)
			inputTokens += in
			outputTokens += out
		}

		switch e.Type {
		case EventSystem:
			return

		case EventAssistantStart:
			if !cfg.assistant {
				return
			}
			inTurn = true
			line := fmt.Sprintf("%s%s%s", colorGreen, assistantPrefix, colorReset)
			if cfg.tokens {
				line += "  " + formatThermobar(inputTokens+outputTokens, cfg.contextWindow)
				line += "  " + formatMetrics(cfg, inputTokens, outputTokens, now().Sub(sessionStart))
			}
			w.Write([]byte(line + "\n"))

		case EventAssistant:
			if !cfg.assistant {
				return
			}
			if !inTurn {
				inTurn = true
				header := fmt.Sprintf("%s%s%s", colorGreen, assistantPrefix, colorReset)
				if cfg.tokens {
					header += "  " + formatThermobar(inputTokens+outputTokens, cfg.contextWindow)
					header += "  " + formatMetrics(cfg, inputTokens, outputTokens, now().Sub(sessionStart))
				}
				w.Write([]byte(header + "\n"))
			}
			if cfg.content {
				w.Write([]byte("  " + e.Text + "\n"))
			}

		case EventToolUse:
			if !cfg.toolUse {
				return
			}
			if !inTurn {
				if cfg.assistant {
					inTurn = true
					header := fmt.Sprintf("%s%s%s", colorGreen, assistantPrefix, colorReset)
					if cfg.tokens {
						header += "  " + formatThermobar(inputTokens+outputTokens, cfg.contextWindow)
						header += "  " + formatMetrics(cfg, inputTokens, outputTokens, now().Sub(sessionStart))
					}
					w.Write([]byte(header + "\n"))
				}
			}
			indent := "  "
			if !inTurn {
				indent = ""
			}
			body := " " + e.ToolName
			if cfg.content && len(e.ToolInput) > 0 {
				body += " " + truncate(string(e.ToolInput), maxToolInputLen)
			}
			w.Write([]byte(fmt.Sprintf("%s%s[tool_use]%s%s\n", indent, colorCyan, colorReset, body)))

		case EventToolResult:
			if !cfg.toolResult {
				return
			}
			indent := "  "
			if !inTurn {
				indent = ""
			}
			var body string
			if cfg.content {
				body = " " + truncate(e.Text, maxToolResultLen)
			}
			w.Write([]byte(fmt.Sprintf("%s%s[tool_result]%s%s\n", indent, colorBlue, colorReset, body)))

		case EventResult:
			if !cfg.result {
				return
			}
			inTurn = false
			body := fmt.Sprintf(" turns=%d duration=%dms", e.NumTurns, e.Duration)
			line := fmt.Sprintf("%s[result]%s%s", colorMagenta, colorReset, body)
			if cfg.tokens {
				line += "  " + formatThermobar(inputTokens+outputTokens, cfg.contextWindow)
				line += "  " + formatMetrics(cfg, inputTokens, outputTokens, now().Sub(sessionStart))
			}
			w.Write([]byte(line + "\n"))

		case EventResultError:
			if !cfg.result {
				return
			}
			inTurn = false
			var body string
			if cfg.content {
				body = " " + e.Text
			}
			w.Write([]byte(fmt.Sprintf("%s[error]%s%s\n", colorBoldRed, colorReset, body)))

		default:
			return
		}
	}
}

// buildTagPrefix builds a bracket-delimited tag with optional model and agent suffixes.
// e.g. buildTagPrefix("assistant", "opus", "researcher") => "[assistant:opus:researcher]"
func buildTagPrefix(tag, model, agent string) string {
	parts := []string{tag}
	if model != "" {
		parts = append(parts, model)
	}
	if agent != "" {
		parts = append(parts, agent)
	}
	return "[" + strings.Join(parts, ":") + "]"
}

// formatThermobar renders a visual progress bar for context window usage.
// Uses colored block characters: green < 65%, yellow 65-85%, red > 85%.
func formatThermobar(totalTokens, contextWindow int) string {
	pct := float64(totalTokens) / float64(contextWindow) * 100
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}
	filled := int(pct / 100 * float64(thermobarWidth))
	if filled > thermobarWidth {
		filled = thermobarWidth
	}

	var barColor string
	switch {
	case pct >= 85:
		barColor = colorBoldRed
	case pct >= 65:
		barColor = colorYellow
	default:
		barColor = colorGreen
	}

	return fmt.Sprintf("%s%s%s%s%s %s%.1f%%%s",
		barColor,
		strings.Repeat("█", filled),
		colorDim,
		strings.Repeat("░", thermobarWidth-filled),
		colorReset,
		barColor,
		pct,
		colorReset,
	)
}

// formatMetrics renders the stats block with only metrics (tokens, cost, duration).
func formatMetrics(cfg loggerConfig, inputTokens, outputTokens int, elapsed time.Duration) string {
	cost := cfg.pricing.Cost(inputTokens, outputTokens)
	return fmt.Sprintf("%s~%s in + ~%s out | $%.4f | %s%s",
		colorYellow,
		formatTokenCount(inputTokens),
		formatTokenCount(outputTokens),
		cost,
		formatDuration(elapsed),
		colorReset,
	)
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
	case EventAssistantStart:
		return 0, 0
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

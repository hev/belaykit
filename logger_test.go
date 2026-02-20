package rack

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoggerDefaultEmitsAllTypes(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	events := []Event{
		{Type: EventSystem, Subtype: "init", SessionID: "sess-1"},
		{Type: EventAssistantStart},
		{Type: EventAssistant, Text: "hello"},
		{Type: EventToolUse, ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"ls"}`)},
		{Type: EventToolResult, Text: "file1.go", ToolID: "tool_01"},
		{Type: EventResult, NumTurns: 2, Duration: 3000},
		{Type: EventResultError, Text: "something failed", IsError: true},
	}

	for _, e := range events {
		logger(e)
	}

	output := buf.String()
	// System events are currently suppressed (no hook support yet)
	for _, prefix := range []string{"[assistant]", "[tool_use]", "[tool_result]", "[result]", "[error]"} {
		if !strings.Contains(output, prefix) {
			t.Errorf("output missing prefix %q", prefix)
		}
	}
}

func TestLoggerSystemEventsSupressed(t *testing.T) {
	// System events are currently suppressed since hook support isn't implemented.
	var buf bytes.Buffer
	logger := NewLogger(&buf)
	logger(Event{Type: EventSystem, Subtype: "init", SessionID: "abc"})
	if buf.Len() != 0 {
		t.Errorf("expected no output for system events (currently suppressed), got %q", buf.String())
	}
}

func TestLoggerToggleAssistant(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogAssistant(false))
	logger(Event{Type: EventAssistant, Text: "hello"})
	if buf.Len() != 0 {
		t.Errorf("expected no output when assistant disabled, got %q", buf.String())
	}
}

func TestLoggerToggleAssistantStart(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogAssistant(false))
	logger(Event{Type: EventAssistantStart})
	if buf.Len() != 0 {
		t.Errorf("expected no output for assistant_start when assistant disabled, got %q", buf.String())
	}
}

func TestLoggerToggleToolUse(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogToolUse(false))
	logger(Event{Type: EventToolUse, ToolName: "Bash"})
	if buf.Len() != 0 {
		t.Errorf("expected no output when tool_use disabled, got %q", buf.String())
	}
}

func TestLoggerToggleToolResult(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogToolResult(false))
	logger(Event{Type: EventToolResult, Text: "result text"})
	if buf.Len() != 0 {
		t.Errorf("expected no output when tool_result disabled, got %q", buf.String())
	}
}

func TestLoggerToggleResult(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogResult(false))

	logger(Event{Type: EventResult, NumTurns: 1, Duration: 100})
	logger(Event{Type: EventResultError, Text: "error", IsError: true})

	if buf.Len() != 0 {
		t.Errorf("expected no output when result disabled, got %q", buf.String())
	}
}

func TestLoggerColorCodes(t *testing.T) {
	tests := []struct {
		name      string
		event     Event
		wantColor string
	}{
		// system events are suppressed, skip color check
		{"assistant_start", Event{Type: EventAssistantStart}, colorGreen},
		{"assistant", Event{Type: EventAssistant, Text: "hi"}, colorGreen},
		{"tool_use", Event{Type: EventToolUse, ToolName: "Bash"}, colorCyan},
		{"tool_result", Event{Type: EventToolResult, Text: "out"}, colorBlue},
		{"result", Event{Type: EventResult}, colorMagenta},
		{"error", Event{Type: EventResultError, Text: "err"}, colorBoldRed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewLogger(&buf)
			logger(tt.event)
			if !strings.Contains(buf.String(), tt.wantColor) {
				t.Errorf("output %q missing color code %q", buf.String(), tt.wantColor)
			}
			if !strings.Contains(buf.String(), colorReset) {
				t.Errorf("output %q missing reset code", buf.String())
			}
		})
	}
}

func TestLoggerSystemStillResetsTokens(t *testing.T) {
	// Even though system events are suppressed for display, they must still
	// reset token counters on init for correct accounting.
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithContextWindow(100),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistantStart})
	logger(Event{Type: EventAssistant, Text: strings.Repeat("a", 40)}) // 10 output tokens

	// New session resets counters
	logger(Event{Type: EventSystem, Subtype: "init", SessionID: "s2"})
	logger(Event{Type: EventAssistantStart})
	logger(Event{Type: EventAssistant, Text: "abcd"}) // 1 output token
	logger(Event{Type: EventResult, NumTurns: 1, Duration: 100})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "~1 out") {
		t.Errorf("expected ~1 output after session reset, got %q", lastLine)
	}
}

func TestLoggerToolUseFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)
	logger(Event{Type: EventToolUse, ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"ls -la"}`)})
	output := buf.String()
	if !strings.Contains(output, "[tool_use]") {
		t.Errorf("missing [tool_use] prefix in %q", output)
	}
	if !strings.Contains(output, "Bash") {
		t.Errorf("missing tool name in %q", output)
	}
	if !strings.Contains(output, `{"command":"ls -la"}`) {
		t.Errorf("missing tool input in %q", output)
	}
}

func TestLoggerResultFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)
	logger(Event{Type: EventResult, NumTurns: 5, Duration: 12000})
	output := buf.String()
	if !strings.Contains(output, "[result]") {
		t.Errorf("missing [result] prefix in %q", output)
	}
	if !strings.Contains(output, "turns=5") {
		t.Errorf("missing turns in %q", output)
	}
	if !strings.Contains(output, "duration=12000ms") {
		t.Errorf("missing duration in %q", output)
	}
}

func TestLoggerTruncation(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	longInput := strings.Repeat("x", 300)
	logger(Event{Type: EventToolUse, ToolName: "Bash", ToolInput: json.RawMessage(longInput)})
	output := buf.String()
	if !strings.Contains(output, "...") {
		t.Error("expected truncation ellipsis for long tool input")
	}

	buf.Reset()
	longResult := strings.Repeat("y", 600)
	logger(Event{Type: EventToolResult, Text: longResult})
	output = buf.String()
	if !strings.Contains(output, "...") {
		t.Error("expected truncation ellipsis for long tool result")
	}
}

func TestLoggerConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger(Event{Type: EventAssistant, Text: "hello"})
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 100 {
		t.Errorf("expected at least 100 lines, got %d", len(lines))
	}
}

func TestLoggerUnknownEventType(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)
	logger(Event{Type: EventType("unknown")})
	if buf.Len() != 0 {
		t.Errorf("expected no output for unknown event type, got %q", buf.String())
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short = %q, want %q", got, "short")
	}
	if got := truncate("exactly10!", 10); got != "exactly10!" {
		t.Errorf("truncate exact = %q, want %q", got, "exactly10!")
	}
	if got := truncate("this is longer than ten", 10); got != "this is lo..." {
		t.Errorf("truncate long = %q, want %q", got, "this is lo...")
	}
}

// --- Indentation tests ---

func TestLoggerToolUseIndentedUnderAssistant(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	logger(Event{Type: EventAssistantStart})
	logger(Event{Type: EventAssistant, Text: "Let me check."})
	logger(Event{Type: EventToolUse, ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"ls"}`)})
	logger(Event{Type: EventToolResult, Text: "file1.go"})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for i, line := range lines {
		if i == 0 {
			if strings.HasPrefix(line, "  ") {
				t.Errorf("header line should not be indented: %q", line)
			}
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("line %d should be indented: %q", i, line)
		}
	}
}

func TestLoggerToolUseNotIndentedWithoutAssistant(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogAssistant(false))

	logger(Event{Type: EventToolUse, ToolName: "Bash"})
	output := buf.String()
	if strings.HasPrefix(output, "  ") {
		t.Errorf("tool_use should not be indented when assistant disabled: %q", output)
	}
}

// --- Assistant start / turn header tests ---

func TestLoggerAssistantStartEmitsHeader(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	logger(Event{Type: EventAssistantStart})
	output := buf.String()
	if !strings.Contains(output, "[assistant]") {
		t.Errorf("expected [assistant] header, got %q", output)
	}
}

func TestLoggerAssistantStartWithModelAgent(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, WithModelName("opus"), WithAgentName("researcher"))

	logger(Event{Type: EventAssistantStart})
	output := buf.String()
	if !strings.Contains(output, "[assistant:opus:researcher]") {
		t.Errorf("expected [assistant:opus:researcher] in output, got %q", output)
	}
}

func TestLoggerAssistantStartShowsThermobar(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithContextWindow(100),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	// Feed some tokens first
	logger(Event{Type: EventAssistant, Text: strings.Repeat("a", 40)}) // 10 output tokens

	buf.Reset()
	logger(Event{Type: EventAssistantStart})
	output := buf.String()

	if !strings.Contains(output, "█") {
		t.Errorf("expected thermobar filled blocks in output, got %q", output)
	}
	if !strings.Contains(output, "░") {
		t.Errorf("expected thermobar empty blocks in output, got %q", output)
	}
	if !strings.Contains(output, "10.0%") {
		t.Errorf("expected 10.0%% in output, got %q", output)
	}
}

func TestLoggerAutoHeaderOnAssistantText(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	logger(Event{Type: EventAssistant, Text: "hello"})
	output := buf.String()
	if !strings.Contains(output, "[assistant]") {
		t.Errorf("expected auto-emitted [assistant] header, got %q", output)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("expected text body, got %q", output)
	}
}

func TestLoggerNoDoubleHeader(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	logger(Event{Type: EventAssistantStart})
	logger(Event{Type: EventAssistant, Text: "hello"})
	logger(Event{Type: EventAssistant, Text: "world"})

	output := buf.String()
	count := strings.Count(output, "[assistant]")
	if count != 1 {
		t.Errorf("expected exactly 1 [assistant] header, got %d in:\n%s", count, output)
	}
}

func TestLoggerNewTurnAfterResult(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	logger(Event{Type: EventAssistantStart})
	logger(Event{Type: EventAssistant, Text: "first turn"})
	logger(Event{Type: EventResult, NumTurns: 1, Duration: 100})

	logger(Event{Type: EventAssistant, Text: "second turn"})

	output := buf.String()
	count := strings.Count(output, "[assistant]")
	if count != 2 {
		t.Errorf("expected 2 [assistant] headers (one per turn), got %d in:\n%s", count, output)
	}
}

// --- Token tracking tests ---

func TestLoggerTokenTracking(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithContextWindow(200_000),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistantStart})
	output := buf.String()

	if !strings.Contains(output, "in +") {
		t.Errorf("expected 'in +' in output, got %q", output)
	}
	if !strings.Contains(output, "out") {
		t.Errorf("expected 'out' in output, got %q", output)
	}
	if !strings.Contains(output, "%") {
		t.Errorf("expected percentage in output, got %q", output)
	}
}

func TestLoggerTokenTrackingInputOutput(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithModelName("haiku"),
		WithPricing(ModelPricing{InputPerMTok: 1, OutputPerMTok: 5}),
		WithContextWindow(200_000),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	// Tool result = input tokens (40 chars = 10 tokens)
	logger(Event{Type: EventToolResult, Text: strings.Repeat("a", 40)})
	// Assistant text = output tokens (20 chars = 5 tokens)
	logger(Event{Type: EventAssistant, Text: strings.Repeat("b", 20)})

	output := buf.String()

	if !strings.Contains(output, "~10 in") {
		t.Errorf("expected ~10 input tokens, got %q", output)
	}
	if !strings.Contains(output, "~5 out") {
		t.Errorf("expected ~5 output tokens, got %q", output)
	}
}

func TestLoggerTokenTrackingAccumulates(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithContextWindow(100),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistantStart})
	logger(Event{Type: EventAssistant, Text: "abcd"})
	logger(Event{Type: EventAssistant, Text: "efghijkl"})
	logger(Event{Type: EventResult, NumTurns: 1, Duration: 100})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "~3 out") {
		t.Errorf("expected accumulated ~3 output tokens in result, got %q", lastLine)
	}
}

func TestLoggerTokenTrackingResetsOnNewSession(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithContextWindow(100),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventSystem, Subtype: "init", SessionID: "session-1"})
	logger(Event{Type: EventAssistantStart})
	logger(Event{Type: EventAssistant, Text: strings.Repeat("a", 40)})
	logger(Event{Type: EventAssistant, Text: strings.Repeat("b", 40)})

	logger(Event{Type: EventSystem, Subtype: "init", SessionID: "session-2"})
	logger(Event{Type: EventAssistantStart})
	logger(Event{Type: EventAssistant, Text: "abcd"})
	logger(Event{Type: EventResult, NumTurns: 1, Duration: 100})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	lastLine := lines[len(lines)-1]

	if !strings.Contains(lastLine, "~1 out") {
		t.Errorf("expected ~1 output token after session reset, got %q", lastLine)
	}
}

func TestLoggerTokenTrackingCostCalculation(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithModelName("haiku"),
		WithPricing(ModelPricing{InputPerMTok: 1, OutputPerMTok: 5}),
		WithContextWindow(200_000),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	// 4000 chars = 1000 input tokens at $1/MTok = $0.001
	logger(Event{Type: EventToolResult, Text: strings.Repeat("x", 4000)})
	// 4000 chars = 1000 output tokens at $5/MTok = $0.005
	logger(Event{Type: EventAssistant, Text: strings.Repeat("y", 4000)})

	output := buf.String()
	// Total cost = $0.001 + $0.005 = $0.006
	if !strings.Contains(output, "$0.0060") {
		t.Errorf("expected cost $0.0060, got %q", output)
	}
}

func TestLoggerTokenTrackingCostByModel(t *testing.T) {
	tests := []struct {
		model    string
		pricing  ModelPricing
		wantCost string // 1000 input + 1000 output tokens
	}{
		// opus: 1000 * $5/M + 1000 * $25/M = $0.005 + $0.025 = $0.0300
		{"opus", ModelPricing{InputPerMTok: 5, OutputPerMTok: 25}, "$0.0300"},
		// sonnet: 1000 * $3/M + 1000 * $15/M = $0.003 + $0.015 = $0.0180
		{"sonnet", ModelPricing{InputPerMTok: 3, OutputPerMTok: 15}, "$0.0180"},
		// haiku: 1000 * $1/M + 1000 * $5/M = $0.001 + $0.005 = $0.0060
		{"haiku", ModelPricing{InputPerMTok: 1, OutputPerMTok: 5}, "$0.0060"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			var buf bytes.Buffer
			frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			logger := NewLogger(&buf,
				LogTokens(true),
				WithModelName(tt.model),
				WithPricing(tt.pricing),
				WithContextWindow(200_000),
				func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
			)

			// 4000 chars = 1000 tokens each
			logger(Event{Type: EventToolResult, Text: strings.Repeat("x", 4000)})
			logger(Event{Type: EventAssistant, Text: strings.Repeat("y", 4000)})

			output := buf.String()
			if !strings.Contains(output, tt.wantCost) {
				t.Errorf("expected cost %s for %s, got %q", tt.wantCost, tt.model, output)
			}
		})
	}
}

func TestLoggerTokenTrackingTiming(t *testing.T) {
	var buf bytes.Buffer
	elapsed := time.Duration(0)
	logger := NewLogger(&buf,
		LogTokens(true),
		func(cfg *loggerConfig) {
			cfg.clock = func() time.Time {
				return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(elapsed)
			}
		},
	)

	elapsed = 2500 * time.Millisecond
	logger(Event{Type: EventAssistantStart})
	output := buf.String()
	if !strings.Contains(output, "2.5s") {
		t.Errorf("expected 2.5s elapsed in output, got %q", output)
	}
}

func TestLoggerTokenTrackingDisabledByDefault(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	logger(Event{Type: EventAssistantStart})
	output := buf.String()
	if strings.Contains(output, "█") || strings.Contains(output, "░") {
		t.Errorf("thermobar should not appear when tokens disabled, got %q", output)
	}
}

func TestLoggerTokenTrackingToolUseIsOutput(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithContextWindow(1000),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistantStart})
	buf.Reset()
	logger(Event{Type: EventToolUse, ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"ls"}`)})
	logger(Event{Type: EventResult, NumTurns: 1, Duration: 100})
	output := buf.String()
	if !strings.Contains(output, "~5 out") {
		t.Errorf("expected ~5 output tokens for tool use, got %q", output)
	}
	if !strings.Contains(output, "~0 in") {
		t.Errorf("expected ~0 input tokens for tool use, got %q", output)
	}
}

func TestLoggerTokenTrackingPercentage(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithContextWindow(100),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistant, Text: strings.Repeat("a", 40)})
	output := buf.String()
	if !strings.Contains(output, "10.0%") {
		t.Errorf("expected 10.0%% in output, got %q", output)
	}
}

// --- Thermobar tests ---

func TestFormatThermobar(t *testing.T) {
	tests := []struct {
		total   int
		window  int
		wantPct string
	}{
		{0, 200_000, "0.0%"},
		{10, 100, "10.0%"},
		{65, 100, "65.0%"},
		{85, 100, "85.0%"},
		{100, 100, "100.0%"},
	}

	for _, tt := range tests {
		bar := formatThermobar(tt.total, tt.window)
		if !strings.Contains(bar, tt.wantPct) {
			t.Errorf("formatThermobar(%d, %d) = %q, want pct %q", tt.total, tt.window, bar, tt.wantPct)
		}
		if !strings.Contains(bar, "█") || !strings.Contains(bar, "░") {
			if tt.total > 0 && tt.total < tt.window {
				t.Errorf("expected both block chars in %q", bar)
			}
		}
	}
}

func TestFormatThermobarColorThresholds(t *testing.T) {
	bar := formatThermobar(50, 100)
	if !strings.Contains(bar, colorGreen) {
		t.Errorf("expected green for 50%%, got %q", bar)
	}

	bar = formatThermobar(70, 100)
	if !strings.Contains(bar, colorYellow) {
		t.Errorf("expected yellow for 70%%, got %q", bar)
	}

	bar = formatThermobar(90, 100)
	if !strings.Contains(bar, colorBoldRed) {
		t.Errorf("expected bold red for 90%%, got %q", bar)
	}
}

// --- Model/agent in prefix tests ---

func TestLoggerModelNameInPrefix(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, WithModelName("opus"))

	logger(Event{Type: EventAssistantStart})
	output := buf.String()
	if !strings.Contains(output, "[assistant:opus]") {
		t.Errorf("expected [assistant:opus] in output, got %q", output)
	}
}

func TestLoggerAgentNameInPrefix(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, WithAgentName("researcher"))

	logger(Event{Type: EventAssistantStart})
	output := buf.String()
	if !strings.Contains(output, "[assistant:researcher]") {
		t.Errorf("expected [assistant:researcher] in output, got %q", output)
	}
}

func TestLoggerModelAndAgentInPrefix(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, WithModelName("opus"), WithAgentName("researcher"))

	logger(Event{Type: EventAssistantStart})
	output := buf.String()
	if !strings.Contains(output, "[assistant:opus:researcher]") {
		t.Errorf("expected [assistant:opus:researcher] in output, got %q", output)
	}
}

func TestLoggerModelNotInStats(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithModelName("opus"),
		WithAgentName("researcher"),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistantStart})
	output := buf.String()

	if !strings.Contains(output, "[assistant:opus:researcher]") {
		t.Errorf("expected model/agent in prefix, got %q", output)
	}
	pctIdx := strings.Index(output, "%")
	if pctIdx > 0 {
		stats := output[pctIdx:]
		if strings.Contains(stats, "opus |") || strings.Contains(stats, "researcher |") {
			t.Errorf("model/agent names should not be in stats block, got %q", stats)
		}
	}
}

func TestWithModelNameSetsDisplayOnly(t *testing.T) {
	var cfg loggerConfig
	WithModelName("haiku")(&cfg)

	if cfg.modelName != "haiku" {
		t.Errorf("modelName = %q, want %q", cfg.modelName, "haiku")
	}
	// WithModelName should NOT auto-set pricing or context window
	if cfg.pricing.InputPerMTok != 0 {
		t.Errorf("pricing.InputPerMTok = %f, want 0 (not auto-set)", cfg.pricing.InputPerMTok)
	}
	if cfg.contextWindow != 0 {
		t.Errorf("contextWindow = %d, want 0 (not auto-set)", cfg.contextWindow)
	}
}

func TestWithPricingSetsValues(t *testing.T) {
	var cfg loggerConfig
	WithPricing(ModelPricing{InputPerMTok: 1, OutputPerMTok: 5})(&cfg)

	if cfg.pricing.InputPerMTok != 1 {
		t.Errorf("pricing.InputPerMTok = %f, want 1", cfg.pricing.InputPerMTok)
	}
	if cfg.pricing.OutputPerMTok != 5 {
		t.Errorf("pricing.OutputPerMTok = %f, want 5", cfg.pricing.OutputPerMTok)
	}
}

// --- Format helper tests ---

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{200_000, "200.0K"},
		{1_000_000, "1.0M"},
		{1_500_000, "1.5M"},
	}

	for _, tt := range tests {
		got := formatTokenCount(tt.n)
		if got != tt.want {
			t.Errorf("formatTokenCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "500ms"},
		{0, "0ms"},
		{1 * time.Second, "1.0s"},
		{2500 * time.Millisecond, "2.5s"},
		{59 * time.Second, "59.0s"},
		{60 * time.Second, "1.0m"},
		{90 * time.Second, "1.5m"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestLoggerLogContentFalse(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		LogContent(false),
		WithAgentName("extract"),
		WithModelName("haiku"),
		WithPricing(ModelPricing{InputPerMTok: 1, OutputPerMTok: 5}),
		WithContextWindow(200_000),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistantStart})
	logger(Event{Type: EventAssistant, Text: "this should not appear in output"})
	output := buf.String()

	if !strings.Contains(output, "[assistant:haiku:extract]") {
		t.Errorf("expected [assistant:haiku:extract] prefix, got %q", output)
	}
	if !strings.Contains(output, "░") {
		t.Errorf("expected thermobar in output, got %q", output)
	}
	if strings.Contains(output, "this should not appear") {
		t.Errorf("body text should be suppressed with LogContent(false), got %q", output)
	}
}

func TestLoggerLogContentFalseStillTracksTokens(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		LogContent(false),
		WithContextWindow(100),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistantStart})
	logger(Event{Type: EventAssistant, Text: strings.Repeat("a", 40)})
	logger(Event{Type: EventResult, NumTurns: 1, Duration: 100})

	output := buf.String()
	if !strings.Contains(output, "~10 out") {
		t.Errorf("expected token tracking with content disabled, got %q", output)
	}
}

func TestLoggerLogContentDefaultTrue(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	logger(Event{Type: EventAssistant, Text: "visible body"})
	output := buf.String()
	if !strings.Contains(output, "visible body") {
		t.Errorf("body should be visible by default, got %q", output)
	}
}

func TestLoggerLogContentFalseShowsToolName(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogContent(false))

	logger(Event{Type: EventToolUse, ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"ls"}`)})
	output := buf.String()
	if !strings.Contains(output, "Bash") {
		t.Errorf("tool name should show even with content disabled, got %q", output)
	}
	if strings.Contains(output, `{"command":"ls"}`) {
		t.Errorf("tool input should be hidden with content disabled, got %q", output)
	}
}

func TestClassifyEventTokens(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		wantIn  int
		wantOut int
	}{
		{
			"assistant is output",
			Event{Type: EventAssistant, Text: "abcdefgh"},
			0, 2,
		},
		{
			"tool use is output",
			Event{Type: EventToolUse, ToolName: "Read", ToolInput: json.RawMessage(`{"path":"/foo"}`)},
			0, 5, // "Read"=1 + `{"path":"/foo"}`=4
		},
		{
			"tool result is input",
			Event{Type: EventToolResult, Text: "file contents here"},
			5, 0,
		},
		{
			"system is input",
			Event{Type: EventSystem, Subtype: "init", SessionID: "abc-123"},
			3, 0, // "init"=1 + "abc-123"=2
		},
		{
			"assistant_start is zero",
			Event{Type: EventAssistantStart},
			0, 0,
		},
		{
			"empty",
			Event{Type: EventAssistant, Text: ""},
			0, 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIn, gotOut := classifyEventTokens(tt.event)
			if gotIn != tt.wantIn {
				t.Errorf("input tokens = %d, want %d", gotIn, tt.wantIn)
			}
			if gotOut != tt.wantOut {
				t.Errorf("output tokens = %d, want %d", gotOut, tt.wantOut)
			}
		})
	}
}

// --- Build tag prefix tests ---

func TestBuildTagPrefix(t *testing.T) {
	tests := []struct {
		tag, model, agent string
		want              string
	}{
		{"assistant", "", "", "[assistant]"},
		{"assistant", "opus", "", "[assistant:opus]"},
		{"assistant", "", "researcher", "[assistant:researcher]"},
		{"assistant", "opus", "researcher", "[assistant:opus:researcher]"},
	}

	for _, tt := range tests {
		got := buildTagPrefix(tt.tag, tt.model, tt.agent)
		if got != tt.want {
			t.Errorf("buildTagPrefix(%q, %q, %q) = %q, want %q",
				tt.tag, tt.model, tt.agent, got, tt.want)
		}
	}
}

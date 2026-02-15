package claude

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
	for _, prefix := range []string{"[system]", "[assistant]", "[tool_use]", "[tool_result]", "[result]", "[error]"} {
		if !strings.Contains(output, prefix) {
			t.Errorf("output missing prefix %q", prefix)
		}
	}
}

func TestLoggerToggleSystem(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogSystem(false))
	logger(Event{Type: EventSystem, Subtype: "init"})
	if buf.Len() != 0 {
		t.Errorf("expected no output when system disabled, got %q", buf.String())
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
		{"system", Event{Type: EventSystem, Subtype: "init"}, colorGray},
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

func TestLoggerSystemFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)
	logger(Event{Type: EventSystem, Subtype: "init", SessionID: "abc"})
	output := buf.String()
	if !strings.Contains(output, "[system]") {
		t.Errorf("missing [system] prefix in %q", output)
	}
	if !strings.Contains(output, "init session=abc") {
		t.Errorf("missing body in %q", output)
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
	if len(lines) != 100 {
		t.Errorf("expected 100 lines, got %d", len(lines))
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

// --- Token tracking tests ---

func TestLoggerTokenTracking(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithContextWindow(200_000),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistant, Text: "Hello, world!"})
	output := buf.String()

	// Should contain input/output token stats
	if !strings.Contains(output, "in +") {
		t.Errorf("expected 'in +' in output, got %q", output)
	}
	if !strings.Contains(output, "out /") {
		t.Errorf("expected 'out /' in output, got %q", output)
	}
	if !strings.Contains(output, "200.0K") {
		t.Errorf("expected context window in output, got %q", output)
	}
	if !strings.Contains(output, "%)") {
		t.Errorf("expected percentage in output, got %q", output)
	}
}

func TestLoggerTokenTrackingInputOutput(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithModelName("haiku"),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	// Tool result = input tokens (40 chars = 10 tokens)
	logger(Event{Type: EventToolResult, Text: strings.Repeat("a", 40)})
	// Assistant text = output tokens (20 chars = 5 tokens)
	logger(Event{Type: EventAssistant, Text: strings.Repeat("b", 20)})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Second line should show ~10 in + ~5 out
	if !strings.Contains(lines[1], "~10 in") {
		t.Errorf("expected ~10 input tokens, got %q", lines[1])
	}
	if !strings.Contains(lines[1], "~5 out") {
		t.Errorf("expected ~5 output tokens, got %q", lines[1])
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

	// "abcd" = 1 output token, "efghijkl" = 2 output tokens => cumulative = 3
	logger(Event{Type: EventAssistant, Text: "abcd"})
	logger(Event{Type: EventAssistant, Text: "efghijkl"})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// Second line should show accumulated ~3 output tokens
	if !strings.Contains(lines[1], "~3 out") {
		t.Errorf("expected accumulated ~3 output tokens in second line, got %q", lines[1])
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

	// First session: accumulate some tokens
	logger(Event{Type: EventSystem, Subtype: "init", SessionID: "session-1"})
	logger(Event{Type: EventAssistant, Text: strings.Repeat("a", 40)}) // 10 output tokens
	logger(Event{Type: EventAssistant, Text: strings.Repeat("b", 40)}) // 10 more = 20 output total

	// New session should reset
	logger(Event{Type: EventSystem, Subtype: "init", SessionID: "session-2"})
	logger(Event{Type: EventAssistant, Text: "abcd"}) // 1 output token after reset

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	lastLine := lines[len(lines)-1]

	// After reset: init adds ~4 input tokens ("init"=1 + "session-2"=3),
	// then "abcd" adds 1 output token. Should NOT show 20 output tokens.
	if !strings.Contains(lastLine, "~1 out") {
		t.Errorf("expected ~1 output token after session reset, got %q", lastLine)
	}
}

func TestLoggerTokenTrackingCostCalculation(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithModelName("haiku"), // $1/MTok input, $5/MTok output
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	// 4000 chars = 1000 input tokens at $1/MTok = $0.001
	logger(Event{Type: EventToolResult, Text: strings.Repeat("x", 4000)})
	// 4000 chars = 1000 output tokens at $5/MTok = $0.005
	logger(Event{Type: EventAssistant, Text: strings.Repeat("y", 4000)})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	lastLine := lines[len(lines)-1]

	// Total cost = $0.001 + $0.005 = $0.006
	if !strings.Contains(lastLine, "$0.0060") {
		t.Errorf("expected cost $0.0060, got %q", lastLine)
	}
}

func TestLoggerTokenTrackingCostByModel(t *testing.T) {
	tests := []struct {
		model    string
		wantCost string // 1000 input + 1000 output tokens
	}{
		// opus: 1000 * $5/M + 1000 * $25/M = $0.005 + $0.025 = $0.0300
		{"opus", "$0.0300"},
		// sonnet: 1000 * $3/M + 1000 * $15/M = $0.003 + $0.015 = $0.0180
		{"sonnet", "$0.0180"},
		// haiku: 1000 * $1/M + 1000 * $5/M = $0.001 + $0.005 = $0.0060
		{"haiku", "$0.0060"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			var buf bytes.Buffer
			frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			logger := NewLogger(&buf,
				LogTokens(true),
				WithModelName(tt.model),
				func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
			)

			// 4000 chars = 1000 tokens each
			logger(Event{Type: EventToolResult, Text: strings.Repeat("x", 4000)})
			logger(Event{Type: EventAssistant, Text: strings.Repeat("y", 4000)})

			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			lastLine := lines[len(lines)-1]
			if !strings.Contains(lastLine, tt.wantCost) {
				t.Errorf("expected cost %s for %s, got %q", tt.wantCost, tt.model, lastLine)
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
	logger(Event{Type: EventAssistant, Text: "hello"})
	output := buf.String()
	if !strings.Contains(output, "2.5s") {
		t.Errorf("expected 2.5s elapsed in output, got %q", output)
	}
}

func TestLoggerTokenTrackingDisabledByDefault(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	logger(Event{Type: EventAssistant, Text: "hello"})
	output := buf.String()
	if strings.Contains(output, "tokens") {
		t.Errorf("token tracking should be disabled by default, got %q", output)
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

	// ToolName "Bash" = 1 token, ToolInput `{"command":"ls"}` = 4 tokens => 5 output total
	logger(Event{Type: EventToolUse, ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"ls"}`)})
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

	// 40 chars = 10 output tokens out of 100 = 10.0%
	logger(Event{Type: EventAssistant, Text: strings.Repeat("a", 40)})
	output := buf.String()
	if !strings.Contains(output, "10.0%") {
		t.Errorf("expected 10.0%% in output, got %q", output)
	}
}

func TestLoggerTokenTrackingWithModelName(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithModelName("opus"),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistant, Text: "hello"})
	output := buf.String()
	if !strings.Contains(output, "opus |") {
		t.Errorf("expected model name 'opus' in output, got %q", output)
	}
}

func TestLoggerTokenTrackingWithoutModelName(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistant, Text: "hello"})
	output := buf.String()
	// Without model name, stats should start with [~
	if !strings.Contains(output, "[~") {
		t.Errorf("expected stats to start with [~ when no model name, got %q", output)
	}
}

func TestLoggerTokenTrackingWithYellowColor(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistant, Text: "hello"})
	output := buf.String()
	if !strings.Contains(output, colorYellow) {
		t.Errorf("expected yellow color for stats, got %q", output)
	}
}

func TestWithModelNameSetsPricingAndWindow(t *testing.T) {
	var cfg loggerConfig
	WithModelName("haiku")(&cfg)

	if cfg.modelName != "haiku" {
		t.Errorf("modelName = %q, want %q", cfg.modelName, "haiku")
	}
	if cfg.pricing.InputPerMTok != 1 {
		t.Errorf("pricing.InputPerMTok = %f, want 1", cfg.pricing.InputPerMTok)
	}
	if cfg.pricing.OutputPerMTok != 5 {
		t.Errorf("pricing.OutputPerMTok = %f, want 5", cfg.pricing.OutputPerMTok)
	}
	if cfg.contextWindow != 200_000 {
		t.Errorf("contextWindow = %d, want 200000", cfg.contextWindow)
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

func TestLoggerTokenTrackingWithAgentName(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithAgentName("researcher"),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistant, Text: "hello"})
	output := buf.String()
	if !strings.Contains(output, "researcher |") {
		t.Errorf("expected agent name 'researcher' in output, got %q", output)
	}
}

func TestLoggerTokenTrackingWithAgentNameAndModel(t *testing.T) {
	var buf bytes.Buffer
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	logger := NewLogger(&buf,
		LogTokens(true),
		WithAgentName("researcher"),
		WithModelName("opus"),
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistant, Text: "hello"})
	output := buf.String()
	if !strings.Contains(output, "researcher | opus |") {
		t.Errorf("expected 'researcher | opus |' in output, got %q", output)
	}
}

func TestLoggerTokenTrackingAgentNameWithoutTokens(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, WithAgentName("researcher"))

	logger(Event{Type: EventAssistant, Text: "hello"})
	output := buf.String()
	if strings.Contains(output, "researcher") {
		t.Errorf("agent name should not appear when tokens disabled, got %q", output)
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
		func(cfg *loggerConfig) { cfg.clock = func() time.Time { return frozen } },
	)

	logger(Event{Type: EventAssistant, Text: "this should not appear in output"})
	output := buf.String()

	// Stats and prefix should be present
	if !strings.Contains(output, "[assistant]") {
		t.Errorf("expected [assistant] prefix, got %q", output)
	}
	if !strings.Contains(output, "extract | haiku |") {
		t.Errorf("expected stats block, got %q", output)
	}
	// Body content should be suppressed
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

	// 40 chars = 10 output tokens
	logger(Event{Type: EventAssistant, Text: strings.Repeat("a", 40)})
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

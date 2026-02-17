package claude

// ObservabilityProvider is the interface that observability partners implement
// to receive trace and completion data from Claude CLI invocations.
//
// Implementations should be safe for concurrent use. Errors should be handled
// internally (logged or ignored) — observability failures must not affect the
// primary application flow.
type ObservabilityProvider interface {
	// StartSession begins a new observability session and returns a session ID.
	// Use sessions to group related Run invocations.
	StartSession(metadata map[string]any) string

	// StartTrace begins a new trace within the current session.
	// Returns a trace ID that can be passed to Run via WithTraceID
	// and must be passed to EndTrace when the operation completes.
	StartTrace(config TraceConfig, inputs map[string]any) string

	// EndTrace completes a trace with the given outputs.
	EndTrace(traceID string, outputs map[string]any)

	// RecordCompletion records a single LLM completion.
	// This is called automatically by Run when a result event is received.
	RecordCompletion(record CompletionRecord)
}

// TraceConfig configures a new trace.
type TraceConfig struct {
	Name        string         // Identifier for this trace type (e.g., "extract", "summarize")
	DisplayName string         // Human-readable name for dashboards
	Metadata    map[string]any // Additional metadata
}

// CompletionRecord holds data about a single LLM completion.
// Fields are populated automatically from the Claude CLI result event.
type CompletionRecord struct {
	TraceID    string  // Trace this completion belongs to (from WithTraceID)
	SessionID  string  // Claude CLI session ID (from the system init event)
	Prompt     string  // The input prompt
	Response   string  // The result text
	Model      string  // The model used (resolved from aliases)
	CostUSD    float64 // Total cost in USD
	DurationMS int64   // Total duration in milliseconds
	NumTurns   int     // Number of agentic turns
	IsError    bool    // Whether the result was an error
}

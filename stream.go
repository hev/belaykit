package belaykit

import "encoding/json"

// EventType identifies the type of streaming event.
type EventType string

const (
	// EventAssistant is emitted for each chunk of assistant text.
	EventAssistant EventType = "assistant"
	// EventAssistantStart is emitted when a new assistant turn begins,
	// before the LLM response arrives. Useful for showing a "waiting" indicator.
	EventAssistantStart EventType = "assistant_start"
	// EventResult is emitted once with the final result text.
	EventResult EventType = "result"
	// EventSystem is emitted for session initialization events.
	EventSystem EventType = "system"
	// EventToolUse is emitted when the assistant invokes a tool.
	EventToolUse EventType = "tool_use"
	// EventToolResult is emitted when a tool returns its output.
	EventToolResult EventType = "tool_result"
	// EventResultError is emitted when the result indicates an error.
	EventResultError EventType = "result_error"
	// EventPhase is emitted by callers to mark phase boundaries within a run.
	// belaykit does not generate this automatically; callers emit it to tell
	// observability providers that a new phase is starting.
	EventPhase EventType = "phase"
)

// Event represents a parsed streaming event from an agent.
type Event struct {
	Type    EventType
	Text    string
	RawJSON json.RawMessage

	// System event fields
	SessionID string
	Subtype   string // "init", "success", "error"

	// Tool use fields
	ToolName  string
	ToolID    string
	ToolInput json.RawMessage

	// Result fields
	CostUSD  float64
	Duration int64 // milliseconds
	NumTurns int
	IsError  bool

	// Phase fields (only set for EventPhase events)
	PhaseName string
}

// EventHandler processes streaming events from a Run invocation.
type EventHandler func(Event)

// StreamEvent is the raw JSON structure from Claude's stream-json output.
// Exported for use by agent implementations.
type StreamEvent struct {
	Type       string         `json:"type"`
	Subtype    string         `json:"subtype,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	Message    *StreamMessage `json:"message,omitempty"`
	Result     string         `json:"result,omitempty"`
	CostUSD    float64        `json:"cost_usd,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	NumTurns   int            `json:"num_turns,omitempty"`
	IsError    bool           `json:"is_error,omitempty"`
}

// StreamMessage holds the message content from a streaming event.
type StreamMessage struct {
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a single content block in a streaming message.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

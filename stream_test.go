package belaykit

import (
	"encoding/json"
	"testing"
)

func TestStreamEventParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string
		wantText string
	}{
		{
			name:     "assistant event with text",
			input:    `{"type":"assistant","message":{"content":[{"type":"text","text":"hello world"}]}}`,
			wantType: "assistant",
			wantText: "hello world",
		},
		{
			name:     "result event",
			input:    `{"type":"result","result":"final answer"}`,
			wantType: "result",
			wantText: "final answer",
		},
		{
			name:     "assistant event with multiple content blocks",
			input:    `{"type":"assistant","message":{"content":[{"type":"text","text":"part1"},{"type":"text","text":"part2"}]}}`,
			wantType: "assistant",
			wantText: "part1",
		},
		{
			name:     "assistant event with non-text block",
			input:    `{"type":"assistant","message":{"content":[{"type":"tool_use","text":""}]}}`,
			wantType: "assistant",
			wantText: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var event StreamEvent
			if err := json.Unmarshal([]byte(tt.input), &event); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			if event.Type != tt.wantType {
				t.Errorf("type = %q, want %q", event.Type, tt.wantType)
			}

			switch event.Type {
			case "assistant":
				if event.Message == nil || len(event.Message.Content) == 0 {
					if tt.wantText != "" {
						t.Fatalf("no message content, expected %q", tt.wantText)
					}
					return
				}
				got := event.Message.Content[0].Text
				if got != tt.wantText {
					t.Errorf("text = %q, want %q", got, tt.wantText)
				}
			case "result":
				if event.Result != tt.wantText {
					t.Errorf("result = %q, want %q", event.Result, tt.wantText)
				}
			}
		})
	}
}

func TestEventTypes(t *testing.T) {
	if EventAssistant != "assistant" {
		t.Errorf("EventAssistant = %q, want %q", EventAssistant, "assistant")
	}
	if EventResult != "result" {
		t.Errorf("EventResult = %q, want %q", EventResult, "result")
	}
	if EventSystem != "system" {
		t.Errorf("EventSystem = %q, want %q", EventSystem, "system")
	}
	if EventToolUse != "tool_use" {
		t.Errorf("EventToolUse = %q, want %q", EventToolUse, "tool_use")
	}
	if EventToolResult != "tool_result" {
		t.Errorf("EventToolResult = %q, want %q", EventToolResult, "tool_result")
	}
	if EventResultError != "result_error" {
		t.Errorf("EventResultError = %q, want %q", EventResultError, "result_error")
	}
}

func TestSystemEventParsing(t *testing.T) {
	input := `{"type":"system","subtype":"init","session_id":"abc-123"}`
	var event StreamEvent
	if err := json.Unmarshal([]byte(input), &event); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if event.Type != "system" {
		t.Errorf("type = %q, want %q", event.Type, "system")
	}
	if event.Subtype != "init" {
		t.Errorf("subtype = %q, want %q", event.Subtype, "init")
	}
	if event.SessionID != "abc-123" {
		t.Errorf("session_id = %q, want %q", event.SessionID, "abc-123")
	}
}

func TestToolUseContentBlockParsing(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tool_01","name":"Bash","input":{"command":"ls"}}]}}`
	var event StreamEvent
	if err := json.Unmarshal([]byte(input), &event); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if event.Message == nil || len(event.Message.Content) == 0 {
		t.Fatal("expected content blocks")
	}
	block := event.Message.Content[0]
	if block.Type != "tool_use" {
		t.Errorf("block type = %q, want %q", block.Type, "tool_use")
	}
	if block.Name != "Bash" {
		t.Errorf("name = %q, want %q", block.Name, "Bash")
	}
	if block.ID != "tool_01" {
		t.Errorf("id = %q, want %q", block.ID, "tool_01")
	}
	if string(block.Input) != `{"command":"ls"}` {
		t.Errorf("input = %s, want %s", block.Input, `{"command":"ls"}`)
	}
}

func TestToolResultContentBlockParsing(t *testing.T) {
	input := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tool_01","content":"file1.go\nfile2.go"}]}}`
	var event StreamEvent
	if err := json.Unmarshal([]byte(input), &event); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if event.Message == nil || len(event.Message.Content) == 0 {
		t.Fatal("expected content blocks")
	}
	block := event.Message.Content[0]
	if block.Type != "tool_result" {
		t.Errorf("block type = %q, want %q", block.Type, "tool_result")
	}
	if block.ToolUseID != "tool_01" {
		t.Errorf("tool_use_id = %q, want %q", block.ToolUseID, "tool_01")
	}
	if block.Content != "file1.go\nfile2.go" {
		t.Errorf("content = %q, want %q", block.Content, "file1.go\nfile2.go")
	}
}

func TestResultEventWithMetadata(t *testing.T) {
	input := `{"type":"result","subtype":"success","result":"done","cost_usd":0.0123,"duration_ms":4500,"num_turns":3}`
	var event StreamEvent
	if err := json.Unmarshal([]byte(input), &event); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if event.Type != "result" {
		t.Errorf("type = %q, want %q", event.Type, "result")
	}
	if event.Subtype != "success" {
		t.Errorf("subtype = %q, want %q", event.Subtype, "success")
	}
	if event.Result != "done" {
		t.Errorf("result = %q, want %q", event.Result, "done")
	}
	if event.CostUSD != 0.0123 {
		t.Errorf("cost_usd = %f, want %f", event.CostUSD, 0.0123)
	}
	if event.DurationMS != 4500 {
		t.Errorf("duration_ms = %d, want %d", event.DurationMS, 4500)
	}
	if event.NumTurns != 3 {
		t.Errorf("num_turns = %d, want %d", event.NumTurns, 3)
	}
}

func TestResultErrorEventParsing(t *testing.T) {
	input := `{"type":"result","subtype":"error","result":"something went wrong","is_error":true}`
	var event StreamEvent
	if err := json.Unmarshal([]byte(input), &event); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if event.Type != "result" {
		t.Errorf("type = %q, want %q", event.Type, "result")
	}
	if event.Subtype != "error" {
		t.Errorf("subtype = %q, want %q", event.Subtype, "error")
	}
	if !event.IsError {
		t.Error("is_error should be true")
	}
	if event.Result != "something went wrong" {
		t.Errorf("result = %q, want %q", event.Result, "something went wrong")
	}
}

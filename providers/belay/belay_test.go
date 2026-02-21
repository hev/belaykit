package belay

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	rack "go-rack"
)

func approxEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// traceJSON is used to unmarshal and verify the output JSON structure.
type traceJSON struct {
	ID        string      `json:"id"`
	NodeType  string      `json:"node_type"`
	AgentName string      `json:"agent_name"`
	Model     string      `json:"model,omitempty"`
	Duration  int64       `json:"duration_ms"`
	CostUSD   float64     `json:"cost_usd"`
	Children  []traceJSON `json:"children,omitempty"`
}

func TestMultiPhaseRun(t *testing.T) {
	dir := t.TempDir()
	p := NewProvider(WithDir(dir))

	_ = p.StartSession(nil)
	tid := p.StartTrace(rack.TraceConfig{Name: "test-pipeline"}, nil)
	handler := p.EventHandler()

	// Phase 0: subreddit-discovery
	handler(rack.Event{Type: rack.EventPhase, PhaseName: "subreddit-discovery"})
	handler(rack.Event{Type: rack.EventToolUse, ToolName: "search_reddit", ToolID: "tool_01"})
	handler(rack.Event{Type: rack.EventToolResult, ToolID: "tool_01"})
	p.RecordCompletion(rack.CompletionRecord{
		Model:      "claude-opus-4-20250514",
		CostUSD:    0.05,
		DurationMS: 1200,
	})

	// Phase 1: thread-discovery
	handler(rack.Event{Type: rack.EventPhase, PhaseName: "thread-discovery"})
	handler(rack.Event{Type: rack.EventToolUse, ToolName: "fetch_threads", ToolID: "tool_02"})
	handler(rack.Event{Type: rack.EventToolResult, ToolID: "tool_02"})
	handler(rack.Event{Type: rack.EventToolUse, ToolName: "analyze_thread", ToolID: "tool_03"})
	handler(rack.Event{Type: rack.EventToolResult, ToolID: "tool_03"})
	p.RecordCompletion(rack.CompletionRecord{
		Model:      "claude-opus-4-20250514",
		CostUSD:    0.08,
		DurationMS: 2000,
	})

	p.EndTrace(tid, nil)

	// Read and verify the output file
	path := filepath.Join(dir, tid+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading trace file: %v", err)
	}

	var root traceJSON
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshaling trace: %v", err)
	}

	// Root node
	if root.NodeType != "trace" {
		t.Errorf("root node_type = %q, want %q", root.NodeType, "trace")
	}
	if root.AgentName != "test-pipeline" {
		t.Errorf("root agent_name = %q, want %q", root.AgentName, "test-pipeline")
	}
	if root.Duration < 0 {
		t.Errorf("root duration_ms = %d, want >= 0", root.Duration)
	}

	// Expected children: marker, phase, marker, phase = 4
	if len(root.Children) != 4 {
		t.Fatalf("root children = %d, want 4 (marker, phase, marker, phase)", len(root.Children))
	}

	// First marker
	if root.Children[0].NodeType != "marker" {
		t.Errorf("child[0] node_type = %q, want %q", root.Children[0].NodeType, "marker")
	}

	// First phase
	phase0 := root.Children[1]
	if phase0.NodeType != "phase" {
		t.Errorf("child[1] node_type = %q, want %q", phase0.NodeType, "phase")
	}
	if phase0.AgentName != "subreddit-discovery" {
		t.Errorf("child[1] agent_name = %q, want %q", phase0.AgentName, "subreddit-discovery")
	}
	if phase0.Model != "claude-opus-4-20250514" {
		t.Errorf("child[1] model = %q, want %q", phase0.Model, "claude-opus-4-20250514")
	}
	if !approxEqual(phase0.CostUSD, 0.05, 1e-9) {
		t.Errorf("child[1] cost_usd = %f, want %f", phase0.CostUSD, 0.05)
	}
	if len(phase0.Children) != 1 {
		t.Errorf("phase0 children = %d, want 1", len(phase0.Children))
	} else if phase0.Children[0].AgentName != "search_reddit" {
		t.Errorf("phase0 tool agent_name = %q, want %q", phase0.Children[0].AgentName, "search_reddit")
	}

	// Second marker
	if root.Children[2].NodeType != "marker" {
		t.Errorf("child[2] node_type = %q, want %q", root.Children[2].NodeType, "marker")
	}

	// Second phase
	phase1 := root.Children[3]
	if phase1.NodeType != "phase" {
		t.Errorf("child[3] node_type = %q, want %q", phase1.NodeType, "phase")
	}
	if phase1.AgentName != "thread-discovery" {
		t.Errorf("child[3] agent_name = %q, want %q", phase1.AgentName, "thread-discovery")
	}
	if !approxEqual(phase1.CostUSD, 0.08, 1e-9) {
		t.Errorf("child[3] cost_usd = %f, want %f", phase1.CostUSD, 0.08)
	}
	if len(phase1.Children) != 2 {
		t.Errorf("phase1 children = %d, want 2", len(phase1.Children))
	} else {
		if phase1.Children[0].AgentName != "fetch_threads" {
			t.Errorf("phase1 tool[0] agent_name = %q, want %q", phase1.Children[0].AgentName, "fetch_threads")
		}
		if phase1.Children[1].AgentName != "analyze_thread" {
			t.Errorf("phase1 tool[1] agent_name = %q, want %q", phase1.Children[1].AgentName, "analyze_thread")
		}
	}
}

func TestToolCallNodeType(t *testing.T) {
	dir := t.TempDir()
	p := NewProvider(WithDir(dir))

	_ = p.StartSession(nil)
	tid := p.StartTrace(rack.TraceConfig{Name: "tool-test"}, nil)
	handler := p.EventHandler()

	handler(rack.Event{Type: rack.EventPhase, PhaseName: "phase-1"})
	handler(rack.Event{Type: rack.EventToolUse, ToolName: "Bash", ToolID: "t1"})
	handler(rack.Event{Type: rack.EventToolResult, ToolID: "t1"})
	p.EndTrace(tid, nil)

	data, err := os.ReadFile(filepath.Join(dir, tid+".json"))
	if err != nil {
		t.Fatal(err)
	}

	var root traceJSON
	json.Unmarshal(data, &root)

	// marker, phase
	phase := root.Children[1]
	if len(phase.Children) != 1 {
		t.Fatalf("phase children = %d, want 1", len(phase.Children))
	}
	tool := phase.Children[0]
	if tool.NodeType != "tool_call" {
		t.Errorf("tool node_type = %q, want %q", tool.NodeType, "tool_call")
	}
	if tool.AgentName != "Bash" {
		t.Errorf("tool agent_name = %q, want %q", tool.AgentName, "Bash")
	}
}

func TestDefaultPhaseCreatedWithoutExplicitPhase(t *testing.T) {
	dir := t.TempDir()
	p := NewProvider(WithDir(dir))

	_ = p.StartSession(nil)
	tid := p.StartTrace(rack.TraceConfig{Name: "no-phase"}, nil)
	handler := p.EventHandler()

	// Emit tool events without a prior EventPhase
	handler(rack.Event{Type: rack.EventToolUse, ToolName: "Read", ToolID: "t1"})
	handler(rack.Event{Type: rack.EventToolResult, ToolID: "t1"})

	p.RecordCompletion(rack.CompletionRecord{
		Model:      "claude-sonnet-4-20250514",
		CostUSD:    0.01,
		DurationMS: 500,
	})
	p.EndTrace(tid, nil)

	data, err := os.ReadFile(filepath.Join(dir, tid+".json"))
	if err != nil {
		t.Fatal(err)
	}

	var root traceJSON
	json.Unmarshal(data, &root)

	if len(root.Children) != 1 {
		t.Fatalf("root children = %d, want 1 (default phase)", len(root.Children))
	}

	phase := root.Children[0]
	if phase.NodeType != "phase" {
		t.Errorf("default phase node_type = %q, want %q", phase.NodeType, "phase")
	}
	if phase.AgentName != "default" {
		t.Errorf("default phase agent_name = %q, want %q", phase.AgentName, "default")
	}
	if phase.Model != "claude-sonnet-4-20250514" {
		t.Errorf("default phase model = %q, want %q", phase.Model, "claude-sonnet-4-20250514")
	}
	if len(phase.Children) != 1 {
		t.Fatalf("default phase children = %d, want 1", len(phase.Children))
	}
	if phase.Children[0].AgentName != "Read" {
		t.Errorf("tool agent_name = %q, want %q", phase.Children[0].AgentName, "Read")
	}
}

func TestEndTraceWithoutStartIsNoop(t *testing.T) {
	dir := t.TempDir()
	p := NewProvider(WithDir(dir))

	// EndTrace without StartTrace should not panic or write anything
	p.EndTrace("nonexistent", nil)

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected no files written, got %d", len(entries))
	}
}

func TestRecordCompletionWithoutStartTraceIsNoop(t *testing.T) {
	p := NewProvider()

	// Should not panic
	p.RecordCompletion(rack.CompletionRecord{
		Model:      "opus",
		CostUSD:    1.0,
		DurationMS: 1000,
	})
}

func TestEventHandlerWithoutStartTraceIsNoop(t *testing.T) {
	p := NewProvider()
	handler := p.EventHandler()

	// Should not panic
	handler(rack.Event{Type: rack.EventPhase, PhaseName: "orphan"})
	handler(rack.Event{Type: rack.EventToolUse, ToolName: "Bash", ToolID: "t1"})
	handler(rack.Event{Type: rack.EventToolResult, ToolID: "t1"})
}

func TestToolResultWithoutMatchingUseIsIgnored(t *testing.T) {
	dir := t.TempDir()
	p := NewProvider(WithDir(dir))

	_ = p.StartSession(nil)
	tid := p.StartTrace(rack.TraceConfig{Name: "orphan-result"}, nil)
	handler := p.EventHandler()

	handler(rack.Event{Type: rack.EventPhase, PhaseName: "phase-1"})
	// ToolResult without a prior ToolUse for this ID
	handler(rack.Event{Type: rack.EventToolResult, ToolID: "unknown"})

	p.EndTrace(tid, nil)

	data, _ := os.ReadFile(filepath.Join(dir, tid+".json"))
	var root traceJSON
	json.Unmarshal(data, &root)

	// Phase should have no tool children
	phase := root.Children[1] // [0] is marker
	if len(phase.Children) != 0 {
		t.Errorf("expected 0 tool children, got %d", len(phase.Children))
	}
}

func TestToolDurationIsRecorded(t *testing.T) {
	dir := t.TempDir()
	p := NewProvider(WithDir(dir))

	_ = p.StartSession(nil)
	tid := p.StartTrace(rack.TraceConfig{Name: "duration-test"}, nil)
	handler := p.EventHandler()

	handler(rack.Event{Type: rack.EventPhase, PhaseName: "phase-1"})
	handler(rack.Event{Type: rack.EventToolUse, ToolName: "Bash", ToolID: "t1"})
	time.Sleep(10 * time.Millisecond) // ensure nonzero duration
	handler(rack.Event{Type: rack.EventToolResult, ToolID: "t1"})
	p.EndTrace(tid, nil)

	data, _ := os.ReadFile(filepath.Join(dir, tid+".json"))
	var root traceJSON
	json.Unmarshal(data, &root)

	phase := root.Children[1]
	if len(phase.Children) != 1 {
		t.Fatalf("expected 1 tool child, got %d", len(phase.Children))
	}
	if phase.Children[0].Duration <= 0 {
		t.Errorf("tool duration_ms = %d, want > 0", phase.Children[0].Duration)
	}
}

func TestCostAccumulatesAcrossCompletions(t *testing.T) {
	dir := t.TempDir()
	p := NewProvider(WithDir(dir))

	_ = p.StartSession(nil)
	tid := p.StartTrace(rack.TraceConfig{Name: "cost-test"}, nil)
	handler := p.EventHandler()

	handler(rack.Event{Type: rack.EventPhase, PhaseName: "expensive-phase"})
	p.RecordCompletion(rack.CompletionRecord{CostUSD: 0.10, DurationMS: 100, Model: "opus"})
	p.RecordCompletion(rack.CompletionRecord{CostUSD: 0.05, DurationMS: 200, Model: "opus"})
	p.EndTrace(tid, nil)

	data, _ := os.ReadFile(filepath.Join(dir, tid+".json"))
	var root traceJSON
	json.Unmarshal(data, &root)

	phase := root.Children[1] // [0] is marker
	if !approxEqual(phase.CostUSD, 0.15, 1e-9) {
		t.Errorf("phase cost_usd = %f, want %f", phase.CostUSD, 0.15)
	}
	if phase.Duration != 300 {
		t.Errorf("phase duration_ms = %d, want %d", phase.Duration, 300)
	}
}

func TestOutputDirIsCreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "trace", "dir")
	p := NewProvider(WithDir(dir))

	_ = p.StartSession(nil)
	tid := p.StartTrace(rack.TraceConfig{Name: "mkdir-test"}, nil)
	p.EndTrace(tid, nil)

	if _, err := os.Stat(filepath.Join(dir, tid+".json")); err != nil {
		t.Errorf("expected trace file to exist in created dir: %v", err)
	}
}

func TestJSONSchemaMatchesBelayNode(t *testing.T) {
	dir := t.TempDir()
	p := NewProvider(WithDir(dir))

	_ = p.StartSession(nil)
	tid := p.StartTrace(rack.TraceConfig{Name: "schema-test"}, nil)
	handler := p.EventHandler()

	handler(rack.Event{Type: rack.EventPhase, PhaseName: "phase-1"})
	handler(rack.Event{Type: rack.EventToolUse, ToolName: "Bash", ToolID: "t1"})
	handler(rack.Event{Type: rack.EventToolResult, ToolID: "t1"})
	p.RecordCompletion(rack.CompletionRecord{Model: "opus", CostUSD: 0.01, DurationMS: 100})
	p.EndTrace(tid, nil)

	data, _ := os.ReadFile(filepath.Join(dir, tid+".json"))

	// Verify the JSON has exactly the expected field names
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("top-level unmarshal: %v", err)
	}

	expectedFields := []string{"id", "node_type", "agent_name", "duration_ms", "cost_usd", "input_tokens", "output_tokens", "children"}
	for _, f := range expectedFields {
		if _, ok := raw[f]; !ok {
			t.Errorf("missing expected field %q in JSON output", f)
		}
	}
}

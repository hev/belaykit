# Belay Integration Plan

## Context

belaykit provides a unified interface for AI coding agents (Claude, Codex) with observability via `ObservabilityProvider`. Belay is a sibling project that visualizes agent trace trees showing phases, tool calls, token usage, cost, and context window utilization.

The goal is to make belaykit write trace files that belay can read, using belay's existing `trace.Node` tree format. This positions belay as an observability add-on alongside freeplay.

There is a gap: belaykit's event system has no concept of "phases" (named groupings of tool calls within a run). Belay's tree model expects `trace > phase > tool_call`. We need a custom event type to bridge this.

---

## Changes

### 1. Add `EventPhase` to belaykit's event system

**File: `stream.go`**

Add a new event type and phase-related fields to `Event`:

```go
EventPhase EventType = "phase"  // emitted by callers to mark phase boundaries
```

Add to `Event` struct:
```go
PhaseName string // name of the phase (e.g. "subreddit-discovery")
```

This is a **caller-emitted** event — belaykit doesn't generate it automatically. The caller (e.g. hiveminer) emits `EventPhase` events between `Run()` calls to tell observability providers "a new phase is starting." The logger and slack handler will ignore unknown event types by default, so this is backwards-compatible.

### 2. Add JSON tags to belay's `trace.Node`

**File: `../belay/trace/node.go`**

Add JSON struct tags so nodes can be serialized/deserialized:

```go
type Node struct {
    ID            string  `json:"id"`
    NodeType      string  `json:"node_type"`
    AgentName     string  `json:"agent_name"`
    Model         string  `json:"model,omitempty"`
    DurationMS    int64   `json:"duration_ms"`
    CostUSD       float64 `json:"cost_usd"`
    InputTokens   int     `json:"input_tokens"`
    OutputTokens  int     `json:"output_tokens"`
    ContextWindow int     `json:"context_window,omitempty"`
    Children      []*Node `json:"children,omitempty"`
}
```

### 3. Add file-reading capability to belay

**File: `../belay/trace/read.go`** (new)

Add a function to load a `Node` tree from a JSON file:

```go
func ReadFile(path string) (*Node, error)
```

And a function to read the most recent trace from a directory:

```go
func ReadLatest(dir string) (*Node, error)
```

### 4. Create `providers/belay/` package in belaykit

**File: `providers/belay/belay.go`** (new)

A new `ObservabilityProvider` implementation that collects events and writes `trace.Node` JSON files to `.belay/traces/`.

```go
type Provider struct {
    dir          string           // output directory (default ".belay/traces")
    mu           sync.Mutex
    root         *traceNode       // in-progress trace tree
    currentPhase *traceNode       // current phase being built
    startTime    time.Time
    phaseStart   time.Time
    toolStart    map[string]time.Time // toolID -> start time
}
```

Key design decisions:
- Uses an **internal `traceNode` struct** that mirrors belay's `trace.Node` fields (to avoid importing belay as a dependency — they're separate projects). Serializes to the same JSON format.
- Implements `ObservabilityProvider` interface: `StartSession`, `StartTrace`, `EndTrace`, `RecordCompletion`.
- Also exposes an **`EventHandler`** that can be composed with the logger, allowing it to capture tool-level granularity (tool names, per-tool token estimates, per-tool timing).

#### How events map to nodes:

| belaykit event | belay node |
|---|---|
| `StartSession()` | Creates root `trace` node |
| `EventPhase` | Creates `phase` node + preceding `marker` node |
| `EventToolUse` | Starts a `tool_call` node (records start time, tool name) |
| `EventToolResult` | Completes the `tool_call` node (computes duration, tokens) |
| `RecordCompletion` | Fills in phase-level cost/model/duration from result |
| `EndTrace` or session end | Writes the tree to `.belay/traces/{session-id}.json` |

#### EventHandler function:

```go
func (p *Provider) EventHandler() rack.EventHandler
```

This returns an `EventHandler` that the caller composes alongside the logger:

```go
belay := belay.NewProvider()
handler := func(e rack.Event) {
    logger(e)
    belay.EventHandler()(e)
}
client.Run(ctx, prompt, rack.WithEventHandler(handler))
```

#### File output:

On `EndTrace` (or `RecordCompletion` if no explicit trace), writes:
```
.belay/traces/{trace-id}.json
```

The JSON matches belay's `trace.Node` schema exactly.

### 5. Usage example

```go
// Create providers
bp := belay.NewProvider(belay.WithDir(".belay/traces"))
logger := rack.NewLogger(os.Stderr)
client := claude.NewClient(
    claude.WithObservability(bp),
    claude.WithDefaultEventHandler(func(e rack.Event) {
        logger(e)
        bp.EventHandler()(e)
    }),
)

// Multi-phase pipeline
sid := bp.StartSession(map[string]any{"pipeline": "hiveminer"})
tid := bp.StartTrace(rack.TraceConfig{Name: "hiveminer-run"}, nil)

// Phase 0
handler(rack.Event{Type: rack.EventPhase, PhaseName: "subreddit-discovery"})
client.Run(ctx, "find subreddits...", rack.WithTraceID(tid), rack.WithModel("opus"))

// Phase 1
handler(rack.Event{Type: rack.EventPhase, PhaseName: "thread-discovery"})
client.Run(ctx, "find threads...", rack.WithTraceID(tid), rack.WithModel("opus"))

bp.EndTrace(tid, nil) // writes .belay/traces/{tid}.json
```

---

## Files to create/modify

| File | Action |
|---|---|
| `belaykit/stream.go` | Add `EventPhase` constant + `PhaseName` field to `Event` |
| `belaykit/providers/belay/belay.go` | New — provider implementation |
| `../belay/trace/node.go` | Add JSON struct tags |
| `../belay/trace/read.go` | New — `ReadFile` and `ReadLatest` functions |

---

## Verification

1. Write a test in `providers/belay/belay_test.go` that simulates a multi-phase run, verifies the output JSON matches belay's expected schema
2. Update belay's `main.go` to optionally read from a JSON file instead of `SampleTree()`, confirming the round-trip works
3. Run `go vet ./...` and `go test ./...` in both projects

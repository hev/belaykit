// Package belay provides an ObservabilityProvider that writes trace trees
// as JSON files compatible with the belay visualization tool.
//
// Usage:
//
//	bp := belay.NewProvider(belay.WithDir(".belay/traces"))
//	logger := belaykit.NewLogger(os.Stderr)
//	client := claude.NewClient(
//	    claude.WithObservability(bp),
//	    claude.WithDefaultEventHandler(func(e belaykit.Event) {
//	        logger(e)
//	        bp.EventHandler()(e)
//	    }),
//	)
//
//	tid := bp.StartTrace(belaykit.TraceConfig{Name: "my-run"}, nil)
//	handler(belaykit.Event{Type: belaykit.EventPhase, PhaseName: "phase-1"})
//	client.Run(ctx, prompt, belaykit.WithTraceID(tid))
//	bp.EndTrace(tid, nil) // writes JSON file
package belay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"belaykit"

	"github.com/google/uuid"
)

// traceNode is an internal representation that serializes to the same JSON
// format as belay's trace.Node. We avoid importing belay directly since
// they are separate projects.
type traceNode struct {
	ID            string       `json:"id"`
	NodeType      string       `json:"node_type"`
	AgentName     string       `json:"agent_name"`
	Model         string       `json:"model,omitempty"`
	DurationMS    int64        `json:"duration_ms"`
	CostUSD       float64      `json:"cost_usd"`
	InputTokens   int          `json:"input_tokens"`
	OutputTokens  int          `json:"output_tokens"`
	ContextWindow int          `json:"context_window,omitempty"`
	Children      []*traceNode `json:"children,omitempty"`
}

// Provider implements belaykit.ObservabilityProvider and writes trace trees
// as JSON files that belay can read.
type Provider struct {
	dir           string           // output directory (default ".belay/traces")
	pricing       belaykit.ModelPricing // model pricing for cost estimation
	contextWindow int              // context window size in tokens

	mu           sync.Mutex
	root         *traceNode            // in-progress trace tree
	currentPhase *traceNode            // current phase being built
	startTime    time.Time             // trace start time
	phaseStart   time.Time             // current phase start time
	toolStart    map[string]time.Time  // toolID -> start time
	toolNodes    map[string]*traceNode // toolID -> in-progress tool node
	traceID      string                // current trace ID
	inputTokens  int                   // accumulated input token estimate
	outputTokens int                   // accumulated output token estimate
}

// Option configures a Provider.
type Option func(*Provider)

// WithDir sets the output directory for trace files.
func WithDir(dir string) Option {
	return func(p *Provider) {
		p.dir = dir
	}
}

// WithPricing sets the model pricing used for cost estimation.
func WithPricing(pricing belaykit.ModelPricing) Option {
	return func(p *Provider) {
		p.pricing = pricing
	}
}

// WithContextWindow sets the context window size in tokens for utilization tracking.
func WithContextWindow(tokens int) Option {
	return func(p *Provider) {
		p.contextWindow = tokens
	}
}

// NewProvider creates a new belay trace provider.
func NewProvider(opts ...Option) *Provider {
	p := &Provider{
		dir:       ".belay/traces",
		toolStart: make(map[string]time.Time),
		toolNodes: make(map[string]*traceNode),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func shortID() string {
	return uuid.New().String()[:8]
}

// StartSession begins a new observability session.
func (p *Provider) StartSession(metadata map[string]any) string {
	return shortID()
}

// StartTrace begins a new trace.
func (p *Provider) StartTrace(config belaykit.TraceConfig, inputs map[string]any) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := shortID()
	p.traceID = id
	p.startTime = time.Now()
	p.root = &traceNode{
		ID:        id,
		NodeType:  "trace",
		AgentName: config.Name,
	}
	p.currentPhase = nil
	p.toolStart = make(map[string]time.Time)
	p.toolNodes = make(map[string]*traceNode)
	p.inputTokens = 0
	p.outputTokens = 0
	return id
}

// EndTrace completes a trace and writes the JSON file.
func (p *Provider) EndTrace(traceID string, outputs map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.root == nil {
		return
	}

	// Finalize the current phase if one is open
	p.finalizeCurrentPhase()

	p.root.DurationMS = time.Since(p.startTime).Milliseconds()
	p.writeTrace(traceID)
	p.root = nil
	p.currentPhase = nil
}

// RecordCompletion records an LLM completion, updating the current phase
// with cost, model, and duration information.
func (p *Provider) RecordCompletion(record belaykit.CompletionRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.root == nil {
		return
	}

	// If no phase exists, create a default one
	if p.currentPhase == nil {
		p.currentPhase = &traceNode{
			ID:        shortID(),
			NodeType:  "phase",
			AgentName: "default",
		}
		p.phaseStart = time.Now().Add(-time.Duration(record.DurationMS) * time.Millisecond)
		p.root.Children = append(p.root.Children, p.currentPhase)
	}

	p.currentPhase.Model = record.Model
	p.currentPhase.DurationMS += record.DurationMS

	// Use token counts from record if available, otherwise use accumulated estimates
	inTok := record.InputTokens
	outTok := record.OutputTokens
	if inTok == 0 && outTok == 0 {
		inTok = p.inputTokens
		outTok = p.outputTokens
	}
	p.currentPhase.InputTokens += inTok
	p.currentPhase.OutputTokens += outTok

	// Use cost from record if available, otherwise estimate from pricing
	cost := record.CostUSD
	if cost == 0 && (inTok > 0 || outTok > 0) {
		cost = p.pricing.Cost(inTok, outTok)
	}
	p.currentPhase.CostUSD += cost
}

// EventHandler returns an EventHandler function that captures tool-level
// events for the trace tree. Compose it alongside the logger:
//
//	handler := func(e belaykit.Event) {
//	    logger(e)
//	    bp.EventHandler()(e)
//	}
func (p *Provider) EventHandler() belaykit.EventHandler {
	return func(e belaykit.Event) {
		p.mu.Lock()
		defer p.mu.Unlock()

		if p.root == nil {
			return
		}

		// Accumulate token estimates from events (mirrors logger's classifyEventTokens)
		switch e.Type {
		case belaykit.EventAssistant:
			p.outputTokens += belaykit.EstimateTokens(e.Text)
		case belaykit.EventToolUse:
			p.outputTokens += belaykit.EstimateTokens(e.ToolName) + belaykit.EstimateTokens(string(e.ToolInput))
		case belaykit.EventToolResult:
			p.inputTokens += belaykit.EstimateTokens(e.Text)
		case belaykit.EventSystem:
			p.inputTokens += belaykit.EstimateTokens(e.Subtype) + belaykit.EstimateTokens(e.SessionID)
		case belaykit.EventResult, belaykit.EventResultError:
			p.inputTokens += belaykit.EstimateTokens(e.Text)
		}

		switch e.Type {
		case belaykit.EventPhase:
			p.handlePhase(e)
		case belaykit.EventToolUse:
			p.handleToolUse(e)
		case belaykit.EventToolResult:
			p.handleToolResult(e)
		}
	}
}

func (p *Provider) handlePhase(e belaykit.Event) {
	// Finalize the previous phase
	p.finalizeCurrentPhase()

	// Add a marker node for the phase transition
	marker := &traceNode{
		ID:        shortID(),
		NodeType:  "marker",
		AgentName: fmt.Sprintf("→ %s", e.PhaseName),
	}
	p.root.Children = append(p.root.Children, marker)

	// Start new phase
	p.currentPhase = &traceNode{
		ID:        shortID(),
		NodeType:  "phase",
		AgentName: e.PhaseName,
	}
	p.phaseStart = time.Now()
	p.root.Children = append(p.root.Children, p.currentPhase)
}

func (p *Provider) handleToolUse(e belaykit.Event) {
	if p.currentPhase == nil {
		// Create a default phase if tools are used without an explicit phase
		p.currentPhase = &traceNode{
			ID:        shortID(),
			NodeType:  "phase",
			AgentName: "default",
		}
		p.phaseStart = time.Now()
		p.root.Children = append(p.root.Children, p.currentPhase)
	}

	node := &traceNode{
		ID:            shortID(),
		NodeType:      "tool_call",
		AgentName:     e.ToolName,
		InputTokens:   p.inputTokens,
		OutputTokens:  p.outputTokens,
		ContextWindow: p.contextWindow,
	}
	p.toolStart[e.ToolID] = time.Now()
	p.toolNodes[e.ToolID] = node
	p.currentPhase.Children = append(p.currentPhase.Children, node)
}

func (p *Provider) handleToolResult(e belaykit.Event) {
	node, ok := p.toolNodes[e.ToolID]
	if !ok {
		return
	}
	if start, ok := p.toolStart[e.ToolID]; ok {
		node.DurationMS = time.Since(start).Milliseconds()
		delete(p.toolStart, e.ToolID)
	}
	delete(p.toolNodes, e.ToolID)
}

func (p *Provider) finalizeCurrentPhase() {
	if p.currentPhase == nil {
		return
	}
	if p.currentPhase.DurationMS == 0 && !p.phaseStart.IsZero() {
		p.currentPhase.DurationMS = time.Since(p.phaseStart).Milliseconds()
	}
}

func (p *Provider) writeTrace(traceID string) {
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return
	}

	data, err := json.MarshalIndent(p.root, "", "  ")
	if err != nil {
		return
	}

	path := filepath.Join(p.dir, traceID+".json")
	_ = os.WriteFile(path, data, 0o644)
}

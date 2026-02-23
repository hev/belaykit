// Package freeplay provides an ObservabilityProvider backed by Freeplay
// (https://freeplay.ai).
//
// Usage:
//
//	fp := freeplay.NewClient(apiKey, projectID, freeplay.WithEnvironment("production"))
//	client := claude.NewClient(claude.WithObservability(freeplay.NewProvider(fp)))
//
//	// Optional: group runs in a session
//	fp.StartSession(map[string]any{"name": "my-pipeline"})
//
//	// Optional: wrap in a trace
//	tid := provider.StartTrace(belaykit.TraceConfig{Name: "extract"}, inputs)
//	result, _ := client.Run(ctx, prompt, belaykit.WithTraceID(tid))
//	provider.EndTrace(tid, map[string]any{"fields": 42})
package freeplay

import (
	"time"

	"belaykit"

	fp "github.com/hev/freeplay-go"
)

// Provider implements belaykit.ObservabilityProvider using the Freeplay API.
type Provider struct {
	client *fp.Client
}

// NewProvider creates a new Freeplay observability provider.
// The freeplay.Client should already be configured with API key, project ID,
// and any options (environment, error handler, etc.).
func NewProvider(client *fp.Client) *Provider {
	return &Provider{client: client}
}

// StartSession begins a new Freeplay session.
func (p *Provider) StartSession(metadata map[string]any) string {
	return p.client.StartSession(metadata)
}

// StartTrace begins a new Freeplay trace.
func (p *Provider) StartTrace(config belaykit.TraceConfig, inputs map[string]any) string {
	return p.client.StartTrace(fp.TraceConfig{
		AgentName:      config.Name,
		DisplayName:    config.DisplayName,
		CustomMetadata: config.Metadata,
	}, inputs)
}

// EndTrace completes a Freeplay trace.
func (p *Provider) EndTrace(traceID string, outputs map[string]any) {
	p.client.EndTrace(traceID, outputs)
}

// RecordCompletion records an LLM completion in Freeplay.
func (p *Provider) RecordCompletion(record belaykit.CompletionRecord) {
	now := time.Now()
	start := now.Add(-time.Duration(record.DurationMS) * time.Millisecond)

	p.client.RecordCompletion(fp.CompletionData{
		TraceID:    record.TraceID,
		Prompt:     record.Prompt,
		Response:   record.Response,
		Model:      record.Model,
		Provider:   "anthropic",
		StartTime:  start,
		EndTime:    now,
		CostUSD:    record.CostUSD,
		DurationMS: record.DurationMS,
		NumTurns:   record.NumTurns,
	})
}

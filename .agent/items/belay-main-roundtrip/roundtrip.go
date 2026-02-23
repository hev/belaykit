//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	trace := map[string]any{
		"id": "roundtrip1", "node_type": "trace", "agent_name": "roundtrip-test",
		"duration_ms": 5000, "cost_usd": 0.13, "input_tokens": 0, "output_tokens": 0,
		"children": []map[string]any{
			{"id": "m1", "node_type": "marker", "agent_name": "phase-0", "duration_ms": 0, "cost_usd": 0, "input_tokens": 0, "output_tokens": 0},
			{
				"id": "p1", "node_type": "phase", "agent_name": "discovery",
				"model": "claude-opus-4-6", "duration_ms": 3000, "cost_usd": 0.08,
				"input_tokens": 50000, "output_tokens": 2000, "context_window": 200000,
				"children": []map[string]any{
					{"id": "t1", "node_type": "tool_call", "agent_name": "bash", "model": "claude-opus-4-6", "duration_ms": 1500, "cost_usd": 0.04, "input_tokens": 25000, "output_tokens": 1000, "context_window": 200000},
					{"id": "t2", "node_type": "tool_call", "agent_name": "bash", "model": "claude-opus-4-6", "duration_ms": 1500, "cost_usd": 0.04, "input_tokens": 50000, "output_tokens": 1000, "context_window": 200000},
				},
			},
			{"id": "m2", "node_type": "marker", "agent_name": "phase-1", "duration_ms": 0, "cost_usd": 0, "input_tokens": 0, "output_tokens": 0},
			{
				"id": "p2", "node_type": "phase", "agent_name": "extraction",
				"model": "claude-haiku-4-5-20251001", "duration_ms": 2000, "cost_usd": 0.05,
				"input_tokens": 30000, "output_tokens": 1500, "context_window": 200000,
				"children": []map[string]any{
					{"id": "t3", "node_type": "tool_call", "agent_name": "bash", "model": "claude-haiku-4-5-20251001", "duration_ms": 2000, "cost_usd": 0.05, "input_tokens": 30000, "output_tokens": 1500, "context_window": 200000},
				},
			},
		},
	}

	dir, _ := os.MkdirTemp("", "belay-roundtrip-*")
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "test-trace.json")
	data, _ := json.MarshalIndent(trace, "", "  ")
	os.WriteFile(path, data, 0644)

	fmt.Printf("Trace file: %s\n", path)

	cmd := exec.Command("/tmp/belay-test-binary", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "belay failed: %v\n%s\n", err, out)
		os.Exit(1)
	}

	fmt.Printf("Belay output:\n%s\n", out)

	cmd2 := exec.Command("/tmp/belay-test-binary")
	out2, err2 := cmd2.CombinedOutput()
	if err2 != nil {
		fmt.Fprintf(os.Stderr, "belay (no args) failed: %v\n%s\n", err2, out2)
		os.Exit(1)
	}

	fmt.Println("SampleTree mode also works.")
}

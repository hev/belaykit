package claude

import "testing"

func TestContextWindowForModel(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"opus", 200_000},
		{"claude-opus-4-6", 200_000},
		{"sonnet", 200_000},
		{"claude-sonnet-4-5-20250929", 200_000},
		{"haiku", 200_000},
		{"claude-haiku-4-5-20251001", 200_000},
		{"unknown-model", 200_000},
		{"", 200_000},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := ContextWindowForModel(tt.model)
			if got != tt.want {
				t.Errorf("ContextWindowForModel(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"short", "hi", 1},
		{"exact multiple", "abcdefgh", 2},
		{"not exact", "abcde", 2},
		{"one char", "a", 1},
		{"four chars", "abcd", 1},
		{"five chars", "abcde", 2},
		{"typical sentence", "Hello, world! This is a test.", 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateTokens(tt.input)
			if got != tt.want {
				t.Errorf("EstimateTokens(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestPricingForModel(t *testing.T) {
	tests := []struct {
		model     string
		wantInput float64
		wantOut   float64
	}{
		{"opus", 5, 25},
		{"claude-opus-4-6", 5, 25},
		{"sonnet", 3, 15},
		{"claude-sonnet-4-5-20250929", 3, 15},
		{"haiku", 1, 5},
		{"claude-haiku-4-5-20251001", 1, 5},
		{"unknown", 5, 25}, // defaults to opus pricing
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			p := PricingForModel(tt.model)
			if p.InputPerMTok != tt.wantInput {
				t.Errorf("InputPerMTok = %f, want %f", p.InputPerMTok, tt.wantInput)
			}
			if p.OutputPerMTok != tt.wantOut {
				t.Errorf("OutputPerMTok = %f, want %f", p.OutputPerMTok, tt.wantOut)
			}
		})
	}
}

func TestModelPricingCost(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		in     int
		out    int
		want   float64
		approx bool
	}{
		// opus: 1M input * $5/M + 0 output = $5.00
		{"opus input only", "opus", 1_000_000, 0, 5.0, false},
		// opus: 0 input + 1M output * $25/M = $25.00
		{"opus output only", "opus", 0, 1_000_000, 25.0, false},
		// haiku: 1000 input * $1/M + 1000 output * $5/M = $0.001 + $0.005 = $0.006
		{"haiku mixed", "haiku", 1000, 1000, 0.006, false},
		// zero tokens = zero cost
		{"zero", "opus", 0, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PricingForModel(tt.model)
			got := p.Cost(tt.in, tt.out)
			if got != tt.want {
				t.Errorf("Cost(%d, %d) = %f, want %f", tt.in, tt.out, got, tt.want)
			}
		})
	}
}

package rack

import "testing"

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

func TestModelPricingCost(t *testing.T) {
	tests := []struct {
		name    string
		pricing ModelPricing
		in      int
		out     int
		want    float64
	}{
		{"input only", ModelPricing{InputPerMTok: 5, OutputPerMTok: 25}, 1_000_000, 0, 5.0},
		{"output only", ModelPricing{InputPerMTok: 5, OutputPerMTok: 25}, 0, 1_000_000, 25.0},
		{"mixed", ModelPricing{InputPerMTok: 1, OutputPerMTok: 5}, 1000, 1000, 0.006},
		{"zero", ModelPricing{InputPerMTok: 5, OutputPerMTok: 25}, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pricing.Cost(tt.in, tt.out)
			if got != tt.want {
				t.Errorf("Cost(%d, %d) = %f, want %f", tt.in, tt.out, got, tt.want)
			}
		})
	}
}

package belaykit

// ModelPricing holds per-million-token costs for a model.
type ModelPricing struct {
	InputPerMTok  float64 // USD per million input tokens
	OutputPerMTok float64 // USD per million output tokens
}

// Cost calculates the USD cost for the given input and output token counts.
func (p ModelPricing) Cost(inputTokens, outputTokens int) float64 {
	return float64(inputTokens)/1_000_000*p.InputPerMTok +
		float64(outputTokens)/1_000_000*p.OutputPerMTok
}

// EstimateTokens returns a rough token count for the given text.
// Uses ~4 characters per token, which is a standard heuristic for
// Claude's byte-pair encoding tokenizer on mixed English/code text.
func EstimateTokens(text string) int {
	n := len(text)
	if n == 0 {
		return 0
	}
	return (n + 3) / 4 // round up
}

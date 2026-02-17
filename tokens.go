package claude

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

// PricingForModel returns the token pricing for a given model name or alias.
// Returns Opus 4.6 pricing as the default for unknown models.
func PricingForModel(model string) ModelPricing {
	switch model {
	case "opus", "claude-opus-4-6":
		return ModelPricing{InputPerMTok: 5, OutputPerMTok: 25}
	case "sonnet", "claude-sonnet-4-5-20250929":
		return ModelPricing{InputPerMTok: 3, OutputPerMTok: 15}
	case "haiku", "claude-haiku-4-5-20251001":
		return ModelPricing{InputPerMTok: 1, OutputPerMTok: 5}
	default:
		return ModelPricing{InputPerMTok: 5, OutputPerMTok: 25}
	}
}

// ContextWindowForModel returns the context window size in tokens for a given
// model name or alias. Returns 200,000 as the default for unknown models.
func ContextWindowForModel(model string) int {
	switch model {
	case "opus", "claude-opus-4-6":
		return 200_000
	case "sonnet", "claude-sonnet-4-5-20250929":
		return 200_000
	case "haiku", "claude-haiku-4-5-20251001":
		return 200_000
	default:
		return 200_000
	}
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

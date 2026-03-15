package pi

import "belaykit"

// PricingForModel returns the token pricing for a given model ID.
// Pi supports many providers; this covers common models. Returns
// Claude Sonnet pricing as the default for unknown models.
func PricingForModel(model string) belaykit.ModelPricing {
	switch model {
	// Anthropic
	case "claude-opus-4-6":
		return belaykit.ModelPricing{InputPerMTok: 5, OutputPerMTok: 25}
	case "claude-sonnet-4-20250514", "claude-sonnet-4-5-20250929":
		return belaykit.ModelPricing{InputPerMTok: 3, OutputPerMTok: 15}
	case "claude-haiku-4-5-20251001":
		return belaykit.ModelPricing{InputPerMTok: 1, OutputPerMTok: 5}
	// OpenAI
	case "gpt-4o":
		return belaykit.ModelPricing{InputPerMTok: 2.5, OutputPerMTok: 10}
	case "gpt-4o-mini":
		return belaykit.ModelPricing{InputPerMTok: 0.15, OutputPerMTok: 0.6}
	case "o3":
		return belaykit.ModelPricing{InputPerMTok: 10, OutputPerMTok: 40}
	// Google
	case "gemini-2.5-pro":
		return belaykit.ModelPricing{InputPerMTok: 1.25, OutputPerMTok: 10}
	case "gemini-2.5-flash":
		return belaykit.ModelPricing{InputPerMTok: 0.15, OutputPerMTok: 0.6}
	default:
		return belaykit.ModelPricing{InputPerMTok: 3, OutputPerMTok: 15}
	}
}

// ContextWindowForModel returns the context window size in tokens for a given
// model ID. Returns 200,000 as the default for unknown models.
func ContextWindowForModel(model string) int {
	switch model {
	case "gpt-4o", "gpt-4o-mini":
		return 128_000
	case "o3":
		return 200_000
	case "gemini-2.5-pro", "gemini-2.5-flash":
		return 1_000_000
	default:
		return 200_000
	}
}

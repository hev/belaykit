package claude

import "belaykit"

// PricingForModel returns the token pricing for a given model name or alias.
// Returns Opus 4.6 pricing as the default for unknown models.
func PricingForModel(model string) belaykit.ModelPricing {
	switch model {
	case "opus", "claude-opus-4-6":
		return belaykit.ModelPricing{InputPerMTok: 5, OutputPerMTok: 25}
	case "sonnet", "claude-sonnet-4-5-20250929":
		return belaykit.ModelPricing{InputPerMTok: 3, OutputPerMTok: 15}
	case "haiku", "claude-haiku-4-5-20251001":
		return belaykit.ModelPricing{InputPerMTok: 1, OutputPerMTok: 5}
	default:
		return belaykit.ModelPricing{InputPerMTok: 5, OutputPerMTok: 25}
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

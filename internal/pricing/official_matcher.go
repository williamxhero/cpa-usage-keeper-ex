package pricing

import (
	"strings"

	"cpa-usage-keeper/internal/entities"
)

type OfficialPriceSegment string

const (
	OfficialPricePrompt     OfficialPriceSegment = "prompt"
	OfficialPriceCompletion OfficialPriceSegment = "completion"
	OfficialPriceCacheRead  OfficialPriceSegment = "cache_read"
	OfficialPriceCacheWrite OfficialPriceSegment = "cache_write"
)

type OfficialMatchStatus string

const (
	OfficialMatchAvailable   OfficialMatchStatus = "available"
	OfficialMatchUnavailable OfficialMatchStatus = "unavailable"
	OfficialMatchAmbiguous   OfficialMatchStatus = "ambiguous"
)

// OfficialMatch reports deterministic catalog resolution. Price is populated for
// a unique identity match even when a segment required by this request is absent.
type OfficialMatch struct {
	Price           *entities.OfficialModelPrice
	Status          OfficialMatchStatus
	Reason          string
	MissingSegments []OfficialPriceSegment
}

// MatchOfficialPrice tries identity-preserving tiers in order: exact canonical
// ID, case-folded canonical ID, the existing CPA prefix-stripping convention,
// then explicit aliases. It never guesses from a short name or substring.
func MatchOfficialPrice(prices []entities.OfficialModelPrice, model string, required []OfficialPriceSegment) OfficialMatch {
	return MatchOfficialPriceWithPrefixes(prices, model, required, nil)
}

// MatchOfficialPriceWithPrefixes accepts only explicitly registered CPA prefixes.
// Removing one prefix preserves the entire remaining identity, including an
// OpenRouter provider/model namespace or a fine-tuned/versioned model ID.
func MatchOfficialPriceWithPrefixes(prices []entities.OfficialModelPrice, model string, required []OfficialPriceSegment, prefixes []string) OfficialMatch {
	model = strings.TrimSpace(model)
	if model == "" {
		return OfficialMatch{Status: OfficialMatchUnavailable, Reason: "no_match"}
	}
	// Canonical IDs are case-insensitive for lookup. Treat duplicate folded IDs
	// as a conflict even when one happens to match the request's original case;
	// otherwise the result would depend on how the caller capitalized the ID.
	if candidates := filterOfficialPrices(prices, func(price entities.OfficialModelPrice) bool {
		return strings.EqualFold(price.CanonicalModelID, model)
	}); len(candidates) > 1 {
		return OfficialMatch{Status: OfficialMatchAmbiguous, Reason: "ambiguous_match"}
	}

	tiers := [][]entities.OfficialModelPrice{
		filterOfficialPrices(prices, func(price entities.OfficialModelPrice) bool { return price.CanonicalModelID == model }),
		filterOfficialPrices(prices, func(price entities.OfficialModelPrice) bool { return strings.EqualFold(price.CanonicalModelID, model) }),
	}
	stripped := officialPrefixCandidates(model, prefixes)
	if len(stripped) > 0 {
		tiers = append(tiers, filterOfficialPrices(prices, func(price entities.OfficialModelPrice) bool {
			for _, candidate := range stripped {
				if strings.EqualFold(price.CanonicalModelID, candidate) {
					return true
				}
			}
			return false
		}))
	}
	tiers = append(tiers,
		filterOfficialAliases(prices, model, false),
		filterOfficialAliases(prices, model, true),
	)

	for _, candidates := range tiers {
		if len(candidates) == 0 {
			continue
		}
		if len(candidates) != 1 {
			return OfficialMatch{Status: OfficialMatchAmbiguous, Reason: "ambiguous_match"}
		}
		price := candidates[0]
		missing := missingOfficialPriceSegments(price, required)
		result := OfficialMatch{Price: &price, Status: OfficialMatchAvailable}
		if len(missing) > 0 {
			result.Status = OfficialMatchUnavailable
			result.Reason = "missing_segments"
			result.MissingSegments = missing
		}
		return result
	}
	return OfficialMatch{Status: OfficialMatchUnavailable, Reason: "no_match"}
}

func officialPrefixCandidates(model string, prefixes []string) []string {
	if prefixes == nil {
		stripped := stripOfficialPricingModelPrefix(model)
		if stripped != model {
			return []string{stripped}
		}
		return nil
	}
	candidates := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(model), strings.ToLower(prefix)+"/") || strings.HasPrefix(strings.ToLower(model), strings.ToLower(prefix)+":") {
			candidates = append(candidates, strings.TrimSpace(model[len(prefix)+1:]))
		}
	}
	return candidates
}

func filterOfficialPrices(prices []entities.OfficialModelPrice, matches func(entities.OfficialModelPrice) bool) []entities.OfficialModelPrice {
	result := make([]entities.OfficialModelPrice, 0, 1)
	for _, price := range prices {
		if matches(price) {
			result = append(result, price)
		}
	}
	return result
}

func filterOfficialAliases(prices []entities.OfficialModelPrice, model string, fold bool) []entities.OfficialModelPrice {
	result := make([]entities.OfficialModelPrice, 0, 1)
	for _, price := range prices {
		matched := false
		for _, alias := range price.Aliases {
			if (fold && strings.EqualFold(alias.Alias, model)) || (!fold && alias.Alias == model) {
				matched = true
				break
			}
		}
		if matched {
			result = append(result, price)
		}
	}
	return result
}

func missingOfficialPriceSegments(price entities.OfficialModelPrice, required []OfficialPriceSegment) []OfficialPriceSegment {
	missing := make([]OfficialPriceSegment, 0)
	for _, segment := range required {
		var value *float64
		switch segment {
		case OfficialPricePrompt:
			value = price.PromptPricePer1M
		case OfficialPriceCompletion:
			value = price.CompletionPricePer1M
		case OfficialPriceCacheRead:
			value = price.CacheReadPricePer1M
		case OfficialPriceCacheWrite:
			value = price.CacheWritePricePer1M
		default:
			continue
		}
		if value == nil {
			missing = append(missing, segment)
		}
	}
	return missing
}

// Keep this convention local to the new official matcher so the legacy matcher
// and its prioritization behavior remain unchanged.
func stripOfficialPricingModelPrefix(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ""
	}
	for index := 0; index+len("ft:") <= len(trimmed); index++ {
		if (index == 0 || trimmed[index-1] == '/' || trimmed[index-1] == ':') && strings.EqualFold(trimmed[index:index+len("ft:")], "ft:") {
			return trimmed[index:]
		}
	}
	for end := len(trimmed); end > 0; {
		index := strings.LastIndexAny(trimmed[:end], "/:")
		if index < 0 || index == len(trimmed)-1 {
			return trimmed
		}
		if trimmed[index] == ':' && isOfficialModelVersion(trimmed[index+1:end]) {
			end = index
			continue
		}
		prefix := strings.TrimSpace(trimmed[:index])
		// A provider/model ID is already a complete identity. Do not split it
		// into a bare model and accidentally select another provider's price.
		// CPA-specific prefixes remain supported for namespaces that are not a
		// known provider namespace; explicit aliases are preferred when unsure.
		if isOfficialProviderNamespace(prefix) {
			return trimmed
		}
		return strings.TrimSpace(trimmed[index+1:])
	}
	return trimmed
}

func isOfficialProviderNamespace(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if slash := strings.LastIndexByte(value, '/'); slash >= 0 {
		value = value[slash+1:]
	}
	switch value {
	case "provider", "openai", "azure", "anthropic", "google", "google-vertex", "vertex", "amazon-bedrock", "bedrock", "deepseek", "mistral", "cohere", "xai", "x-ai", "qwen", "alibaba", "alibaba-cn", "zai", "zhipuai", "moonshot", "moonshotai", "minimax", "llama", "meta", "openrouter":
		return true
	default:
		return false
	}
}

func isOfficialModelVersion(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

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
	// These are the only built-in CPA wrappers; callers with other registered
	// wrappers must pass them explicitly to avoid provider/model guessing.
	return MatchOfficialPriceWithPrefixes(prices, model, required, []string{"custom", "cpa"})
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
	}); officialCandidateStatus(candidates) == OfficialMatchAmbiguous {
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
	tiers = append(tiers, filterOfficialAliases(prices, model))

	for _, candidates := range tiers {
		price, ok, status := selectOfficialCandidate(candidates)
		if status == OfficialMatchAmbiguous {
			return OfficialMatch{Status: OfficialMatchAmbiguous, Reason: "ambiguous_match"}
		}
		if !ok {
			continue
		}
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

// OpenRouter records are fallback facts. A primary official record wins within
// an identity tier; conflicting primary records remain ambiguous.
func officialCandidateStatus(candidates []entities.OfficialModelPrice) OfficialMatchStatus {
	_, ok, status := selectOfficialCandidate(candidates)
	if status == OfficialMatchAmbiguous {
		return status
	}
	if ok {
		return OfficialMatchAvailable
	}
	return OfficialMatchUnavailable
}

func selectOfficialCandidate(candidates []entities.OfficialModelPrice) (entities.OfficialModelPrice, bool, OfficialMatchStatus) {
	if len(candidates) == 0 {
		return entities.OfficialModelPrice{}, false, ""
	}
	primary := make([]entities.OfficialModelPrice, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Source != entities.OfficialModelPriceSourceOpenRouter {
			primary = append(primary, candidate)
		}
	}
	if len(primary) > 1 {
		return entities.OfficialModelPrice{}, false, OfficialMatchAmbiguous
	}
	if len(primary) == 1 {
		return primary[0], true, OfficialMatchAvailable
	}
	if len(candidates) == 1 {
		return candidates[0], true, OfficialMatchAvailable
	}
	return entities.OfficialModelPrice{}, false, OfficialMatchAmbiguous
}

func filterOfficialAliases(prices []entities.OfficialModelPrice, model string) []entities.OfficialModelPrice {
	result := make([]entities.OfficialModelPrice, 0, 1)
	for _, price := range prices {
		matched := false
		for _, alias := range price.Aliases {
			if strings.EqualFold(alias.Alias, model) {
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

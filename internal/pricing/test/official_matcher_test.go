package test

import (
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
)

func TestMatchOfficialPriceDeterministicIdentityTiers(t *testing.T) {
	base := entities.OfficialModelPrice{CanonicalModelID: "claude-sonnet", PromptPricePer1M: floatPointer(3), CompletionPricePer1M: floatPointer(15)}
	base.Aliases = []entities.OfficialModelPriceAlias{{Alias: "claude-sonnet-latest"}}
	prices := []entities.OfficialModelPrice{base}

	for _, model := range []string{"claude-sonnet", "CLAUDE-SONNET", "custom/claude-sonnet", "claude-sonnet-latest"} {
		got := pricing.MatchOfficialPrice(prices, model, []pricing.OfficialPriceSegment{pricing.OfficialPricePrompt, pricing.OfficialPriceCompletion})
		if got.Status != pricing.OfficialMatchAvailable || got.Price == nil || got.Price.CanonicalModelID != base.CanonicalModelID {
			t.Errorf("model %q did not resolve deterministically: %#v", model, got)
		}
	}
}

func TestMatchOfficialPriceRejectsGuessingAndAmbiguity(t *testing.T) {
	prices := []entities.OfficialModelPrice{
		{CanonicalModelID: "exact", PromptPricePer1M: floatPointer(1)},
		{CanonicalModelID: "exact", PromptPricePer1M: floatPointer(2)},
		{CanonicalModelID: "canonical", Aliases: []entities.OfficialModelPriceAlias{{Alias: "shared"}}},
		{CanonicalModelID: "alias-target", Aliases: []entities.OfficialModelPriceAlias{{Alias: "shared"}}},
	}
	for _, model := range []string{"exact", "shared"} {
		got := pricing.MatchOfficialPrice(prices, model, nil)
		if got.Status != pricing.OfficialMatchAmbiguous {
			t.Errorf("model %q status = %q, want ambiguous: %#v", model, got.Status, got)
		}
	}
	for _, model := range []string{"model-1", "some-prefix/canonical-suffix", "absent"} {
		got := pricing.MatchOfficialPrice(prices, model, nil)
		if got.Status != pricing.OfficialMatchUnavailable || got.Reason != "no_match" {
			t.Errorf("guessed model %q should be unavailable: %#v", model, got)
		}
	}
}

func floatPointer(value float64) *float64 { return &value }

func TestMatchOfficialPriceRejectsCaseFoldedCanonicalConflict(t *testing.T) {
	prices := []entities.OfficialModelPrice{
		{CanonicalModelID: "model", PromptPricePer1M: floatPointer(1)},
		{CanonicalModelID: "MODEL", PromptPricePer1M: floatPointer(2)},
	}
	got := pricing.MatchOfficialPrice(prices, "model", nil)
	if got.Status != pricing.OfficialMatchAmbiguous || got.Reason != "ambiguous_match" {
		t.Fatalf("case-folded canonical conflict = %#v, want ambiguous", got)
	}
}

func TestMatchOfficialPriceDoesNotSplitProviderModelID(t *testing.T) {
	prices := []entities.OfficialModelPrice{{CanonicalModelID: "model", PromptPricePer1M: floatPointer(1)}}
	got := pricing.MatchOfficialPrice(prices, "provider/model", nil)
	if got.Status != pricing.OfficialMatchUnavailable || got.Reason != "no_match" {
		t.Fatalf("provider/model was guessed as bare model: %#v", got)
	}
}

func TestMatchOfficialPriceRejectsCaseFoldedAliasConflict(t *testing.T) {
	prices := []entities.OfficialModelPrice{
		{CanonicalModelID: "one", Aliases: []entities.OfficialModelPriceAlias{{Alias: "shared"}}},
		{CanonicalModelID: "two", Aliases: []entities.OfficialModelPriceAlias{{Alias: "SHARED"}}},
	}
	for _, model := range []string{"shared", "SHARED"} {
		got := pricing.MatchOfficialPrice(prices, model, nil)
		if got.Status != pricing.OfficialMatchAmbiguous {
			t.Fatalf("case-folded alias conflict was selected for %q: %#v", model, got)
		}
	}
}

func TestMatchOfficialPriceWithPrefixesRequiresRegisteredWrapper(t *testing.T) {
	prices := []entities.OfficialModelPrice{{CanonicalModelID: "model"}}
	got := pricing.MatchOfficialPriceWithPrefixes(prices, "unknown/model", nil, nil)
	if got.Status != pricing.OfficialMatchUnavailable {
		t.Fatalf("unregistered prefix was stripped: %#v", got)
	}
	got = pricing.MatchOfficialPriceWithPrefixes(prices, "unknown/model", nil, []string{"unknown"})
	if got.Status != pricing.OfficialMatchAvailable {
		t.Fatalf("registered prefix was not stripped: %#v", got)
	}
}

func TestMatchOfficialPricePreservesIdentitySeparators(t *testing.T) {
	for _, pair := range [][2]string{
		{"provider/model", "providermodel"},
		{"ab/c", "a/bc"},
		{"model:1", "model1"},
		{"ft:model:org:id", "ftmodelorgid"},
		{"model-name", "modelname"},
		{"provider/model", "custom/providermodel"},
	} {
		price := entities.OfficialModelPrice{CanonicalModelID: pair[0], PromptPricePer1M: floatPointer(1)}
		got := pricing.MatchOfficialPrice([]entities.OfficialModelPrice{price}, pair[1], nil)
		if got.Status != pricing.OfficialMatchUnavailable || got.Reason != "no_match" {
			t.Errorf("different identity %q matched %q without an alias: %#v", pair[1], pair[0], got)
		}
		price.Aliases = []entities.OfficialModelPriceAlias{{Alias: pair[1]}}
		got = pricing.MatchOfficialPrice([]entities.OfficialModelPrice{price}, pair[1], nil)
		if got.Status != pricing.OfficialMatchAvailable {
			t.Errorf("explicit alias %q failed to match %q: %#v", pair[1], pair[0], got)
		}
	}
}

func TestMatchOfficialPriceChecksOnlyRequiredSegments(t *testing.T) {
	price := entities.OfficialModelPrice{CanonicalModelID: "model", PromptPricePer1M: floatPointer(1), CompletionPricePer1M: floatPointer(2)}
	got := pricing.MatchOfficialPrice([]entities.OfficialModelPrice{price}, "model", []pricing.OfficialPriceSegment{pricing.OfficialPricePrompt, pricing.OfficialPriceCompletion})
	if got.Status != pricing.OfficialMatchAvailable {
		t.Fatalf("unused missing cache segments blocked price: %#v", got)
	}
	got = pricing.MatchOfficialPrice([]entities.OfficialModelPrice{price}, "model", []pricing.OfficialPriceSegment{pricing.OfficialPricePrompt, pricing.OfficialPriceCacheRead})
	if got.Status != pricing.OfficialMatchUnavailable || got.Reason != "missing_segments" || len(got.MissingSegments) != 1 || got.MissingSegments[0] != pricing.OfficialPriceCacheRead {
		t.Fatalf("required cache-read segment should be unavailable: %#v", got)
	}
}

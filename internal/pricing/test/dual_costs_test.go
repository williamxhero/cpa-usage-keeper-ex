package test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
)

func TestDualCostsLegacyIndependentReferenceAndSafeExplanation(t *testing.T) {
	resolver := compileResolver(t, pricing.ModelConfig{
		Pricing: testPricingWithPromptAndMultiplier("m", 10, .5),
		Rules:   []pricing.RuleConfig{{Key: "service_tier", Value: "priority", Multiplier: 2}, {Key: "auth_index", Value: "synthetic-secret-index", Multiplier: 3}},
	})
	result := resolver.Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "m", ServiceTier: "priority", AuthIndex: "synthetic-secret-index"}, helper.UsageTokenCostInput{InputTokens: 1_000_000}))
	if result.Cost.TotalCostUSD != 30 || !result.ReferenceAvailable || result.ReferenceCost.TotalCostUSD != 10 {
		t.Fatalf("legacy/configured reference must be independent: %+v", result)
	}
	selection := resolver.Selection(result)
	if selection == nil {
		t.Fatal("legacy explanation missing")
	}
	body, err := json.Marshal(selection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "synthetic-secret-index") {
		t.Fatalf("rule condition leaked: %s", body)
	}
}

func TestDualCostsKnownZeroUnavailablePartialAndFiniteScaling(t *testing.T) {
	resolver := compileResolver(t, pricing.ModelConfig{Pricing: testPricingWithPromptAndMultiplier("m", 10, 1)})
	known := resolver.Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "m"}, helper.UsageTokenCostInput{InputTokens: 1_000_000})).DualCosts()
	unknown := resolver.Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "missing"}, helper.UsageTokenCostInput{InputTokens: 1_000_000})).DualCosts()
	zero := resolver.Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "missing"}, helper.UsageTokenCostInput{})).DualCosts()
	var total pricing.DualCosts
	total.Merge(unknown)
	if total.Configured.HasKnown || total.Configured.TotalCostUSD != nil || total.Configured.Status != "unavailable" {
		t.Fatalf("empty merge fabricated known status %+v", total)
	}
	total.Merge(zero)
	if !total.Configured.HasKnown || total.Configured.TotalCostUSD == nil || *total.Configured.TotalCostUSD != 0 || total.Configured.Status != "partial" {
		t.Fatalf("known zero lost %+v", total)
	}
	total.Merge(known)
	if *total.Reference.TotalCostUSD != 10 || total.Reference.Complete || total.Reference.Status != "partial" {
		t.Fatalf("known subtotal lost %+v", total)
	}
	scaled := total.Scale(.5)
	if *scaled.Reference.TotalCostUSD != 5 || *total.Reference.TotalCostUSD != 10 {
		t.Fatal("scaling aliased input")
	}
	for _, factor := range []float64{math.Inf(1), math.NaN(), math.MaxFloat64, -1} {
		invalid := known.Scale(factor)
		if invalid.Reference.HasKnown || invalid.Reference.TotalCostUSD != nil {
			t.Fatalf("invalid scale %+v", invalid)
		}
		if _, err := json.Marshal(invalid); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDualCostsLegacyZeroKeepsUnsafeBaselineCompatible(t *testing.T) {
	zero := 0.0
	resolver := compileResolver(t, pricing.ModelConfig{Pricing: entities.ModelPriceSetting{Model: "huge", PromptPricePer1M: math.MaxFloat64, PriceMultiplier: &zero}})
	result := resolver.Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "huge"}, helper.UsageTokenCostInput{InputTokens: 2_000_000}))
	if !result.Available || result.Cost.TotalCostUSD != 0 {
		t.Fatalf("legacy compatibility lost: %+v", result)
	}
	if result.ReferenceAvailable {
		t.Fatal("overflow reference must be unavailable")
	}
	if _, err := json.Marshal(resolver.Selection(result)); err != nil {
		t.Fatalf("reference must not emit NaN/Inf JSON: %v", err)
	}
}

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

func TestDualCostsCredentialOnlyAttributionWarnings(t *testing.T) {
	unique := []pricing.CredentialBinding{{SubjectID: "cred_a", AuthType: "oauth", AuthIndex: "synthetic-private-index", Status: "unique"}}
	ambiguous := []pricing.CredentialBinding{{SubjectID: "cred_a", AuthType: "oauth", AuthIndex: "synthetic-private-index", Status: "ambiguous"}}
	collision := append(append([]pricing.CredentialBinding{}, unique...), pricing.CredentialBinding{SubjectID: "cred_b", AuthType: "apikey", AuthIndex: "synthetic-private-index", Status: "unique"})
	for _, mode := range []string{"default", "model", "none"} {
		t.Run(mode, func(t *testing.T) {
			for _, test := range []struct {
				name               string
				bindings           []pricing.CredentialBinding
				index, authType    string
				observed, selected bool
				warning            string
			}{
				{"unique", unique, "synthetic-private-index", "oauth", true, true, ""},
				{"ambiguous", ambiguous, "synthetic-private-index", "oauth", true, false, "unresolved_identity"},
				{"missing index", unique, "", "oauth", true, false, "unknown_identity"},
				{"unknown raw type", unique, "synthetic-private-index", "unknown", true, false, "unknown_identity"},
				{"missing raw type", unique, "synthetic-private-index", "", true, false, "unknown_identity"},
				{"typed collision", collision, "synthetic-private-index", "oauth", true, true, ""},
				{"typeless collision", collision, "synthetic-private-index", "", false, false, "unresolved_identity"},
			} {
				t.Run(test.name, func(t *testing.T) {
					var defaults []pricing.CredentialConfig
					var overrides pricing.OverrideConfig
					if mode == "default" {
						defaults = []pricing.CredentialConfig{{SubjectID: "cred_a", Multiplier: .2}}
					}
					if mode == "model" {
						overrides.CredentialModels = []pricing.CredentialModelConfig{{SubjectID: "cred_a", Model: "m", Mode: pricing.ModeMultiplier, Multiplier: .2}}
					}
					snapshot, err := pricing.CompileSnapshotWithCredentials([]pricing.ModelConfig{{Pricing: testPricingWithPromptAndMultiplier("m", 10, 3)}}, test.bindings, defaults, overrides)
					if err != nil {
						t.Fatal(err)
					}
					resolver := pricing.NewCatalog(snapshot).NewResolver()
					subject := pricing.NewCostSubject(pricing.UsageDimensions{Model: "m", AuthIndex: test.index}, helper.UsageTokenCostInput{InputTokens: 1_000_000})
					subject.AuthType, subject.ObservedIdentity = test.authType, test.observed
					result := resolver.Calculate(subject)
					warning, cost := test.warning, 30.0
					if mode == "none" {
						warning = ""
					} else if test.selected {
						cost = 2
					}
					if result.AttributionWarning != warning || result.Cost.TotalCostUSD != cost || !result.Available || result.ReferenceCost.TotalCostUSD != 10 || !result.ReferenceAvailable || result.ChannelID != "" || result.ChannelName != "" {
						t.Fatalf("credential-only attribution %+v want warning=%q cost=%g", result, warning, cost)
					}
					selection := resolver.Selection(result)
					if selection == nil || selection.SnapshotID != snapshot.ID() || selection.AttributionWarning != warning || (!test.selected && selection.SubjectID != "") {
						t.Fatalf("unsafe/unpinned explanation %+v", selection)
					}
					body, err := json.Marshal(selection)
					if err != nil || strings.Contains(string(body), "synthetic-private-index") {
						t.Fatalf("unsafe explanation %s %v", body, err)
					}
				})
			}
		})
	}
}

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

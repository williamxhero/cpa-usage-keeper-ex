package test

import (
	"encoding/json"
	"math"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
)

func TestResolverStrictThreeLayerPrecedenceAndLegacyIsolation(t *testing.T) {
	one, legacyMultiplier, matrixRate, officialRate := 1.0, 9.0, 7.0, 3.0
	snapshot, err := pricing.CompileSnapshotWithCredentials(
		[]pricing.ModelConfig{{Pricing: entities.ModelPriceSetting{Model: "m", PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: 1, PriceMultiplier: &legacyMultiplier}, Rules: []pricing.RuleConfig{{Key: "service_tier", Value: "priority", Multiplier: 5}}}},
		[]pricing.CredentialBinding{{SubjectID: "cred-a", AuthType: "apikey", AuthIndex: "auth-a", Status: "unique"}},
		[]pricing.CredentialConfig{{SubjectID: "cred-a", Multiplier: 2}},
		pricing.OverrideConfig{CredentialModels: []pricing.CredentialModelConfig{{SubjectID: "cred-a", Model: "m", Mode: pricing.ModeMultiplier, Multiplier: 4}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SetOfficialPrices([]entities.OfficialModelPrice{{CanonicalModelID: "m", DisplayModelID: "m", Source: entities.OfficialModelPriceSourceOfficial, PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: &officialRate, CompletionPricePer1M: &one, CacheReadPricePer1M: &one, CacheWritePricePer1M: &one}})
	snapshot.SetCredentialMatrix([]pricing.CredentialMatrixConfig{{SubjectID: "cred-a", CanonicalModelID: "m", Fixed: pricing.FixedTariff{PromptPricePer1M: &matrixRate, CompletionPricePer1M: &one, CacheReadPricePer1M: &one, CacheWritePricePer1M: &one, PricingStyle: entities.ModelPricingStyleOpenAI}}})
	resolver := pricing.NewCatalog(snapshot).NewResolver()
	subject := pricing.NewCostSubject(pricing.UsageDimensions{Model: "m", AuthIndex: "auth-a", ServiceTier: "priority"}, helper.UsageTokenCostInput{InputTokens: 1_000_000})
	result := resolver.Calculate(subject)
	if !result.Available || result.Cost.TotalCostUSD != matrixRate || result.PricingSource != pricing.CredentialMatrixSource {
		t.Fatalf("matrix must win over every legacy layer and official price: %+v", result)
	}
	if result.ReferenceCost.TotalCostUSD != officialRate || !result.ReferenceAvailable {
		t.Fatalf("matrix reference must remain official: %+v", result)
	}
	if result.RuleMultiplier != 1 || result.Multiplier != nil || result.ChannelID != "" {
		t.Fatalf("legacy adjustment leaked into strict result: %+v", result)
	}
}

func TestResolverStrictEmptyCatalogDoesNotFallbackLegacy(t *testing.T) {
	multiplier := 9.0
	snapshot, err := pricing.CompileSnapshotWithCredentials(
		[]pricing.ModelConfig{{Pricing: entities.ModelPriceSetting{Model: "legacy-model", PromptPricePer1M: 1, PriceMultiplier: &multiplier}}},
		[]pricing.CredentialBinding{{SubjectID: "cred-empty", AuthType: "apikey", AuthIndex: "empty-auth", Status: "unique"}},
		[]pricing.CredentialConfig{{SubjectID: "cred-empty", Multiplier: 3}},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SetOfficialPrices([]entities.OfficialModelPrice{})
	result := pricing.NewCatalog(snapshot).NewResolver().Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "legacy-model", AuthIndex: "empty-auth"}, helper.UsageTokenCostInput{InputTokens: 1_000_000}))
	if result.Available || result.ConfiguredEstimate.TotalCostUSD != nil || result.UnavailableReason != "no_match" {
		t.Fatalf("empty strict catalog fell back to legacy pricing: %+v", result)
	}
}

func TestResolverStrictOfficialOpenRouterAndUnavailable(t *testing.T) {
	prompt, completion := 2.0, 4.0
	snapshot, err := pricing.CompileSnapshot(nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SetOfficialPrices([]entities.OfficialModelPrice{{CanonicalModelID: "router/m", DisplayModelID: "router/m", Source: entities.OfficialModelPriceSourceOpenRouter, PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: &prompt, CompletionPricePer1M: &completion}})
	resolver := pricing.NewCatalog(snapshot).NewResolver()
	got := resolver.Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "router/m"}, helper.UsageTokenCostInput{InputTokens: 1_000_000}))
	if !got.Available || got.Cost.TotalCostUSD != prompt || got.PricingSource != entities.OfficialModelPriceSourceOpenRouter {
		t.Fatalf("expected OpenRouter fallback: %+v", got)
	}
	missing := resolver.Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "unknown"}, helper.UsageTokenCostInput{InputTokens: 1}))
	if missing.Available || missing.ConfiguredEstimate.Status != "unavailable" || missing.UnavailableReason != "no_match" {
		t.Fatalf("missing price must be unavailable: %+v", missing)
	}
	zero := resolver.Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "unknown"}, helper.UsageTokenCostInput{}))
	if !zero.Available || zero.ConfiguredEstimate.TotalCostUSD == nil || *zero.ConfiguredEstimate.TotalCostUSD != 0 {
		t.Fatalf("zero-token request must retain known-zero compatibility: %+v", zero)
	}
}

func TestResolverStrictTokenBucketsAndMissingSegment(t *testing.T) {
	prompt, completion, read, write := 2.0, 4.0, 6.0, 8.0
	snapshot, err := pricing.CompileSnapshot(nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SetOfficialPrices([]entities.OfficialModelPrice{{CanonicalModelID: "bucket-model", DisplayModelID: "bucket-model", Source: entities.OfficialModelPriceSourceOfficial, PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: &prompt, CompletionPricePer1M: &completion, CacheReadPricePer1M: &read, CacheWritePricePer1M: &write}})
	resolver := pricing.NewCatalog(snapshot).NewResolver()
	got := resolver.Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "bucket-model"}, helper.UsageTokenCostInput{InputTokens: 1_000_000, OutputTokens: 100_000, CacheReadTokens: 200_000, CacheCreationTokens: 100_000}))
	if !got.Available || math.Abs(got.Cost.UncachedInputCostUSD-1.4) > 1e-12 || math.Abs(got.Cost.CacheReadCostUSD-1.2) > 1e-12 || math.Abs(got.Cost.CacheWriteCostUSD-0.8) > 1e-12 || math.Abs(got.Cost.OutputCostUSD-0.4) > 1e-12 || math.Abs(got.Cost.TotalCostUSD-3.8) > 1e-12 {
		t.Fatalf("four token buckets were not independently priced: %+v", got)
	}
	partialRead := 6.0
	snapshot.SetOfficialPrices([]entities.OfficialModelPrice{{CanonicalModelID: "partial", PromptPricePer1M: &prompt, CompletionPricePer1M: &completion, CacheReadPricePer1M: &partialRead}})
	missingResult := pricing.NewCatalog(snapshot).NewResolver().Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "partial"}, helper.UsageTokenCostInput{CacheCreationTokens: 1}))
	if missingResult.Available || missingResult.UnavailableReason != "missing_segments" {
		t.Fatalf("missing required cache-write segment must be unavailable: %+v", missingResult)
	}
}

func TestResolverStrictExplicitZeroAndSnapshotDefensiveCopy(t *testing.T) {
	zero, one := 0.0, 1.0
	original := entities.OfficialModelPrice{CanonicalModelID: "m", DisplayModelID: "m", Source: entities.OfficialModelPriceSourceOfficial, PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: &zero, CompletionPricePer1M: &one, CacheReadPricePer1M: &one, CacheWritePricePer1M: &one}
	snapshot, err := pricing.CompileSnapshot(nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SetOfficialPrices([]entities.OfficialModelPrice{original})
	before := pricing.NewCatalog(snapshot).NewResolver().Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "m"}, helper.UsageTokenCostInput{InputTokens: 1_000_000}))
	*original.PromptPricePer1M = 99
	after := pricing.NewCatalog(snapshot).NewResolver().Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "m"}, helper.UsageTokenCostInput{InputTokens: 1_000_000}))
	if !before.Available || !after.Available || before.Cost.TotalCostUSD != 0 || after.Cost.TotalCostUSD != 0 {
		t.Fatalf("explicit zero must be available and immutable: before=%+v after=%+v", before, after)
	}
	if snapshot.ID() == "" || snapshot.ID() != snapshot.ID() {
		t.Fatal("snapshot ID must be stable")
	}
	body, err := json.Marshal(pricing.NewCatalog(snapshot).NewResolver().Selection(after))
	if err != nil || !json.Valid(body) || string(body) == "" {
		t.Fatalf("compatibility selection must remain valid JSON: %v %s", err, body)
	}
	if math.IsNaN(after.Cost.TotalCostUSD) || math.IsInf(after.Cost.TotalCostUSD, 0) {
		t.Fatal("cost must remain finite")
	}
}

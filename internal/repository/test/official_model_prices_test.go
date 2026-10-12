package test

import (
	"math"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
)

func TestSeedOfficialModelPricesIsIdempotentAndIgnoresLegacyMultiplier(t *testing.T) {
	db := openTestDatabase(t)
	multiplier := 0.25
	legacy := entities.ModelPriceSetting{
		Model: "legacy-model", PricingStyle: entities.ModelPricingStyleClaude,
		PromptPricePer1M: 3, CompletionPricePer1M: 15, CacheReadPricePer1M: 0.3, CacheWritePricePer1M: 3.75,
		PriceMultiplier: &multiplier,
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy price: %v", err)
	}

	for range 2 {
		if err := repository.SeedOfficialModelPrices(db); err != nil {
			t.Fatalf("SeedOfficialModelPrices: %v", err)
		}
	}
	prices, err := repository.ListOfficialModelPrices(db)
	if err != nil {
		t.Fatalf("ListOfficialModelPrices: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected one seeded row, got %+v", prices)
	}
	price := prices[0]
	if price.Source != entities.OfficialModelPriceSourceLegacySeed || price.PricingStyle != entities.ModelPricingStyleClaude {
		t.Fatalf("unexpected seeded provenance/style: %+v", price)
	}
	if price.PromptPricePer1M == nil || *price.PromptPricePer1M != 3 || price.CompletionPricePer1M == nil || *price.CompletionPricePer1M != 15 || price.CacheReadPricePer1M == nil || *price.CacheReadPricePer1M != 0.3 || price.CacheWritePricePer1M == nil || *price.CacheWritePricePer1M != 3.75 {
		t.Fatalf("legacy four-part price changed or multiplier was applied: %+v", price)
	}
	if price.MissingSegments != "" {
		t.Fatalf("complete seeded price has missing segments %q", price.MissingSegments)
	}
}

func TestUpsertOfficialModelPricesKeepsMissingSegmentsAndRejectsInvalidBatchAtomically(t *testing.T) {
	db := openTestDatabase(t)
	prompt := 1.25
	partial := entities.OfficialModelPrice{CanonicalModelID: "partial-model", PromptPricePer1M: &prompt}
	saved, err := repository.UpsertOfficialModelPrices(db, []entities.OfficialModelPrice{partial})
	if err != nil {
		t.Fatalf("UpsertOfficialModelPrices: %v", err)
	}
	if len(saved) != 1 || saved[0].MissingSegments != "completion,cache_read,cache_write" {
		t.Fatalf("unexpected persisted missing segments: %+v", saved)
	}

	negative := -1.0
	_, err = repository.UpsertOfficialModelPrices(db, []entities.OfficialModelPrice{
		{CanonicalModelID: "would-be-valid", PromptPricePer1M: &prompt},
		{CanonicalModelID: "invalid", PromptPricePer1M: &negative},
	})
	if err == nil {
		t.Fatal("expected negative price batch to be rejected")
	}
	prices, err := repository.ListOfficialModelPrices(db)
	if err != nil {
		t.Fatalf("ListOfficialModelPrices: %v", err)
	}
	if len(prices) != 1 || prices[0].CanonicalModelID != "partial-model" {
		t.Fatalf("invalid batch partially changed catalog: %+v", prices)
	}
}

func TestMarkOfficialModelPricesStalePreservesPriceAndAliases(t *testing.T) {
	db := openTestDatabase(t)
	prompt := 2.0
	price, err := repository.UpsertOfficialModelPrice(db, entities.OfficialModelPrice{
		CanonicalModelID: "model-a", PromptPricePer1M: &prompt,
		Aliases: []entities.OfficialModelPriceAlias{{Alias: "model-alias"}},
	})
	if err != nil {
		t.Fatalf("UpsertOfficialModelPrice: %v", err)
	}
	if err := repository.MarkOfficialModelPricesStale(db, []string{"model-a"}); err != nil {
		t.Fatalf("MarkOfficialModelPricesStale: %v", err)
	}
	prices, err := repository.ListOfficialModelPrices(db)
	if err != nil {
		t.Fatalf("ListOfficialModelPrices: %v", err)
	}
	if len(prices) != 1 || !prices[0].Stale || prices[0].PromptPricePer1M == nil || *prices[0].PromptPricePer1M != 2 || len(prices[0].Aliases) != 1 {
		t.Fatalf("stale marking altered stored price metadata: %+v", prices)
	}
	if price.ID == 0 || prices[0].Aliases[0].Alias != "model-alias" {
		t.Fatalf("unexpected upserted alias: %+v", prices[0])
	}
	if err := repository.RestoreOfficialModelPrices(db, []string{"model-a"}); err != nil {
		t.Fatalf("RestoreOfficialModelPrices: %v", err)
	}
	prices, err = repository.ListOfficialModelPrices(db)
	if err != nil {
		t.Fatalf("ListOfficialModelPrices after restore: %v", err)
	}
	if prices[0].Stale {
		t.Fatalf("expected stale state cleared: %+v", prices[0])
	}
}

func TestUpsertOfficialModelPricesRejectsNonFinitePrice(t *testing.T) {
	db := openTestDatabase(t)
	price := math.Inf(1)
	if _, err := repository.UpsertOfficialModelPrice(db, entities.OfficialModelPrice{
		CanonicalModelID: "invalid", PromptPricePer1M: &price,
	}); err == nil {
		t.Fatal("expected infinite official price to be rejected")
	}
}

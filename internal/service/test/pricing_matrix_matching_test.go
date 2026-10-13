package test

import (
	"context"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/service"
)

func TestMatrixEffectiveUsesDeterministicMatchesWithoutProviderGuessing(t *testing.T) {
	f := newCredentialMatrixFixture(t)
	ctx := context.Background()
	for index, model := range []string{"registered-alias", "custom/vendor/model", "model", "vendor", "ambiguous-alias", "unknown"} {
		if err := f.db.Create(&entities.UsageEvent{EventKey: model, Model: model, Timestamp: time.Date(2026, 10, 13, 12, index, 0, 0, time.UTC)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	number := 1.0
	snapshot, err := pricing.CompileSnapshot(nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SetOfficialPrices([]entities.OfficialModelPrice{
		{CanonicalModelID: "vendor/model", Source: "official", PricingStyle: "openai", PromptPricePer1M: &number, Aliases: []entities.OfficialModelPriceAlias{{Alias: "registered-alias"}, {Alias: "ambiguous-alias"}}},
		{CanonicalModelID: "different/model", Source: "openrouter", PricingStyle: "openai", PromptPricePer1M: &number, Aliases: []entities.OfficialModelPriceAlias{{Alias: "ambiguous-alias"}}},
		{CanonicalModelID: "conflict/a", Source: "official", PricingStyle: "openai", PromptPricePer1M: &number, Aliases: []entities.OfficialModelPriceAlias{{Alias: "conflicting-primary-alias"}}},
		{CanonicalModelID: "conflict/b", Source: "official", PricingStyle: "openai", PromptPricePer1M: &number, Aliases: []entities.OfficialModelPriceAlias{{Alias: "conflicting-primary-alias"}}},
	})
	if err := f.db.Create(&entities.UsageEvent{EventKey: "ambiguous", Model: "conflicting-primary-alias", Timestamp: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	f.catalog.Replace(snapshot)
	page, err := f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
	if err != nil {
		t.Fatal(err)
	}
	cells := effectiveMatrixCells(t, page)
	if len(cells) == 0 {
		t.Fatal("missing effective response")
	}
	for _, cell := range cells {
		switch cell.Model {
		case "registered-alias", "custom/vendor/model", "ambiguous-alias":
			if cell.Status != "partial" || cell.Source != "official" || cell.MatchedModel != "vendor/model" || cell.Prompt == nil || *cell.Prompt != 1 {
				t.Fatalf("deterministic identity/source precedence lost: %+v", cell)
			}
		case "model", "vendor", "unknown":
			if cell.Status != "unavailable" || cell.Reason != "no_match" || cell.Prompt != nil {
				t.Fatalf("provider/short-name price guessed: %+v", cell)
			}
		case "conflicting-primary-alias":
			if cell.Status != "unavailable" || cell.Reason != "ambiguous_match" || cell.Prompt != nil {
				t.Fatalf("ambiguous price guessed: %+v", cell)
			}
		}
	}
}

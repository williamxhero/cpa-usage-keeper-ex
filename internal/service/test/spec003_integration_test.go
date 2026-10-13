package test

import (
	"context"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	repodto "cpa-usage-keeper/internal/repository/dto"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

func TestSPEC003UsageServiceUsesMatrixOfficialAndUnavailableLayers(t *testing.T) {
	f := newCredentialMatrixFixture(t)
	ctx := context.Background()
	one := 1.0
	three := 3.0
	legacyMultiplier := 9.0
	if _, err := repository.UpsertModelPriceSetting(f.db, repodto.ModelPriceSettingInput{
		Model: "legacy-only", PromptPricePer1M: 1, CompletionPricePer1M: 1,
		CacheReadPricePer1M: 1, CacheWritePricePer1M: 1, PriceMultiplier: &legacyMultiplier,
	}); err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"matrix-model", "official-model"} {
		if _, err := repository.UpsertOfficialModelPrice(f.db, entities.OfficialModelPrice{
			CanonicalModelID: model, DisplayModelID: model, Source: entities.OfficialModelPriceSourceOfficial,
			PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: &three,
			CompletionPricePer1M: &one, CacheReadPricePer1M: &one, CacheWritePricePer1M: &one,
		}); err != nil {
			t.Fatal(err)
		}
	}
	matrix := f.matrix.(service.CredentialModelMatrixProvider)
	seven := [4]float64{7, 1, 1, 1}
	if _, err := matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{
		matrixTariff("matrix-a", "matrix-model", "openai", seven),
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if _, _, err := repository.InsertUsageEvents(f.db, []entities.UsageEvent{
		{EventKey: "spec003-matrix", AuthType: "apikey", AuthIndex: "synthetic-matrix-a", Model: "matrix-model", Timestamp: now, InputTokens: 1_000_000, TotalTokens: 1_000_000},
		{EventKey: "spec003-official", AuthType: "apikey", AuthIndex: "synthetic-matrix-a", Model: "official-model", Timestamp: now.Add(time.Millisecond), InputTokens: 1_000_000, TotalTokens: 1_000_000},
		{EventKey: "spec003-unavailable", AuthType: "apikey", AuthIndex: "synthetic-matrix-a", Model: "legacy-only", Timestamp: now.Add(2 * time.Millisecond), InputTokens: 1_000_000, TotalTokens: 1_000_000},
	}); err != nil {
		t.Fatal(err)
	}
	start, end := now.Add(-time.Minute), now.Add(time.Minute)
	page, err := service.NewUsageService(f.db, f.catalog).ListUsageEvents(ctx, servicedto.UsageFilter{StartTime: &start, EndTime: &end, EndExclusive: true})
	if err != nil {
		t.Fatal(err)
	}
	if page.PricingSnapshotID == "" || len(page.Events) != 3 {
		t.Fatalf("unexpected usage page: %+v", page)
	}
	byModel := make(map[string]servicedto.UsageEventRecord, len(page.Events))
	for _, event := range page.Events {
		byModel[event.Model] = event
	}
	matrixEvent := byModel["matrix-model"]
	if !matrixEvent.CostAvailable || matrixEvent.CostUSD != 7 || matrixEvent.DualCosts.Configured.Source != "credential_matrix" || matrixEvent.DualCosts.Reference.TotalCostUSD == nil || *matrixEvent.DualCosts.Reference.TotalCostUSD != 3 {
		t.Fatalf("matrix layer did not win or preserve official reference: %+v", matrixEvent)
	}
	officialEvent := byModel["official-model"]
	if !officialEvent.CostAvailable || officialEvent.CostUSD != 3 || officialEvent.DualCosts.Configured.Source != entities.OfficialModelPriceSourceOfficial {
		t.Fatalf("official layer mismatch: %+v", officialEvent)
	}
	missingEvent := byModel["legacy-only"]
	if missingEvent.CostAvailable || missingEvent.DualCosts.Configured.TotalCostUSD != nil || missingEvent.DualCosts.Configured.Status != "unavailable" {
		t.Fatalf("legacy price leaked into unavailable layer: %+v", missingEvent)
	}
	for _, event := range page.Events {
		if event.PricingSnapshotID != page.PricingSnapshotID {
			t.Fatalf("event used a different pricing snapshot: %+v", event)
		}
	}

	// Clearing the final matrix row must not resurrect the legacy model price
	// after a fresh snapshot load; the persisted mode marker is part of the fact.
	if _, err := matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{{SubjectID: "matrix-a", CanonicalModelID: "matrix-model", Clear: true}}); err != nil {
		t.Fatal(err)
	}
	restarted, err := repository.LoadPricingSnapshot(ctx, f.db)
	if err != nil {
		t.Fatal(err)
	}
	result := pricing.NewCatalog(restarted).NewResolver().Calculate(pricing.NewCostSubject(pricing.UsageDimensions{Model: "legacy-only", AuthIndex: "synthetic-matrix-a"}, helper.UsageTokenCostInput{InputTokens: 1_000_000}))
	if result.Available || result.ConfiguredEstimate.TotalCostUSD != nil {
		t.Fatalf("legacy price returned after strict mode restart: %+v", result)
	}
}

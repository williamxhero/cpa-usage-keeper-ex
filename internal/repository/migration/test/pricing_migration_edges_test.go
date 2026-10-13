package test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
)

func TestPricingMigrationStandaloneFixedAndMultiplierMaterialization(t *testing.T) {
	f := newPricingMigrationFixture(t)
	rows := []entities.CredentialModelMultiplier{
		{SubjectID: "subject-00", Model: "standalone-model", Mode: pricing.ModeFixed, PricingStyle: entities.ModelPricingStyleClaude, PromptPricePer1M: floatPtrForTest(3), CompletionPricePer1M: floatPtrForTest(4), CacheReadPricePer1M: floatPtrForTest(5), CacheWritePricePer1M: floatPtrForTest(6)},
		{SubjectID: "subject-00", Model: "legacy-model-31", Mode: pricing.ModeMultiplier, Multiplier: .25},
		{SubjectID: "subject-00", Model: "incomplete-official", Mode: pricing.ModeMultiplier, Multiplier: 2},
		{SubjectID: "subject-00", Model: "missing-official", Mode: pricing.ModeMultiplier, Multiplier: 2},
	}
	if err := f.db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&entities.OfficialModelPrice{CanonicalModelID: "incomplete-official", DisplayModelID: "incomplete-official", Source: entities.OfficialModelPriceSourceLegacySeed, PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: floatPtrForTest(1), CompletionPricePer1M: floatPtrForTest(2), CacheReadPricePer1M: floatPtrForTest(3)}).Error; err != nil {
		t.Fatal(err)
	}
	report, err := repository.RunLegacyPricingMigration(context.Background(), f.db, repository.PricingMigrationOptions{BatchID: "materialization"})
	if err != nil || report.Batch.Status != entities.PricingMigrationStatusSucceeded {
		t.Fatalf("migration = %+v, %v", report.Batch, err)
	}
	var fixed, materialized entities.CredentialModelPriceOverride
	if err := f.db.Where("subject_id = ? AND canonical_model_id = ?", "subject-00", "standalone-model").First(&fixed).Error; err != nil {
		t.Fatal(err)
	}
	if fixed.PromptPricePer1M != 3 || fixed.CompletionPricePer1M != 4 || fixed.CacheReadPricePer1M != 5 || fixed.CacheWritePricePer1M != 6 || fixed.PricingStyle != entities.ModelPricingStyleClaude || fixed.Source != "migrated_from_fixed" {
		t.Fatalf("standalone fixed tariff changed: %+v", fixed)
	}
	if err := f.db.Where("subject_id = ? AND canonical_model_id = ?", "subject-00", "legacy-model-31").First(&materialized).Error; err != nil {
		t.Fatal(err)
	}
	if materialized.PromptPricePer1M != 8 || materialized.CompletionPricePer1M != 8.25 || materialized.CacheReadPricePer1M != .775 || materialized.CacheWritePricePer1M != .3875 || materialized.Source != "migrated_from_multiplier" {
		t.Fatalf("multiplier did not use newly migrated official segments exactly once: %+v", materialized)
	}
	var fabricated int64
	if err := f.db.Model(&entities.CredentialModelPriceOverride{}).Where("canonical_model_id IN ?", []string{"missing-official", "incomplete-official"}).Count(&fabricated).Error; err != nil || fabricated != 0 {
		t.Fatalf("unknown segments fabricated: %d, %v", fabricated, err)
	}
	reasons := map[string]int{}
	for _, item := range report.Items {
		if item.Category == entities.PricingMigrationCategoryCredentialMultiplier {
			reasons[item.Reason]++
		}
	}
	if reasons["missing_official_model"] != 1 || reasons["official_price_missing_segment"] != 1 {
		t.Fatalf("missing multiplier reasons: %+v", reasons)
	}
}

func TestPricingMigrationRepositoryReportRedactsPollutedAuditRows(t *testing.T) {
	f := newPricingMigrationFixture(t)
	ctx := context.Background()
	if _, err := repository.RunLegacyPricingMigration(ctx, f.db, repository.PricingMigrationOptions{BatchID: "safe-report"}); err != nil {
		t.Fatal(err)
	}
	secret := "https://user:private-token@secret.invalid/?key=secret#private /private/auth.json"
	if err := f.db.Model(&entities.PricingMigrationBatch{}).Where("id = ?", "safe-report").Updates(map[string]any{"kind": secret, "status": secret, "failure_reason": secret, "previous_pricing_mode": secret}).Error; err != nil {
		t.Fatal(err)
	}
	var item entities.PricingMigrationItem
	if err := f.db.Where("batch_id = ?", "safe-report").First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&item).Updates(map[string]any{"reason": secret, "source_ref": secret, "target_ref": secret, "source_fingerprint": secret, "target_fingerprint": secret, "outcome": secret}).Error; err != nil {
		t.Fatal(err)
	}
	report, err := repository.GetPricingMigrationReport(ctx, f.db, "safe-report")
	if err != nil {
		t.Fatal(err)
	}
	batches, err := repository.ListPricingMigrationBatches(ctx, f.db)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{report, batches} {
		encoded, err := json.Marshal(value)
		if err != nil || strings.Contains(string(encoded), "private-token") || strings.Contains(string(encoded), "secret.invalid") || strings.Contains(string(encoded), "/private/") {
			t.Fatalf("repository audit response leaked private data: %s, %v", encoded, err)
		}
	}
}

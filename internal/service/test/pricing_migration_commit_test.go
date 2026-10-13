package test

import (
	"context"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
	"gorm.io/gorm"
)

func TestPricingMigrationDeferredCommitFailureAfterCandidateCompilation(t *testing.T) {
	f := newServicePricingMigrationFixture(t)
	provider := service.NewPricingService(f.db, f.catalog).(service.PricingMigrationProvider)
	before := f.catalog.Snapshot()
	if err := f.db.Exec(`CREATE TABLE migration_deferred_probe (subject_id TEXT, FOREIGN KEY(subject_id) REFERENCES credential_pricing_subjects(id) DEFERRABLE INITIALLY DEFERRED)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`CREATE TRIGGER migration_deferred_failure AFTER INSERT ON official_model_prices BEGIN INSERT INTO migration_deferred_probe(subject_id) VALUES ('absent-subject'); END`).Error; err != nil {
		t.Fatal(err)
	}
	candidateRead := false
	const callback = "test:migration-deferred-candidate"
	if err := f.db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if rows, ok := tx.Statement.Dest.(*[]entities.OfficialModelPrice); ok && tx.Error == nil && len(*rows) == 1 && (*rows)[0].CanonicalModelID == "service-model" {
			candidateRead = true
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.Callback().Query().Remove(callback) })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	report, err := provider.RunLegacyPricingMigration(ctx, repository.PricingMigrationOptions{BatchID: "deferred-commit"})
	if err == nil || !candidateRead || report.Batch.Status != entities.PricingMigrationStatusFailed {
		t.Fatalf("expected failed COMMIT after candidate read: read=%v, report=%+v, err=%v", candidateRead, report.Batch, err)
	}
	if f.catalog.Snapshot() != before || repository.IsStrictPricingActivated(f.db) || serviceMigrationCost(t, f.catalog) != 1 {
		t.Fatal("failed COMMIT changed pricing")
	}
	var official, probe int64
	if err := f.db.Model(&entities.OfficialModelPrice{}).Count(&official).Error; err != nil || official != 0 {
		t.Fatalf("failed COMMIT retained official facts: %d, %v", official, err)
	}
	if err := f.db.Table("migration_deferred_probe").Count(&probe).Error; err != nil || probe != 0 {
		t.Fatalf("failed COMMIT retained trigger effect: %d, %v", probe, err)
	}
	if err := f.db.Exec("DROP TRIGGER migration_deferred_failure").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := provider.RunLegacyPricingMigration(ctx, repository.PricingMigrationOptions{BatchID: "deferred-commit"}); err != nil {
		t.Fatalf("retry after COMMIT failure: %v", err)
	}
	if serviceMigrationCost(t, f.catalog) != 2 {
		t.Fatal("retry did not publish strict pricing")
	}
}

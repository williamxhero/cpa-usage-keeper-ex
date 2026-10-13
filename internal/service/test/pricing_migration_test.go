package test

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
	"gorm.io/gorm"
)

type servicePricingMigrationFixture struct {
	db      *gorm.DB
	catalog *pricing.Catalog
	path    string
}

func newServicePricingMigrationFixture(t *testing.T) servicePricingMigrationFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pricing-migration-service.db")
	db, err := repository.OpenDatabase(config.Config{SQLitePath: path})
	if err != nil {
		t.Fatalf("open pricing migration database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get pricing migration database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, table := range []string{"official_model_price_aliases", "official_model_prices", "credential_model_price_overrides", "model_price_rules", "model_price_settings", "credential_model_multipliers", "credential_price_defaults", "credential_pricing_associations", "credential_pricing_subjects", "usage_identities", "app_settings", "pricing_migration_items", "pricing_migration_batch_counts", "pricing_migration_batches", "pricing_migration_legacy_states"} {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("clear %s: %v", table, err)
		}
	}
	identity := entities.UsageIdentity{ID: 1, Name: "service credential", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "service-auth-index", BindingIdentityStatus: "unique", Type: "codex"}
	subject := entities.CredentialPricingSubject{ID: "service-subject", UsageIdentityID: identity.ID, AuthType: identity.AuthType, AuthTypeName: identity.AuthTypeName, Identity: identity.Identity}
	setting := entities.ModelPriceSetting{Model: "service-model", PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: 2, CompletionPricePer1M: 2, CacheReadPricePer1M: 2, CacheWritePricePer1M: 2}
	defaultMultiplier := entities.CredentialPriceDefault{SubjectID: subject.ID, Multiplier: .5}
	association := entities.CredentialPricingAssociation{ID: "service-association", SubjectID: subject.ID, AuthType: identity.AuthType, AuthTypeName: identity.AuthTypeName, Identity: identity.Identity, Enabled: true}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	if err := db.Create(&subject).Error; err != nil {
		t.Fatalf("seed subject: %v", err)
	}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatalf("seed model setting: %v", err)
	}
	if err := db.Create(&defaultMultiplier).Error; err != nil {
		t.Fatalf("seed default: %v", err)
	}
	if err := db.Create(&association).Error; err != nil {
		t.Fatalf("seed association: %v", err)
	}
	_, catalog := newCatalogPricingService(t, db)
	return servicePricingMigrationFixture{db: db, catalog: catalog, path: path}
}

func serviceMigrationCost(t *testing.T, catalog *pricing.Catalog) float64 {
	t.Helper()
	result := catalog.NewResolver().Calculate(repository.UsageEventCostSubject(entities.UsageEvent{Model: "service-model", AuthType: "oauth", AuthIndex: "service-auth-index", InputTokens: 1_000_000}))
	if !result.Available {
		t.Fatalf("service migration cost unavailable: %+v", result)
	}
	return result.Cost.TotalCostUSD
}

func TestPricingMigrationServicePublishesIdempotentlyRollsBackAndRestarts(t *testing.T) {
	f := newServicePricingMigrationFixture(t)
	provider := service.NewPricingService(f.db, f.catalog).(service.PricingMigrationProvider)
	ctx := context.Background()
	if got := serviceMigrationCost(t, f.catalog); math.Abs(got-1) > 1e-9 {
		t.Fatalf("legacy cost = %v, want 1", got)
	}
	first, err := provider.RunLegacyPricingMigration(ctx, repository.PricingMigrationOptions{BatchID: "service-migration", BatchSize: 1})
	if err != nil {
		t.Fatalf("service migration: %v", err)
	}
	publishedID := f.catalog.Snapshot().ID()
	if first.Batch.Status != entities.PricingMigrationStatusSucceeded || math.Abs(serviceMigrationCost(t, f.catalog)-2) > 1e-9 {
		t.Fatalf("strict publication failed: %+v cost=%v", first.Batch, serviceMigrationCost(t, f.catalog))
	}
	second, err := provider.RunLegacyPricingMigration(ctx, repository.PricingMigrationOptions{BatchID: "service-migration", BatchSize: 9})
	if err != nil {
		t.Fatalf("idempotent service migration: %v", err)
	}
	if second.Batch.ID != first.Batch.ID || f.catalog.Snapshot().ID() != publishedID {
		t.Fatalf("terminal retry republished snapshot: first=%+v second=%+v ids=%q/%q", first.Batch, second.Batch, publishedID, f.catalog.Snapshot().ID())
	}
	if err := f.db.Create(&entities.CredentialModelPriceOverride{SubjectID: "service-subject", CanonicalModelID: "later-admin-model", PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: 7, CompletionPricePer1M: 7, CacheReadPricePer1M: 7, CacheWritePricePer1M: 7, Source: "admin"}).Error; err != nil {
		t.Fatalf("seed later matrix row: %v", err)
	}
	rolled, err := provider.RollbackPricingMigration(ctx, "service-migration")
	if err != nil {
		t.Fatalf("service rollback: %v", err)
	}
	if rolled.Batch.Status != entities.PricingMigrationStatusRolledBack || repository.IsStrictPricingActivated(f.db) || math.Abs(serviceMigrationCost(t, f.catalog)-1) > 1e-9 {
		t.Fatalf("legacy restoration failed: %+v cost=%v", rolled.Batch, serviceMigrationCost(t, f.catalog))
	}
	var laterCount int64
	if err := f.db.Model(&entities.CredentialModelPriceOverride{}).Where("canonical_model_id = ?", "later-admin-model").Count(&laterCount).Error; err != nil || laterCount != 1 {
		t.Fatalf("rollback removed later admin matrix row: %d %v", laterCount, err)
	}
	// A fresh service over the same file must observe a newly published strict batch.
	if _, err := provider.RunLegacyPricingMigration(ctx, repository.PricingMigrationOptions{BatchID: "restart-migration"}); err != nil {
		t.Fatalf("republish strict pricing before restart: %v", err)
	}
	if !repository.IsStrictPricingActivated(f.db) || math.Abs(serviceMigrationCost(t, f.catalog)-2) > 1e-9 {
		t.Fatal("strict snapshot was not persisted before restart")
	}
	sqlDB, err := f.db.DB()
	if err != nil {
		t.Fatalf("get database for restart: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database for restart: %v", err)
	}
	reopened, err := repository.OpenDatabase(config.Config{SQLitePath: f.path})
	if err != nil {
		t.Fatalf("reopen pricing migration database: %v", err)
	}
	reopenedSQL, err := reopened.DB()
	if err != nil {
		t.Fatalf("get restarted database: %v", err)
	}
	t.Cleanup(func() { _ = reopenedSQL.Close() })
	reopenedSnapshot, err := repository.LoadPricingSnapshot(ctx, reopened)
	if err != nil {
		t.Fatalf("load restarted snapshot: %v", err)
	}
	reopenedCatalog := pricing.NewCatalog(reopenedSnapshot)
	if !repository.IsStrictPricingActivated(reopened) || math.Abs(serviceMigrationCost(t, reopenedCatalog)-2) > 1e-9 {
		t.Fatal("restart did not preserve strict pricing")
	}
}

func TestPricingMigrationServiceBlocksStaleRollbackAfterStrictAdminMutation(t *testing.T) {
	f := newServicePricingMigrationFixture(t)
	provider := service.NewPricingService(f.db, f.catalog).(service.PricingMigrationProvider)
	ctx := context.Background()
	if _, err := provider.RunLegacyPricingMigration(ctx, repository.PricingMigrationOptions{BatchID: "stale-rollback"}); err != nil {
		t.Fatalf("service migration: %v", err)
	}
	fixed := 8.0
	_, err := provider.(service.CredentialModelMatrixProvider).UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{{SubjectID: "service-subject", CanonicalModelID: "service-model", PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: &fixed, CompletionPricePer1M: &fixed, CacheReadPricePer1M: &fixed, CacheWritePricePer1M: &fixed}})
	if err != nil {
		t.Fatalf("strict admin matrix mutation: %v", err)
	}
	before := f.catalog.Snapshot().ID()
	if _, err := provider.RollbackPricingMigration(ctx, "stale-rollback"); err == nil {
		t.Fatal("stale rollback after strict admin mutation unexpectedly succeeded")
	}
	if f.catalog.Snapshot().ID() != before || !repository.IsStrictPricingActivated(f.db) {
		t.Fatal("stale rollback changed strict publication")
	}
}

func TestPricingMigrationServiceCommitFailureKeepsPublishedSnapshot(t *testing.T) {
	f := newServicePricingMigrationFixture(t)
	provider := service.NewPricingService(f.db, f.catalog).(service.PricingMigrationProvider)
	before := f.catalog.Snapshot()
	// Deferred foreign keys fail at commit, after compilation, and exercise publication ordering.
	if err := f.db.Exec(`CREATE TABLE pricing_commit_probe (subject_id TEXT, FOREIGN KEY(subject_id) REFERENCES credential_pricing_subjects(id) DEFERRABLE INITIALLY DEFERRED)`).Error; err != nil {
		t.Fatalf("create commit failure probe: %v", err)
	}
	if err := f.db.Exec(`CREATE TRIGGER pricing_commit_failure AFTER INSERT ON official_model_prices BEGIN SELECT RAISE(FAIL, 'credential-key=https://secret.invalid/provider-token'); END`).Error; err != nil {
		t.Fatalf("create deferred commit failure: %v", err)
	}
	_, err := provider.RunLegacyPricingMigration(context.Background(), repository.PricingMigrationOptions{BatchID: "commit-failure"})
	if err == nil {
		t.Fatal("deferred migration commit failure unexpectedly succeeded")
	}
	if failedReport, reportErr := provider.GetPricingMigrationReport(context.Background(), "commit-failure"); reportErr != nil {
		t.Fatalf("load failed migration report: %v", reportErr)
	} else if failedReport.Batch.FailureReason != "" && (strings.Contains(failedReport.Batch.FailureReason, "secret.invalid") || strings.Contains(failedReport.Batch.FailureReason, "provider-token")) {
		t.Fatalf("failure report leaked credential-bearing error: %q", failedReport.Batch.FailureReason)
	}
	if f.catalog.Snapshot() != before || repository.IsStrictPricingActivated(f.db) || math.Abs(serviceMigrationCost(t, f.catalog)-1) > 1e-9 {
		t.Fatal("failed commit changed published pricing")
	}
	var officialCount int64
	if err := f.db.Model(&entities.OfficialModelPrice{}).Count(&officialCount).Error; err != nil || officialCount != 0 {
		t.Fatalf("failed commit persisted official facts: %d %v", officialCount, err)
	}
}

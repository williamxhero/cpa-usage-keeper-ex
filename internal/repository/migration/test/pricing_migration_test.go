package test

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"gorm.io/gorm"
)

const pricingMigrationBatchID = "pricing-migration-fixture"

type pricingMigrationFixture struct {
	db   *gorm.DB
	path string
}

func newPricingMigrationFixture(t *testing.T) pricingMigrationFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pricing-migration.db")
	db, err := repository.OpenDatabase(config.Config{SQLitePath: path})
	if err != nil {
		t.Fatalf("open pricing migration database: %v", err)
	}
	closeMigrationTestDatabase(t, db)
	f := pricingMigrationFixture{db: db, path: path}
	f.clear(t)
	f.seed(t)
	return f
}

func (f pricingMigrationFixture) clear(t *testing.T) {
	t.Helper()
	for _, table := range []string{
		"pricing_migration_legacy_states", "pricing_migration_items", "pricing_migration_batch_counts", "pricing_migration_batches",
		"official_model_price_aliases", "official_model_prices", "credential_model_price_overrides", "model_price_rules", "model_price_settings",
		"credential_model_multipliers", "credential_price_defaults", "pricing_channels", "pricing_channel_members", "channel_price_defaults", "channel_model_prices",
		"credential_pricing_associations", "credential_pricing_subjects", "usage_identities", "app_settings",
	} {
		if err := f.db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("clear %s: %v", table, err)
		}
	}
}

func (f pricingMigrationFixture) seed(t *testing.T) {
	t.Helper()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	identities := make([]entities.UsageIdentity, 10)
	subjects := make([]entities.CredentialPricingSubject, 10)
	for i := 0; i < 10; i++ {
		identity := fmt.Sprintf("directory-secret-%02d", i)
		identities[i] = entities.UsageIdentity{ID: int64(i + 1), Name: "private name " + identity, AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: identity, BindingIdentityStatus: "unique", Type: "codex", LookupKey: "private-lookup-" + identity, BaseURL: "https://private-endpoint.invalid/" + identity, CreatedAt: now, UpdatedAt: now}
		subjects[i] = entities.CredentialPricingSubject{ID: fmt.Sprintf("subject-%02d", i), UsageIdentityID: int64(i + 1), AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: identity, CreatedAt: now}
	}
	if err := f.db.Create(&identities).Error; err != nil {
		t.Fatalf("seed identities: %v", err)
	}
	if err := f.db.Create(&subjects).Error; err != nil {
		t.Fatalf("seed subjects: %v", err)
	}
	settings := make([]entities.ModelPriceSetting, 32)
	for i := range settings {
		settings[i] = entities.ModelPriceSetting{Model: fmt.Sprintf("legacy-model-%02d", i), PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: float64(i + 1), CompletionPricePer1M: float64(i + 2), CacheReadPricePer1M: float64(i) / 10, CacheWritePricePer1M: float64(i) / 20, PriceMultiplier: floatPtrForTest(1), CreatedAt: now, UpdatedAt: now}
	}
	if err := f.db.Create(&settings).Error; err != nil {
		t.Fatalf("seed model settings: %v", err)
	}
	fixed := make([]entities.CredentialModelMultiplier, 21)
	for i := range fixed {
		fixed[i] = entities.CredentialModelMultiplier{SubjectID: subjects[i%len(subjects)].ID, Model: settings[i].Model, Mode: pricing.ModeFixed, PricingStyle: entities.ModelPricingStyleOpenAI, Multiplier: 1, PromptPricePer1M: floatPtrForTest(float64(i + 10)), CompletionPricePer1M: floatPtrForTest(float64(i + 11)), CacheReadPricePer1M: floatPtrForTest(float64(i) / 10), CacheWritePricePer1M: floatPtrForTest(float64(i) / 20), UpdatedAt: now}
	}
	if err := f.db.Create(&fixed).Error; err != nil {
		t.Fatalf("seed fixed credential matrix: %v", err)
	}
	defaults := make([]entities.CredentialPriceDefault, 10)
	for i := range defaults {
		defaults[i] = entities.CredentialPriceDefault{SubjectID: subjects[i].ID, Multiplier: .5, UpdatedAt: now}
	}
	if err := f.db.Create(&defaults).Error; err != nil {
		t.Fatalf("seed credential defaults: %v", err)
	}
	channels := make([]entities.PricingChannel, 4)
	members := make([]entities.PricingChannelMember, 10)
	channelDefaults := make([]entities.ChannelPriceDefault, 4)
	channelModels := make([]entities.ChannelModelPrice, 4)
	for i := range channels {
		id := fmt.Sprintf("channel-%02d", i)
		channels[i] = entities.PricingChannel{ID: id, Name: "private channel endpoint https://secret.invalid/" + id, CreatedAt: now, UpdatedAt: now}
		channelDefaults[i] = entities.ChannelPriceDefault{ChannelID: id, Multiplier: .7, UpdatedAt: now}
		channelModels[i] = entities.ChannelModelPrice{ChannelID: id, Model: settings[i].Model, Multiplier: .8, Mode: pricing.ModeMultiplier, UpdatedAt: now}
	}
	for i := range members {
		members[i] = entities.PricingChannelMember{SubjectID: subjects[i].ID, ChannelID: channels[i%len(channels)].ID}
	}
	for _, rows := range []any{&channels, &members, &channelDefaults, &channelModels} {
		if err := f.db.Create(rows).Error; err != nil {
			t.Fatalf("seed channel layer: %v", err)
		}
	}
	associations := make([]entities.CredentialPricingAssociation, 10)
	for i := range associations {
		associations[i] = entities.CredentialPricingAssociation{ID: fmt.Sprintf("association-%02d", i), SubjectID: subjects[i].ID, AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "association-private-secret-" + fmt.Sprint(i), Enabled: true, CreatedAt: now}
	}
	if err := f.db.Create(&associations).Error; err != nil {
		t.Fatalf("seed associations: %v", err)
	}
	if err := f.db.Create(&entities.ModelPriceRule{ModelPriceSettingID: settings[0].ID, Key: "service_tier", Value: "premium", Multiplier: 1.5, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed model rule: %v", err)
	}
}

func floatPtrForTest(v float64) *float64 { return &v }

func TestPricingMigrationPublishesStrictFactsReportAndRollback(t *testing.T) {
	f := newPricingMigrationFixture(t)
	ctx := context.Background()
	before, err := repository.LoadPricingSnapshot(ctx, f.db)
	if err != nil {
		t.Fatalf("load pre-migration snapshot: %v", err)
	}
	beforeModels := before.ModelConfigs()

	report, err := repository.RunLegacyPricingMigration(ctx, f.db, repository.PricingMigrationOptions{BatchID: pricingMigrationBatchID, BatchSize: 3})
	if err != nil {
		t.Fatalf("run pricing migration: %v", err)
	}
	if report.Batch.Status != entities.PricingMigrationStatusSucceeded || !repository.IsStrictPricingActivated(f.db) {
		t.Fatalf("migration did not publish strict mode: %+v", report.Batch)
	}
	if report.Batch.ImportedCount != 53 || report.Batch.SkippedCount != 43 || report.Batch.RejectedCount != 0 || report.Batch.ConflictCount != 0 {
		t.Fatalf("unexpected batch totals: %+v", report.Batch)
	}
	wantCounts := map[string][4]int{
		entities.PricingMigrationCategoryModelPrices:        {32, 0, 0, 0},
		entities.PricingMigrationCategoryCredentialFixed:    {21, 0, 0, 0},
		entities.PricingMigrationCategoryCredentialDefaults: {0, 10, 0, 0},
		entities.PricingMigrationCategoryChannels:           {0, 4, 0, 0},
		entities.PricingMigrationCategoryChannelMembers:     {0, 10, 0, 0},
		entities.PricingMigrationCategoryChannelDefaults:    {0, 4, 0, 0},
		entities.PricingMigrationCategoryChannelModels:      {0, 4, 0, 0},
		entities.PricingMigrationCategoryAssociations:       {0, 10, 0, 0},
		entities.PricingMigrationCategoryModelRules:         {0, 1, 0, 0},
	}
	if len(report.Counts) != len(wantCounts) {
		t.Fatalf("report counts = %d, want %d: %+v", len(report.Counts), len(wantCounts), report.Counts)
	}
	for _, count := range report.Counts {
		want, ok := wantCounts[count.Category]
		if !ok || [4]int{count.Migrated, count.Skipped, count.Rejected, count.Conflicts} != want {
			t.Errorf("unexpected count %+v", count)
		}
	}
	if len(report.Items) != 96 {
		t.Fatalf("report items = %d, want 96", len(report.Items))
	}
	for _, item := range report.Items {
		for _, secret := range []string{"directory-secret", "private-lookup", "private-endpoint", "association-private", "https://secret"} {
			if strings.Contains(item.SourceRef+item.TargetRef+item.SourceFingerprint+item.TargetFingerprint, secret) {
				t.Errorf("report leaked %q in %+v", secret, item)
			}
		}
	}
	var officialCount, matrixCount int64
	if err := f.db.Model(&entities.OfficialModelPrice{}).Count(&officialCount).Error; err != nil || officialCount != 32 {
		t.Fatalf("official migrated rows = %d, err=%v", officialCount, err)
	}
	if err := f.db.Model(&entities.CredentialModelPriceOverride{}).Count(&matrixCount).Error; err != nil || matrixCount != 21 {
		t.Fatalf("matrix migrated rows = %d, err=%v", matrixCount, err)
	}
	if _, err := repository.RunLegacyPricingMigration(ctx, f.db, repository.PricingMigrationOptions{BatchID: pricingMigrationBatchID, BatchSize: 1}); err != nil {
		t.Fatalf("same successful batch should be idempotent: %v", err)
	}
	later := entities.CredentialModelPriceOverride{SubjectID: "subject-00", CanonicalModelID: "later-admin-model", PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: 9, CompletionPricePer1M: 9, CacheReadPricePer1M: 9, CacheWritePricePer1M: 9, Source: "admin"}
	if err := f.db.Create(&later).Error; err != nil {
		t.Fatalf("seed later admin matrix row: %v", err)
	}
	rolled, err := repository.RollbackPricingMigration(ctx, f.db, pricingMigrationBatchID)
	if err != nil {
		t.Fatalf("rollback pricing migration: %v", err)
	}
	if rolled.Batch.Status != entities.PricingMigrationStatusRolledBack || repository.IsStrictPricingActivated(f.db) {
		t.Fatalf("rollback did not restore legacy mode: %+v", rolled.Batch)
	}
	after, err := repository.LoadPricingSnapshot(ctx, f.db)
	if err != nil {
		t.Fatalf("load post-rollback snapshot: %v", err)
	}
	if !reflect.DeepEqual(beforeModels, after.ModelConfigs()) {
		t.Fatalf("rollback changed legacy model result\nbefore=%+v\nafter=%+v", beforeModels, after.ModelConfigs())
	}
	var laterCount int64
	if err := f.db.Model(&entities.CredentialModelPriceOverride{}).Where("canonical_model_id = ?", later.CanonicalModelID).Count(&laterCount).Error; err != nil || laterCount != 1 {
		t.Fatalf("rollback removed later admin row: count=%d err=%v", laterCount, err)
	}
}

func TestPricingMigrationFreezesInputStagesAndResumesIdempotently(t *testing.T) {
	f := newPricingMigrationFixture(t)
	ctx := context.Background()
	report, err := repository.RunLegacyPricingMigration(ctx, f.db, repository.PricingMigrationOptions{BatchID: "frozen-stage", BatchSize: 2})
	if err != nil {
		t.Fatalf("stage pricing migration: %v", err)
	}
	if !report.Batch.InputFrozen || report.Batch.BatchSize != 2 || report.Batch.StagedCount != len(report.Items) {
		t.Fatalf("input was not frozen/staged: batch=%+v items=%d", report.Batch, len(report.Items))
	}
	first := append([]entities.PricingMigrationItem(nil), report.Items...)
	resumed, err := repository.RunLegacyPricingMigration(ctx, f.db, repository.PricingMigrationOptions{BatchID: "frozen-stage", BatchSize: 99})
	if err != nil {
		t.Fatalf("resume frozen pricing migration: %v", err)
	}
	if resumed.Batch.BatchSize != 2 || !reflect.DeepEqual(first, resumed.Items) {
		t.Fatalf("resume changed frozen staged input: first=%+v resumed=%+v", first, resumed)
	}
}

func TestPricingMigrationStrictReloadDoesNotQueryRetainedLegacyLayersAndSurvivesRestart(t *testing.T) {
	f := newPricingMigrationFixture(t)
	ctx := context.Background()
	if _, err := repository.RunLegacyPricingMigration(ctx, f.db, repository.PricingMigrationOptions{BatchID: pricingMigrationBatchID}); err != nil {
		t.Fatalf("run pricing migration: %v", err)
	}
	for source, retained := range map[string]string{
		"model_price_settings":            "retained_model_price_settings",
		"model_price_rules":               "retained_model_price_rules",
		"credential_model_multipliers":    "retained_credential_model_multipliers",
		"credential_price_defaults":       "retained_credential_price_defaults",
		"pricing_channels":                "retained_pricing_channels",
		"pricing_channel_members":         "retained_pricing_channel_members",
		"channel_price_defaults":          "retained_channel_price_defaults",
		"channel_model_prices":            "retained_channel_model_prices",
		"credential_pricing_associations": "retained_credential_pricing_associations",
	} {
		if err := f.db.Exec("ALTER TABLE " + source + " RENAME TO " + retained).Error; err != nil {
			t.Fatalf("retain legacy table %s: %v", source, err)
		}
	}
	if _, err := repository.LoadPricingSnapshot(ctx, f.db); err != nil {
		t.Fatalf("strict reload queried a retained legacy layer: %v", err)
	}
	closeMigrationTestDatabase(t, f.db)
	sqlDB, err := f.db.DB()
	if err != nil {
		t.Fatalf("get database pool: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close migrated database: %v", err)
	}
	reopened, err := repository.OpenDatabase(config.Config{SQLitePath: f.path})
	if err != nil {
		t.Fatalf("restart migrated database: %v", err)
	}
	closeMigrationTestDatabase(t, reopened)
	if !repository.IsStrictPricingActivated(reopened) {
		t.Fatal("strict marker did not survive restart")
	}
	if _, err := repository.LoadPricingSnapshot(ctx, reopened); err != nil {
		t.Fatalf("strict reload after restart: %v", err)
	}
}

func TestPricingMigrationRejectsInvalidFactsAtomically(t *testing.T) {
	f := newPricingMigrationFixture(t)
	ctx := context.Background()
	invalid := []entities.ModelPriceSetting{
		{Model: "", PromptPricePer1M: 1, CompletionPricePer1M: 1, CacheReadPricePer1M: 1, CacheWritePricePer1M: 1},
		{Model: "negative", PromptPricePer1M: -1, CompletionPricePer1M: 1, CacheReadPricePer1M: 1, CacheWritePricePer1M: 1},
		{Model: "infinite", PromptPricePer1M: math.Inf(1), CompletionPricePer1M: 1, CacheReadPricePer1M: 1, CacheWritePricePer1M: 1},
		{Model: "not-a-number", PromptPricePer1M: math.NaN(), CompletionPricePer1M: 1, CacheReadPricePer1M: 1, CacheWritePricePer1M: 1},
		{Model: "duplicate-target", PromptPricePer1M: 1, CompletionPricePer1M: 1, CacheReadPricePer1M: 1, CacheWritePricePer1M: 1},
		{Model: "DUPLICATE-TARGET", PromptPricePer1M: 1, CompletionPricePer1M: 1, CacheReadPricePer1M: 1, CacheWritePricePer1M: 1},
	}
	if err := f.db.Create(&invalid).Error; err != nil {
		t.Fatalf("seed invalid model facts: %v", err)
	}
	missingSubject := entities.CredentialModelMultiplier{SubjectID: "", Model: "legacy-model-00", Mode: pricing.ModeFixed, Multiplier: 1, PromptPricePer1M: floatPtrForTest(1), CompletionPricePer1M: floatPtrForTest(1), CacheReadPricePer1M: floatPtrForTest(1), CacheWritePricePer1M: floatPtrForTest(1)}
	missingModel := entities.CredentialModelMultiplier{SubjectID: "subject-00", Model: "", Mode: pricing.ModeFixed, Multiplier: 1, PromptPricePer1M: floatPtrForTest(1), CompletionPricePer1M: floatPtrForTest(1), CacheReadPricePer1M: floatPtrForTest(1), CacheWritePricePer1M: floatPtrForTest(1)}
	if err := f.db.Create(&[]entities.CredentialModelMultiplier{missingSubject, missingModel}).Error; err != nil {
		t.Fatalf("seed invalid credential facts: %v", err)
	}
	failed, err := repository.RunLegacyPricingMigration(ctx, f.db, repository.PricingMigrationOptions{BatchID: pricingMigrationBatchID})
	if err == nil || failed.Batch.Status != entities.PricingMigrationStatusFailed {
		t.Fatalf("invalid pricing facts did not fail safely: report=%+v err=%v", failed.Batch, err)
	}
	if repository.IsStrictPricingActivated(f.db) {
		t.Fatal("invalid migration activated strict mode")
	}
	var officialCount, matrixCount int64
	if err := f.db.Model(&entities.OfficialModelPrice{}).Count(&officialCount).Error; err != nil || officialCount != 0 {
		t.Fatalf("invalid migration partially wrote official facts: %d %v", officialCount, err)
	}
	if err := f.db.Model(&entities.CredentialModelPriceOverride{}).Count(&matrixCount).Error; err != nil || matrixCount != 0 {
		t.Fatalf("invalid migration partially wrote matrix facts: %d %v", matrixCount, err)
	}
	var oldCount int64
	if err := f.db.Model(&entities.ModelPriceSetting{}).Count(&oldCount).Error; err != nil || oldCount != 38 {
		t.Fatalf("invalid migration changed retained legacy facts: %d %v", oldCount, err)
	}
}

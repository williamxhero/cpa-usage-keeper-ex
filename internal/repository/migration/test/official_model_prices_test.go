package test

import (
	"path/filepath"
	"testing"

	"cpa-usage-keeper/internal/entities"
	. "cpa-usage-keeper/internal/repository/migration"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOfficialModelPricesMigrationSeedsLegacyPricesWithoutMultiplier(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(testSQLiteDSN(filepath.Join(t.TempDir(), "official-prices.db"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer closeOpenedDatabase(t, db)
	if err := MarkAllAsApplied(db); err != nil {
		t.Fatalf("mark migrations applied: %v", err)
	}
	if err := db.Exec("DELETE FROM schema_migrations WHERE version = ?", "20261012_official_model_prices").Error; err != nil {
		t.Fatalf("mark official prices migration pending: %v", err)
	}
	if err := db.Exec(`CREATE TABLE model_price_settings (
		id integer PRIMARY KEY,
		model text,
		pricing_style text NOT NULL DEFAULT 'openai',
		prompt_price_per1_m real,
		completion_price_per1_m real,
		cache_read_price_per1_m real,
		cache_creation_price_per1_m real NOT NULL DEFAULT 0,
		price_multiplier real NOT NULL DEFAULT 1
	)`).Error; err != nil {
		t.Fatalf("create legacy model price table: %v", err)
	}
	if err := db.Exec(`INSERT INTO model_price_settings (
		id, model, pricing_style, prompt_price_per1_m, completion_price_per1_m,
		cache_read_price_per1_m, cache_creation_price_per1_m, price_multiplier
	) VALUES (1, 'legacy-model', 'claude', 3, 15, 0.3, 3.75, 0.1)`).Error; err != nil {
		t.Fatalf("insert legacy price row: %v", err)
	}

	if err := Run(db); err != nil {
		t.Fatalf("Run migration: %v", err)
	}
	var prices []entities.OfficialModelPrice
	if err := db.Find(&prices).Error; err != nil {
		t.Fatalf("load migrated prices: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected one migrated official price, got %+v", prices)
	}
	price := prices[0]
	if price.CanonicalModelID != "legacy-model" || price.Source != entities.OfficialModelPriceSourceLegacySeed || price.PricingStyle != entities.ModelPricingStyleClaude {
		t.Fatalf("unexpected migrated metadata: %+v", price)
	}
	if price.PromptPricePer1M == nil || *price.PromptPricePer1M != 3 || price.CompletionPricePer1M == nil || *price.CompletionPricePer1M != 15 || price.CacheReadPricePer1M == nil || *price.CacheReadPricePer1M != 0.3 || price.CacheWritePricePer1M == nil || *price.CacheWritePricePer1M != 3.75 {
		t.Fatalf("migration changed legacy price or applied multiplier: %+v", price)
	}

	// Replaying the seed must preserve an administrator's later catalog edit.
	if err := db.Model(&price).Updates(map[string]any{"source": entities.OfficialModelPriceSourceOfficial, "prompt_price_per1_m": 7}).Error; err != nil {
		t.Fatalf("edit official price: %v", err)
	}
	if err := db.Exec("DELETE FROM schema_migrations WHERE version = ?", "20261012_official_model_prices").Error; err != nil {
		t.Fatalf("mark official migration pending again: %v", err)
	}
	if err := Run(db); err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	if err := db.Find(&prices).Error; err != nil {
		t.Fatalf("load replayed prices: %v", err)
	}
	if len(prices) != 1 || prices[0].ID != price.ID || prices[0].Source != entities.OfficialModelPriceSourceOfficial || prices[0].PromptPricePer1M == nil || *prices[0].PromptPricePer1M != 7 {
		t.Fatalf("replay duplicated or overwrote official price: %+v", prices)
	}
	var legacy struct {
		PromptPricePer1M     float64
		CompletionPricePer1M float64
		CacheReadPricePer1M  float64
		CacheWritePricePer1M float64 `gorm:"column:cache_creation_price_per1_m"`
		PriceMultiplier      float64
	}
	if err := db.Table("model_price_settings").Where("id = ?", 1).First(&legacy).Error; err != nil {
		t.Fatalf("read preserved legacy row: %v", err)
	}
	if legacy.PromptPricePer1M != 3 || legacy.CompletionPricePer1M != 15 || legacy.CacheReadPricePer1M != 0.3 || legacy.CacheWritePricePer1M != 3.75 || legacy.PriceMultiplier != 0.1 {
		t.Fatalf("migration modified legacy prices or multiplier: %+v", legacy)
	}
}

func TestOfficialModelPricesMigrationRollsBackInvalidLegacyBatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(testSQLiteDSN(filepath.Join(t.TempDir(), "official-prices.db"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer closeOpenedDatabase(t, db)
	if err := MarkAllAsApplied(db); err != nil {
		t.Fatalf("mark migrations applied: %v", err)
	}
	if err := db.Exec("DELETE FROM schema_migrations WHERE version = ?", "20261012_official_model_prices").Error; err != nil {
		t.Fatalf("mark official prices migration pending: %v", err)
	}
	if err := db.AutoMigrate(&entities.OfficialModelPrice{}); err != nil {
		t.Fatalf("create official prices table: %v", err)
	}
	if err := db.Exec("CREATE TABLE model_price_settings (model text, prompt_price_per1_m real)").Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if err := db.Exec("INSERT INTO model_price_settings VALUES ('a-valid', 2), ('z-invalid', -1)").Error; err != nil {
		t.Fatalf("insert legacy batch: %v", err)
	}
	if err := Run(db); err == nil {
		t.Fatal("expected invalid legacy price to reject the entire migration")
	}
	var count int64
	if err := db.Model(&entities.OfficialModelPrice{}).Count(&count).Error; err != nil {
		t.Fatalf("count official prices after rejection: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed migration published %d prices from a partial batch", count)
	}
	if err := db.Table("schema_migrations").Where("version = ?", "20261012_official_model_prices").Count(&count).Error; err != nil {
		t.Fatalf("check failed migration marker: %v", err)
	}
	if count != 0 {
		t.Fatal("failed migration was marked applied")
	}
	if err := db.Exec("UPDATE model_price_settings SET prompt_price_per1_m = 4 WHERE model = 'z-invalid'").Error; err != nil {
		t.Fatalf("repair legacy row: %v", err)
	}
	if err := Run(db); err != nil {
		t.Fatalf("retry migration after repair: %v", err)
	}
	if err := db.Model(&entities.OfficialModelPrice{}).Count(&count).Error; err != nil {
		t.Fatalf("count retried prices: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected complete retried batch, got %d prices", count)
	}
}

func TestOfficialModelPricesMigrationKeepsAbsentLegacySegmentsMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(testSQLiteDSN(filepath.Join(t.TempDir(), "official-prices.db"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer closeOpenedDatabase(t, db)
	if err := MarkAllAsApplied(db); err != nil {
		t.Fatalf("mark migrations applied: %v", err)
	}
	if err := db.Exec("DELETE FROM schema_migrations WHERE version = ?", "20261012_official_model_prices").Error; err != nil {
		t.Fatalf("mark official prices migration pending: %v", err)
	}
	if err := db.Exec("CREATE TABLE model_price_settings (model text, prompt_price_per1_m real)").Error; err != nil {
		t.Fatalf("create older legacy table: %v", err)
	}
	if err := db.Exec("INSERT INTO model_price_settings VALUES ('older-model', 0)").Error; err != nil {
		t.Fatalf("insert older legacy row: %v", err)
	}
	if err := Run(db); err != nil {
		t.Fatalf("run migration: %v", err)
	}
	var price entities.OfficialModelPrice
	if err := db.Where("canonical_model_id = ?", "older-model").First(&price).Error; err != nil {
		t.Fatalf("load migrated price: %v", err)
	}
	if price.PromptPricePer1M == nil || *price.PromptPricePer1M != 0 || price.CompletionPricePer1M != nil || price.CacheReadPricePer1M != nil || price.CacheWritePricePer1M != nil || price.MissingSegments != "completion,cache_read,cache_write" {
		t.Fatalf("migration confused explicit zero with absent legacy segments: %+v", price)
	}
}

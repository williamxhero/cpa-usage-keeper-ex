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
}

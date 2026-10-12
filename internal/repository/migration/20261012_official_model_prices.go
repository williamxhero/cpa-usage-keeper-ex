package migration

import (
	"fmt"
	"math"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// officialModelPricesMigration creates the official catalog and imports the
// legacy four-part baseline without applying the legacy multiplier column.
func officialModelPricesMigration(tx *gorm.DB) error {
	if err := tx.AutoMigrate(&entities.OfficialModelPrice{}, &entities.OfficialModelPriceAlias{}); err != nil {
		return fmt.Errorf("create official model prices tables: %w", err)
	}
	if !tx.Migrator().HasTable(&entities.ModelPriceSetting{}) {
		return nil
	}

	type legacyModelPrice struct {
		Model                string
		PricingStyle         string
		PromptPricePer1M     *float64
		CompletionPricePer1M *float64
		CacheReadPricePer1M  *float64
		CacheWritePricePer1M *float64 `gorm:"column:cache_creation_price_per1_m"`
	}
	legacyColumns := []string{"model"}
	for _, column := range []struct{ name, expression string }{
		{"pricing_style", "'' AS pricing_style"},
		{"prompt_price_per1_m", "NULL AS prompt_price_per1_m"},
		{"completion_price_per1_m", "NULL AS completion_price_per1_m"},
		{"cache_read_price_per1_m", "NULL AS cache_read_price_per1_m"},
		{"cache_creation_price_per1_m", "NULL AS cache_creation_price_per1_m"},
	} {
		if tx.Migrator().HasColumn("model_price_settings", column.name) {
			legacyColumns = append(legacyColumns, column.name)
		} else {
			legacyColumns = append(legacyColumns, column.expression)
		}
	}
	var legacy []legacyModelPrice
	if err := tx.Table("model_price_settings").Select(legacyColumns).Order("model asc").Find(&legacy).Error; err != nil {
		return fmt.Errorf("read legacy model prices: %w", err)
	}

	for _, row := range legacy {
		model := strings.TrimSpace(row.Model)
		if model == "" {
			continue
		}
		style := strings.TrimSpace(row.PricingStyle)
		if !strings.EqualFold(style, entities.ModelPricingStyleClaude) {
			style = entities.ModelPricingStyleOpenAI
		} else {
			style = entities.ModelPricingStyleClaude
		}
		price := entities.OfficialModelPrice{
			CanonicalModelID:     model,
			DisplayModelID:       model,
			Source:               entities.OfficialModelPriceSourceLegacySeed,
			PricingStyle:         style,
			PromptPricePer1M:     row.PromptPricePer1M,
			CompletionPricePer1M: row.CompletionPricePer1M,
			CacheReadPricePer1M:  row.CacheReadPricePer1M,
			CacheWritePricePer1M: row.CacheWritePricePer1M,
		}
		price.MissingSegments = officialMissingSegments(price)
		if !officialFiniteNonNegative(price) {
			return fmt.Errorf("legacy model price %q contains an invalid price", model)
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&price).Error; err != nil {
			return fmt.Errorf("seed official model price %q: %w", model, err)
		}
	}
	return nil
}

func officialMissingSegments(price entities.OfficialModelPrice) string {
	missing := make([]string, 0, 4)
	if price.PromptPricePer1M == nil {
		missing = append(missing, "prompt")
	}
	if price.CompletionPricePer1M == nil {
		missing = append(missing, "completion")
	}
	if price.CacheReadPricePer1M == nil {
		missing = append(missing, "cache_read")
	}
	if price.CacheWritePricePer1M == nil {
		missing = append(missing, "cache_write")
	}
	return strings.Join(missing, ",")
}

func officialFiniteNonNegative(price entities.OfficialModelPrice) bool {
	for _, value := range []*float64{price.PromptPricePer1M, price.CompletionPricePer1M, price.CacheReadPricePer1M, price.CacheWritePricePer1M} {
		if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return false
		}
	}
	return true
}

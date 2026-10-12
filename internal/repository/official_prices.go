package repository

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/timeutil"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

type legacyPriceForOfficialSeed struct {
	Model                string
	PricingStyle         string
	PromptPricePer1M     *float64
	CompletionPricePer1M *float64
	CacheReadPricePer1M  *float64
	CacheWritePricePer1M *float64 `gorm:"column:cache_creation_price_per1_m"`
}

var officialModelPriceColumns = []string{
	"id", "canonical_model_id", "display_model_id", "source", "pricing_style", "source_url", "fetched_at",
	"response_hash", "source_version", "prompt_price_per1_m", "completion_price_per1_m", "cache_read_price_per1_m",
	"cache_write_price_per1_m", "missing_segments", "stale", "stale_at", "created_at", "updated_at",
}

// ListOfficialModelPrices returns the persisted public baseline in deterministic order.
func ListOfficialModelPrices(db *gorm.DB) ([]entities.OfficialModelPrice, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	prices := make([]entities.OfficialModelPrice, 0)
	if err := db.Select(officialModelPriceColumns).
		Preload("Aliases", func(tx *gorm.DB) *gorm.DB { return tx.Order("alias asc") }).
		Order("canonical_model_id asc").Find(&prices).Error; err != nil {
		return nil, fmt.Errorf("list official model prices: %w", err)
	}
	return prices, nil
}

// ListOfficialPrices is the short-name compatibility alias used by pricing callers.
func ListOfficialPrices(db *gorm.DB) ([]entities.OfficialModelPrice, error) {
	return ListOfficialModelPrices(db)
}

// UpsertOfficialModelPrice atomically writes one official-price record.
func UpsertOfficialModelPrice(db *gorm.DB, input entities.OfficialModelPrice) (*entities.OfficialModelPrice, error) {
	prices, err := UpsertOfficialModelPrices(db, []entities.OfficialModelPrice{input})
	if err != nil {
		return nil, err
	}
	return &prices[0], nil
}

// UpsertOfficialModelPrices atomically upserts a batch. Nil segments remain
// missing rather than being silently converted to zero. Supplied aliases replace
// the saved aliases; nil aliases leave them unchanged.
func UpsertOfficialModelPrices(db *gorm.DB, inputs []entities.OfficialModelPrice) ([]entities.OfficialModelPrice, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	normalized := make([]entities.OfficialModelPrice, len(inputs))
	seenIDs := make(map[string]struct{}, len(inputs))
	for index := range inputs {
		price, err := normalizeOfficialModelPrice(inputs[index])
		if err != nil {
			return nil, fmt.Errorf("official model price %d: %w", index, err)
		}
		canonicalKey := strings.ToLower(price.CanonicalModelID)
		if _, exists := seenIDs[canonicalKey]; exists {
			return nil, fmt.Errorf("duplicate canonical_model_id %q in batch", price.CanonicalModelID)
		}
		seenIDs[canonicalKey] = struct{}{}
		normalized[index] = price
	}
	if len(normalized) == 0 {
		return []entities.OfficialModelPrice{}, nil
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := validateOfficialAliasConflicts(tx, normalized); err != nil {
			return err
		}
		for index := range normalized {
			input := &normalized[index]
			var existing entities.OfficialModelPrice
			err := tx.Clauses(dbresolver.Write).Where("LOWER(canonical_model_id) = LOWER(?)", input.CanonicalModelID).First(&existing).Error
			suppliedAliases := input.Aliases != nil
			if err == nil {
				input.ID = existing.ID
				input.CreatedAt = existing.CreatedAt
			} else if err != gorm.ErrRecordNotFound {
				return fmt.Errorf("load official model price %q: %w", input.CanonicalModelID, err)
			} else {
				// Canonical IDs, not caller-provided primary keys, define identity.
				// Never let an exported repository caller overwrite another row.
				input.ID = 0
				input.CreatedAt = time.Time{}
			}
			if err := tx.Omit("Aliases").Save(input).Error; err != nil {
				return fmt.Errorf("save official model price %q: %w", input.CanonicalModelID, err)
			}
			if !suppliedAliases {
				continue
			}
			if err := tx.Where("official_model_price_id = ?", input.ID).Delete(&entities.OfficialModelPriceAlias{}).Error; err != nil {
				return fmt.Errorf("replace aliases for official model price %q: %w", input.CanonicalModelID, err)
			}
			aliases := normalizeOfficialModelPriceAliases(input.Aliases, input.ID)
			if len(aliases) > 0 {
				if err := tx.CreateInBatches(&aliases, insertBatchSize(entities.OfficialModelPriceAlias{})).Error; err != nil {
					return fmt.Errorf("save aliases for official model price %q: %w", input.CanonicalModelID, err)
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return ListOfficialModelPricesByCanonicalIDs(db, normalized)
}

// UpsertOfficialPrice and UpsertOfficialPriceBatch are concise compatibility names.
func UpsertOfficialPrice(db *gorm.DB, input entities.OfficialModelPrice) (*entities.OfficialModelPrice, error) {
	return UpsertOfficialModelPrice(db, input)
}

func UpsertOfficialPriceBatch(db *gorm.DB, inputs []entities.OfficialModelPrice) ([]entities.OfficialModelPrice, error) {
	return UpsertOfficialModelPrices(db, inputs)
}

func ListOfficialModelPricesByCanonicalIDs(db *gorm.DB, inputs []entities.OfficialModelPrice) ([]entities.OfficialModelPrice, error) {
	ids := make([]string, 0, len(inputs))
	for _, input := range inputs {
		ids = append(ids, input.CanonicalModelID)
	}
	var prices []entities.OfficialModelPrice
	if err := db.Select(officialModelPriceColumns).
		Preload("Aliases", func(tx *gorm.DB) *gorm.DB { return tx.Order("alias asc") }).
		Where("canonical_model_id IN ?", ids).Order("canonical_model_id asc").Find(&prices).Error; err != nil {
		return nil, fmt.Errorf("load official model prices: %w", err)
	}
	return prices, nil
}

// SeedOfficialModelPrices imports the legacy four-part prices without reading or
// applying model_price_settings.price_multiplier. Existing official rows win.
func SeedOfficialModelPrices(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if !db.Migrator().HasTable(&entities.ModelPriceSetting{}) || !db.Migrator().HasTable(&entities.OfficialModelPrice{}) {
		return nil
	}
	var legacy []legacyPriceForOfficialSeed
	if err := db.Table("model_price_settings").Select("model", "pricing_style", "prompt_price_per1_m", "completion_price_per1_m", "cache_read_price_per1_m", "cache_creation_price_per1_m").Order("model asc").Find(&legacy).Error; err != nil {
		return fmt.Errorf("list legacy model prices for official seed: %w", err)
	}
	return seedLegacyOfficialModelPrices(db, legacy)
}

func seedLegacyOfficialModelPrices(db *gorm.DB, legacy []legacyPriceForOfficialSeed) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, row := range legacy {
			model := strings.TrimSpace(row.Model)
			if model == "" {
				continue
			}
			var count int64
			if err := tx.Model(&entities.OfficialModelPrice{}).Where("canonical_model_id = ?", model).Count(&count).Error; err != nil {
				return fmt.Errorf("check official seed %q: %w", model, err)
			}
			if count > 0 {
				continue
			}
			seed := entities.OfficialModelPrice{
				CanonicalModelID:     model,
				DisplayModelID:       model,
				Source:               entities.OfficialModelPriceSourceLegacySeed,
				PricingStyle:         normalizeSeedPricingStyle(row.PricingStyle),
				PromptPricePer1M:     row.PromptPricePer1M,
				CompletionPricePer1M: row.CompletionPricePer1M,
				CacheReadPricePer1M:  row.CacheReadPricePer1M,
				CacheWritePricePer1M: row.CacheWritePricePer1M,
			}
			seed.MissingSegments = missingOfficialModelPriceSegments(seed)
			if !finiteNonNegativePrices(seed) {
				return fmt.Errorf("legacy model price %q contains an invalid price", model)
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "canonical_model_id"}}, DoNothing: true}).Create(&seed).Error; err != nil {
				return fmt.Errorf("seed official model price %q: %w", model, err)
			}
		}
		return nil
	})
}

func normalizeSeedPricingStyle(style string) string {
	if strings.EqualFold(strings.TrimSpace(style), entities.ModelPricingStyleClaude) {
		return entities.ModelPricingStyleClaude
	}
	return entities.ModelPricingStyleOpenAI
}

func SeedOfficialPrices(db *gorm.DB) error { return SeedOfficialModelPrices(db) }

// MarkOfficialModelPricesStale marks requested models stale. An empty list marks all.
func MarkOfficialModelPricesStale(db *gorm.DB, models []string) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	query := db.Model(&entities.OfficialModelPrice{})
	if normalized := normalizeModelIDs(models); len(normalized) > 0 {
		query = query.Where("canonical_model_id IN ?", normalized)
	} else {
		query = query.Where("1 = 1")
	}
	if err := query.Updates(map[string]any{"stale": true, "stale_at": timeutil.NormalizeStorageTime(time.Now())}).Error; err != nil {
		return fmt.Errorf("mark official model prices stale: %w", err)
	}
	return nil
}

func MarkOfficialPricesStale(db *gorm.DB, models []string) error {
	return MarkOfficialModelPricesStale(db, models)
}

// RestoreOfficialModelPrices clears stale state for requested models. An empty list clears all.
func RestoreOfficialModelPrices(db *gorm.DB, models []string) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	query := db.Model(&entities.OfficialModelPrice{})
	if normalized := normalizeModelIDs(models); len(normalized) > 0 {
		query = query.Where("canonical_model_id IN ?", normalized)
	} else {
		query = query.Where("1 = 1")
	}
	if err := query.Updates(map[string]any{"stale": false, "stale_at": nil}).Error; err != nil {
		return fmt.Errorf("restore official model prices: %w", err)
	}
	return nil
}

func normalizeOfficialModelPrice(input entities.OfficialModelPrice) (entities.OfficialModelPrice, error) {
	input.CanonicalModelID = strings.TrimSpace(input.CanonicalModelID)
	if input.CanonicalModelID == "" {
		return entities.OfficialModelPrice{}, fmt.Errorf("canonical_model_id is required")
	}
	input.DisplayModelID = strings.TrimSpace(input.DisplayModelID)
	if input.DisplayModelID == "" {
		input.DisplayModelID = input.CanonicalModelID
	}
	input.Source = strings.ToLower(strings.TrimSpace(input.Source))
	if input.Source == "" {
		input.Source = entities.OfficialModelPriceSourceOfficial
	}
	if input.Source != entities.OfficialModelPriceSourceOfficial && input.Source != entities.OfficialModelPriceSourceModelsDev && input.Source != entities.OfficialModelPriceSourceLiteLLM && input.Source != entities.OfficialModelPriceSourceOpenRouter && input.Source != entities.OfficialModelPriceSourceLegacySeed {
		return entities.OfficialModelPrice{}, fmt.Errorf("source is not supported")
	}
	input.PricingStyle = strings.ToLower(strings.TrimSpace(input.PricingStyle))
	if input.PricingStyle == "" {
		input.PricingStyle = entities.ModelPricingStyleOpenAI
	}
	if input.PricingStyle != entities.ModelPricingStyleOpenAI && input.PricingStyle != entities.ModelPricingStyleClaude {
		return entities.OfficialModelPrice{}, fmt.Errorf("pricing_style must be openai or claude")
	}
	if !finiteNonNegativePrices(input) {
		return entities.OfficialModelPrice{}, fmt.Errorf("prices must be finite and non-negative")
	}
	input.MissingSegments = missingOfficialModelPriceSegments(input)
	return input, nil
}

func finiteNonNegativePrices(input entities.OfficialModelPrice) bool {
	for _, price := range []*float64{input.PromptPricePer1M, input.CompletionPricePer1M, input.CacheReadPricePer1M, input.CacheWritePricePer1M} {
		if price != nil && (*price < 0 || math.IsNaN(*price) || math.IsInf(*price, 0)) {
			return false
		}
	}
	return true
}

func missingOfficialModelPriceSegments(input entities.OfficialModelPrice) string {
	missing := make([]string, 0, 4)
	if input.PromptPricePer1M == nil {
		missing = append(missing, "prompt")
	}
	if input.CompletionPricePer1M == nil {
		missing = append(missing, "completion")
	}
	if input.CacheReadPricePer1M == nil {
		missing = append(missing, "cache_read")
	}
	if input.CacheWritePricePer1M == nil {
		missing = append(missing, "cache_write")
	}
	return strings.Join(missing, ",")
}

func validateOfficialAliasConflicts(db *gorm.DB, inputs []entities.OfficialModelPrice) error {
	prices, err := ListOfficialModelPrices(db)
	if err != nil {
		return err
	}
	updating := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		updating[strings.ToLower(input.CanonicalModelID)] = struct{}{}
	}
	owners := make(map[string]string)
	for _, price := range prices {
		canonical := strings.ToLower(strings.TrimSpace(price.CanonicalModelID))
		if _, replaced := updating[canonical]; replaced {
			continue
		}
		if canonical != "" {
			owners[canonical] = price.CanonicalModelID
		}
		for _, alias := range price.Aliases {
			key := strings.ToLower(strings.TrimSpace(alias.Alias))
			if key != "" {
				owners[key] = price.CanonicalModelID
			}
		}
	}
	for _, input := range inputs {
		canonical := strings.ToLower(input.CanonicalModelID)
		owners[canonical] = input.CanonicalModelID
	}
	for _, input := range inputs {
		for _, alias := range normalizeOfficialModelPriceAliases(input.Aliases, 0) {
			key := strings.ToLower(alias.Alias)
			if owner, exists := owners[key]; exists && !strings.EqualFold(owner, input.CanonicalModelID) {
				return fmt.Errorf("alias %q conflicts with model %q", alias.Alias, owner)
			}
			owners[key] = input.CanonicalModelID
		}
	}
	return nil
}

func normalizeOfficialModelPriceAliases(input []entities.OfficialModelPriceAlias, priceID int64) []entities.OfficialModelPriceAlias {
	seen := make(map[string]struct{}, len(input))
	aliases := make([]entities.OfficialModelPriceAlias, 0, len(input))
	for _, alias := range input {
		name := strings.TrimSpace(alias.Alias)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		aliases = append(aliases, entities.OfficialModelPriceAlias{OfficialModelPriceID: priceID, Alias: name})
	}
	sort.Slice(aliases, func(i, j int) bool { return aliases[i].Alias < aliases[j].Alias })
	return aliases
}

func normalizeModelIDs(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	sort.Strings(result)
	return result
}

func floatPointer(value float64) *float64 { return &value }

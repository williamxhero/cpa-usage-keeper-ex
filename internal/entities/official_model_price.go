package entities

import "time"

const (
	OfficialModelPriceSourceOfficial   = "official"
	OfficialModelPriceSourceModelsDev  = "models-dev"
	OfficialModelPriceSourceLiteLLM    = "litellm"
	OfficialModelPriceSourceOpenRouter = "openrouter"
	OfficialModelPriceSourceLegacySeed = "legacy-seed"
)

// OfficialModelPrice is the persisted public baseline for one canonical model.
// A nil segment means that the source did not publish that price; it must not be
// interpreted as a free segment.
type OfficialModelPrice struct {
	ID                   int64  `gorm:"primaryKey"`
	CanonicalModelID     string `gorm:"not null;uniqueIndex:uniq_official_model_prices_canonical_model_id"`
	DisplayModelID       string `gorm:"not null"`
	Source               string `gorm:"not null"`
	PricingStyle         string `gorm:"not null;default:openai"`
	SourceURL            string
	FetchedAt            *time.Time `gorm:"serializer:storageTime"`
	ResponseHash         string
	SourceVersion        string
	PromptPricePer1M     *float64
	CompletionPricePer1M *float64
	CacheReadPricePer1M  *float64
	CacheWritePricePer1M *float64
	// MissingSegments is a stable comma-separated list of missing segment names.
	// The pointer prices remain the source of truth; this field is metadata for
	// safe display and refresh diagnostics.
	MissingSegments string     `gorm:"not null;default:''"`
	Stale           bool       `gorm:"not null;default:false;index"`
	StaleAt         *time.Time `gorm:"serializer:storageTime"`
	CreatedAt       time.Time  `gorm:"serializer:storageTime"`
	UpdatedAt       time.Time  `gorm:"serializer:storageTime"`

	Aliases []OfficialModelPriceAlias `gorm:"foreignKey:OfficialModelPriceID;references:ID"`
}

// OfficialModelPriceAlias is an explicitly registered alternate model ID.
// Aliases are never inferred from substrings or provider/model components.
type OfficialModelPriceAlias struct {
	ID                   int64     `gorm:"primaryKey"`
	OfficialModelPriceID int64     `gorm:"not null;uniqueIndex:uniq_official_model_price_aliases_identity,priority:1"`
	Alias                string    `gorm:"not null;uniqueIndex:uniq_official_model_price_aliases_identity,priority:2;uniqueIndex:uniq_official_model_price_aliases_alias"`
	CreatedAt            time.Time `gorm:"serializer:storageTime"`
	UpdatedAt            time.Time `gorm:"serializer:storageTime"`
}

// CanonicalModel and DisplayModel are compatibility helpers for callers that
// use the shorter names from the pricing domain. They are not extra columns.
func (p OfficialModelPrice) CanonicalModel() string { return p.CanonicalModelID }
func (p OfficialModelPrice) DisplayModel() string   { return p.DisplayModelID }

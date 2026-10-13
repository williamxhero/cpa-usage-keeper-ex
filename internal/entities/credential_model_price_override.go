package entities

import "time"

// CredentialModelPriceOverride is the sparse, Keeper-owned credential/model
// matrix. A row is always a complete fixed tariff; an absent row means inherit.
type CredentialModelPriceOverride struct {
	SubjectID            string    `gorm:"primaryKey"`
	CanonicalModelID     string    `gorm:"primaryKey"`
	PricingStyle         string    `gorm:"not null;default:openai"`
	PromptPricePer1M     float64   `gorm:"not null"`
	CompletionPricePer1M float64   `gorm:"not null"`
	CacheReadPricePer1M  float64   `gorm:"not null"`
	CacheWritePricePer1M float64   `gorm:"not null"`
	Source               string    `gorm:"not null;default:credential_matrix"`
	UpdatedAt            time.Time `gorm:"serializer:storageTime"`
}

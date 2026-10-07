package entities

import "time"

// CredentialModelMultiplier retains the original table/name for compatibility.
// One subject/model row holds either a multiplier or complete fixed tariff;
// removing it restores inheritance. Inactive mode fields never participate.
type CredentialModelMultiplier struct {
	SubjectID            string  `gorm:"primaryKey"`
	Model                string  `gorm:"primaryKey"`
	Multiplier           float64 `gorm:"not null"`
	Mode                 string  `gorm:"not null;default:multiplier"`
	PromptPricePer1M     *float64
	CompletionPricePer1M *float64
	CacheReadPricePer1M  *float64
	CacheWritePricePer1M *float64
	PricingStyle         string
	UpdatedAt            time.Time `gorm:"serializer:storageTime"`
}

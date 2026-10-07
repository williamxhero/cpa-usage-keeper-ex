package entities

import "time"

// One channel/model row contains exactly one active pricing mode.
type ChannelModelPrice struct {
	ChannelID            string  `gorm:"primaryKey"`
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

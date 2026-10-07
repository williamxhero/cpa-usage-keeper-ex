package migration

import (
	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

func pricingChannelsMigration(tx *gorm.DB) error {
	return tx.AutoMigrate(&entities.PricingChannel{}, &entities.PricingChannelMember{}, &entities.ChannelPriceDefault{})
}

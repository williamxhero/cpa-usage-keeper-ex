package migration

import (
	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

func channelModelPricesMigration(tx *gorm.DB) error {
	return tx.AutoMigrate(&entities.ChannelModelPrice{})
}

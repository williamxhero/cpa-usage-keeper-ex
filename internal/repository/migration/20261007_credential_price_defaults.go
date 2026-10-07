package migration

import (
	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

func credentialPriceDefaultsMigration(tx *gorm.DB) error {
	return tx.AutoMigrate(&entities.CredentialPriceDefault{})
}

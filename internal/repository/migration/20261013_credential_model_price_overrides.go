package migration

import (
	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

func credentialModelPriceOverridesMigration(tx *gorm.DB) error {
	return tx.AutoMigrate(&entities.CredentialModelPriceOverride{})
}

package migration

import (
	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

func credentialModelMultipliersMigration(tx *gorm.DB) error {
	return tx.AutoMigrate(&entities.CredentialModelMultiplier{})
}

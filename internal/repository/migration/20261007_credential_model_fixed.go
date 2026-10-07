package migration

import (
	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

// Extend the same unique subject/model row; existing rows default to multiplier.
func credentialModelFixedMigration(tx *gorm.DB) error {
	return tx.AutoMigrate(&entities.CredentialModelMultiplier{})
}

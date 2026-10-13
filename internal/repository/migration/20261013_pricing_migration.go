package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

// pricingMigrationSchemaMigration creates durable batch/report tables. Data
// adoption is explicit and is not performed during service startup.
func pricingMigrationSchemaMigration(tx *gorm.DB) error {
	if err := tx.AutoMigrate(
		&entities.PricingMigrationBatch{},
		&entities.PricingMigrationBatchCount{},
		&entities.PricingMigrationItem{},
		&entities.PricingMigrationLegacyState{},
	); err != nil {
		return fmt.Errorf("create pricing migration report tables: %w", err)
	}
	return nil
}

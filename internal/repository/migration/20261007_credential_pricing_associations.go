package migration

import (
	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

func credentialPricingAssociationsMigration(tx *gorm.DB) error {
	// Original relations remain in place; the loader provides their compatible
	// virtual associations. This schema migration does not rewrite/backfill events.
	return tx.AutoMigrate(&entities.CredentialPricingAssociation{})
}

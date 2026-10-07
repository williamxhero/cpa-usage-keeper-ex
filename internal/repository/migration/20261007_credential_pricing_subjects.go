package migration

import (
	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

func credentialPricingSubjectsMigration(tx *gorm.DB) error {
	if !tx.Migrator().HasColumn(&entities.UsageIdentity{}, "BindingIdentityStatus") {
		if err := tx.Migrator().AddColumn(&entities.UsageIdentity{}, "BindingIdentityStatus"); err != nil {
			return err
		}
	}
	return tx.AutoMigrate(&entities.CredentialPricingSubject{})
}

package repository

import (
	"context"
	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ReadCredentialPricingDirectory observes directory metadata and saved relations
// in one transaction. Do not log SQL arguments: the stored original identity is internal.
func ReadCredentialPricingDirectory(ctx context.Context, db *gorm.DB, read func([]entities.UsageIdentity, []entities.CredentialPricingSubject) error) error {
	return db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Transaction(func(tx *gorm.DB) error {
		identities, subjects, err := LoadCredentialPricingDirectory(tx)
		if err != nil {
			return err
		}
		return read(identities, subjects)
	})
}

func LoadCredentialPricingDirectory(tx *gorm.DB) ([]entities.UsageIdentity, []entities.CredentialPricingSubject, error) {
	identities, err := ListUsageIdentities(tx.Statement.Context, tx)
	if err != nil {
		return nil, nil, err
	}
	var subjects []entities.CredentialPricingSubject
	if err := tx.Order("created_at asc, id asc").Find(&subjects).Error; err != nil {
		return nil, nil, err
	}
	return identities, subjects, nil
}

func SaveCredentialPricingSubject(ctx context.Context, db *gorm.DB, validate func([]entities.UsageIdentity, []entities.CredentialPricingSubject) (*entities.CredentialPricingSubject, error)) error {
	return db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Transaction(func(tx *gorm.DB) error {
		identities, subjects, err := LoadCredentialPricingDirectory(tx)
		if err != nil {
			return err
		}
		subject, err := validate(identities, subjects)
		if err != nil {
			return err
		}
		return tx.Create(subject).Error
	})
}

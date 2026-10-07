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
	associations, err := loadCredentialPricingAssociations(tx, subjects)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]entities.CredentialPricingSubject, len(subjects))
	for _, subject := range subjects {
		byID[subject.ID] = subject
	}
	// Originals remain sanitization/existence evidence, not effective bindings.
	result := make([]entities.CredentialPricingSubject, 0, len(subjects)+len(associations))
	for _, subject := range subjects {
		subject.BindingDisabled = true
		result = append(result, subject)
	}
	for _, association := range associations {
		subject, found := byID[association.SubjectID]
		if !found {
			return nil, nil, ErrInvalidPricingSnapshot
		}
		// Disabled relations still provide private sanitization evidence, never
		// active ownership. A missing directory must not expose their exact index.
		subject.BindingDisabled = !association.Enabled
		subject.AuthType, subject.AuthTypeName, subject.Identity = association.AuthType, association.AuthTypeName, association.Identity
		subject.UsageIdentityID = association.UsageIdentityID
		subject.BindingRef = association.ID
		result = append(result, subject)
	}
	return identities, result, nil
}

type credentialAssociationKey struct {
	AuthType entities.UsageIdentityAuthType
	Identity string
}

// LoadCredentialPricingAssociations includes virtual legacy origins until an
// explicit correction shadows that exact pair. References contain no identity.
func LoadCredentialPricingAssociations(tx *gorm.DB) ([]entities.CredentialPricingAssociation, error) {
	var subjects []entities.CredentialPricingSubject
	if err := tx.Order("created_at asc, id asc").Find(&subjects).Error; err != nil {
		return nil, err
	}
	return loadCredentialPricingAssociations(tx, subjects)
}

func loadCredentialPricingAssociations(tx *gorm.DB, subjects []entities.CredentialPricingSubject) ([]entities.CredentialPricingAssociation, error) {
	var stored []entities.CredentialPricingAssociation
	if err := tx.Order("created_at asc, id asc").Find(&stored).Error; err != nil {
		return nil, err
	}
	seen := map[credentialAssociationKey]bool{}
	for _, association := range stored {
		seen[credentialAssociationKey{association.AuthType, association.Identity}] = true
	}
	result := make([]entities.CredentialPricingAssociation, 0, len(subjects)+len(stored))
	for _, subject := range subjects {
		if !seen[credentialAssociationKey{subject.AuthType, subject.Identity}] {
			result = append(result, entities.CredentialPricingAssociation{ID: "binding_" + subject.ID, SubjectID: subject.ID, UsageIdentityID: subject.UsageIdentityID, AuthType: subject.AuthType, AuthTypeName: subject.AuthTypeName, Identity: subject.Identity, Enabled: true, CreatedAt: subject.CreatedAt})
		}
	}
	return append(result, stored...), nil
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

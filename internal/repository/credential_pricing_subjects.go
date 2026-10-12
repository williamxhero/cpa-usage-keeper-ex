package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

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

// EnsureCredentialPricingSubjects creates Keeper-owned opaque subjects for every
// currently observed, uniquely attributable CPA identity. It deliberately uses
// only the exact auth type/index relation; presentation metadata and secrets are
// never used as identity evidence. The caller should invoke it inside the same
// transaction that publishes a successful metadata scope.
func EnsureCredentialPricingSubjects(tx *gorm.DB, now time.Time) error {
	if tx == nil {
		return fmt.Errorf("database is nil")
	}
	if now.IsZero() {
		return fmt.Errorf("credential pricing subject sync time is zero")
	}

	var identities []entities.UsageIdentity
	if err := tx.Where("is_deleted = ? AND binding_identity_status = ?", false, "unique").
		Order("id asc").Find(&identities).Error; err != nil {
		return fmt.Errorf("list uniquely attributable identities: %w", err)
	}
	var subjects []entities.CredentialPricingSubject
	if err := tx.Order("created_at asc, id asc").Find(&subjects).Error; err != nil {
		return fmt.Errorf("list credential pricing subjects: %w", err)
	}
	var associations []entities.CredentialPricingAssociation
	if err := tx.Order("created_at asc, id asc").Find(&associations).Error; err != nil {
		return fmt.Errorf("list credential pricing associations: %w", err)
	}

	type relation struct {
		authType entities.UsageIdentityAuthType
		identity string
	}
	// Original subject relations remain the exact historical evidence. Enabled
	// overlays can also account for a relation when they point at an existing
	// subject; disabled historical overlays must not suppress auto-discovery.
	existing := make(map[relation]struct{}, len(subjects)+len(associations))
	subjectIDs := make(map[string]struct{}, len(subjects))
	for _, subject := range subjects {
		subjectIDs[subject.ID] = struct{}{}
		if subject.Identity != "" {
			existing[relation{authType: subject.AuthType, identity: subject.Identity}] = struct{}{}
		}
	}
	for _, association := range associations {
		if !association.Enabled {
			continue
		}
		if _, found := subjectIDs[association.SubjectID]; found {
			existing[relation{authType: association.AuthType, identity: association.Identity}] = struct{}{}
		}
	}

	for _, identity := range identities {
		rel := relation{authType: identity.AuthType, identity: identity.Identity}
		if rel.identity == "" || (identity.AuthType != entities.UsageIdentityAuthTypeAuthFile && identity.AuthType != entities.UsageIdentityAuthTypeAIProvider) {
			continue
		}
		if _, found := existing[rel]; found {
			continue
		}
		id, err := newCredentialPricingSubjectID()
		if err != nil {
			return err
		}
		subject := entities.CredentialPricingSubject{
			ID:              id,
			UsageIdentityID: identity.ID,
			AuthType:        identity.AuthType,
			AuthTypeName:    identity.AuthTypeName,
			Identity:        identity.Identity,
			CreatedAt:       now,
		}
		if err := tx.Create(&subject).Error; err != nil {
			return fmt.Errorf("create credential pricing subject: %w", err)
		}
		existing[rel] = struct{}{}
	}
	return nil
}

func newCredentialPricingSubjectID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate credential pricing subject id: %w", err)
	}
	return "cred_" + hex.EncodeToString(bytes), nil
}

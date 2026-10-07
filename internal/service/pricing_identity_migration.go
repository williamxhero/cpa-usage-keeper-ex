package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var ErrPricingBindingConfirmation = errors.New("explicit binding confirmation required")
var ErrPricingBindingStale = errors.New("binding selection changed; refresh and confirm again")

type PricingIdentitySelection struct {
	Ref        string                       `json:"ref"`
	Credential servicedto.PricingCredential `json:"credential"`
}
type PricingIdentityBinding struct {
	Ref        string                       `json:"ref"`
	SubjectID  string                       `json:"subject_id"`
	Enabled    bool                         `json:"enabled"`
	Credential servicedto.PricingCredential `json:"credential"`
}
type PricingIdentityState struct {
	SnapshotID string                         `json:"snapshot_id"`
	Subjects   []servicedto.PricingCredential `json:"subjects"`
	Directory  []PricingIdentitySelection     `json:"directory"`
	Bindings   []PricingIdentityBinding       `json:"bindings"`
}
type PricingIdentityMigrationInput struct {
	SubjectID    string `json:"subject_id"`
	DirectoryRef string `json:"directory_ref"`
	SnapshotID   string `json:"snapshot_id"`
	Confirmed    bool   `json:"confirmed"`
}
type PricingIdentityCorrectionInput struct {
	BindingRef        string `json:"-"`
	ExpectedSubjectID string `json:"expected_subject_id"`
	TargetSubjectID   string `json:"target_subject_id"`
	Action            string `json:"action"`
	SnapshotID        string `json:"snapshot_id"`
	Confirmed         bool   `json:"confirmed"`
}
type PricingIdentityMutationResult struct {
	BindingRef string `json:"binding_ref"`
	SubjectID  string `json:"subject_id"`
	Enabled    bool   `json:"enabled"`
	SnapshotID string `json:"snapshot_id"`
}
type PricingIdentityMigrationProvider interface {
	GetPricingIdentityState(context.Context) (PricingIdentityState, error)
	MigratePricingIdentity(context.Context, PricingIdentityMigrationInput) (PricingIdentityMutationResult, error)
	CorrectPricingIdentity(context.Context, PricingIdentityCorrectionInput) (PricingIdentityMutationResult, error)
}

// Directory references are scoped to a published snapshot and existing directory
// row, not derived from credentials and not promised as permanent upstream IDs.
func pricingDirectoryRef(snapshotID string, directoryID int64) string {
	sum := sha256.Sum256([]byte(snapshotID + "|directory|" + strconv.FormatInt(directoryID, 10)))
	return "selection_" + hex.EncodeToString(sum[:])
}

func (s *pricingService) GetPricingIdentityState(ctx context.Context) (PricingIdentityState, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	snapshot := s.catalog.Snapshot()
	result := PricingIdentityState{SnapshotID: snapshot.ID(), Directory: []PricingIdentitySelection{}, Bindings: []PricingIdentityBinding{}}
	err := s.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Transaction(func(tx *gorm.DB) error {
		identities, subjects, err := repository.LoadCredentialPricingDirectory(tx)
		if err != nil {
			return err
		}
		associations, err := repository.LoadCredentialPricingAssociations(tx)
		if err != nil {
			return err
		}
		for _, identity := range identities {
			result.Directory = append(result.Directory, PricingIdentitySelection{Ref: pricingDirectoryRef(snapshot.ID(), identity.ID), Credential: pricingCredential(identity, subjects)})
		}
		for _, association := range associations {
			item := servicedto.PricingCredential{SubjectID: association.SubjectID, Name: snapshot.CredentialName(association.SubjectID), AuthType: pricingAssociationAuthType(association), ProviderType: "unknown", Status: "stale", BindingStatus: "stale"}
			for _, identity := range identities {
				if identity.AuthType == association.AuthType && identity.Identity == association.Identity {
					item = pricingCredential(identity, subjects)
				}
			}
			item.SubjectID = association.SubjectID
			if !association.Enabled {
				item.BindingStatus = "unbound"
			}
			result.Bindings = append(result.Bindings, PricingIdentityBinding{Ref: association.ID, SubjectID: association.SubjectID, Enabled: association.Enabled, Credential: item})
		}
		return nil
	})
	if err != nil {
		return PricingIdentityState{}, err
	}
	result.Subjects, err = s.ListCredentialPricingSubjects(ctx)
	return result, err
}

func pricingAssociationAuthType(association entities.CredentialPricingAssociation) string {
	if association.AuthType == entities.UsageIdentityAuthTypeAuthFile && association.AuthTypeName == "oauth" {
		return "oauth"
	}
	if association.AuthType == entities.UsageIdentityAuthTypeAIProvider && association.AuthTypeName == "apikey" {
		return "apikey"
	}
	return "unknown"
}

func requirePricingBindingSnapshot(s *pricingService, id string, confirmed bool) error {
	if !confirmed {
		return ErrPricingBindingConfirmation
	}
	if id == "" || id != s.catalog.Snapshot().ID() {
		return ErrPricingBindingStale
	}
	return nil
}
func requirePricingSubject(tx *gorm.DB, id string) error {
	var count int64
	if err := tx.Model(&entities.CredentialPricingSubject{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrPricingCredentialNotFound
	}
	return nil
}

func (s *pricingService) MigratePricingIdentity(ctx context.Context, input PricingIdentityMigrationInput) (PricingIdentityMutationResult, error) {
	result := PricingIdentityMutationResult{SubjectID: input.SubjectID, Enabled: true}
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		tx = tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		if err := requirePricingBindingSnapshot(s, input.SnapshotID, input.Confirmed); err != nil {
			return err
		}
		if err := requirePricingSubject(tx, input.SubjectID); err != nil {
			return err
		}
		identities, subjects, err := repository.LoadCredentialPricingDirectory(tx)
		if err != nil {
			return err
		}
		associations, err := repository.LoadCredentialPricingAssociations(tx)
		if err != nil {
			return err
		}
		for _, identity := range identities {
			if pricingDirectoryRef(input.SnapshotID, identity.ID) != input.DirectoryRef {
				continue
			}
			if identity.IsDeleted || pricingCredential(identity, subjects).BindingStatus != "unbound" {
				return ErrCredentialNotSelectable
			}
			// Duplicated directory claims remain conflicting even when they would point
			// at the same subject; selecting a first row does not prove upstream identity.
			for _, other := range identities {
				if other.ID != identity.ID && other.AuthType == identity.AuthType && other.Identity == identity.Identity {
					return ErrCredentialBindingConflict
				}
			}
			for _, association := range associations {
				if association.AuthType == identity.AuthType && association.Identity == identity.Identity {
					return ErrCredentialBindingConflict
				}
			}
			bytes := make([]byte, 16)
			if _, err := rand.Read(bytes); err != nil {
				return err
			}
			association := entities.CredentialPricingAssociation{ID: "binding_" + hex.EncodeToString(bytes), SubjectID: input.SubjectID, UsageIdentityID: identity.ID, AuthType: identity.AuthType, AuthTypeName: identity.AuthTypeName, Identity: identity.Identity, Enabled: true}
			result.BindingRef = association.ID
			return tx.Create(&association).Error
		}
		return ErrCredentialNotSelectable
	})
	if err != nil {
		return PricingIdentityMutationResult{}, err
	}
	result.SnapshotID = snapshot.ID()
	return result, nil
}

func (s *pricingService) CorrectPricingIdentity(ctx context.Context, input PricingIdentityCorrectionInput) (PricingIdentityMutationResult, error) {
	if (input.Action != "unbind" && input.Action != "rebind") || (input.Action == "unbind" && input.TargetSubjectID != "") || (input.Action == "rebind" && input.TargetSubjectID == "") {
		return PricingIdentityMutationResult{}, ErrInvalidPricingInput
	}
	var result PricingIdentityMutationResult
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		tx = tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		if err := requirePricingBindingSnapshot(s, input.SnapshotID, input.Confirmed); err != nil {
			return err
		}
		associations, err := repository.LoadCredentialPricingAssociations(tx)
		if err != nil {
			return err
		}
		for _, association := range associations {
			if association.ID != input.BindingRef {
				continue
			}
			if input.ExpectedSubjectID == "" || association.SubjectID != input.ExpectedSubjectID {
				return ErrPricingBindingStale
			}
			if input.Action == "unbind" {
				if !association.Enabled {
					return ErrCredentialBindingConflict
				}
				association.Enabled = false
			} else {
				if association.Enabled && association.SubjectID == input.TargetSubjectID {
					return ErrCredentialBindingConflict
				}
				if err := requirePricingSubject(tx, input.TargetSubjectID); err != nil {
					return err
				}
				identities, subjects, err := repository.LoadCredentialPricingDirectory(tx)
				if err != nil {
					return err
				}
				matches := 0
				for _, identity := range identities {
					if identity.AuthType == association.AuthType && identity.Identity == association.Identity {
						matches++
						status := pricingCredential(identity, subjects).BindingStatus
						if status == "ambiguous" || status == "unknown" {
							return ErrCredentialNotSelectable
						}
					}
				}
				if matches > 1 || pricingAssociationAuthType(association) == "unknown" {
					return ErrCredentialNotSelectable
				}
				association.SubjectID, association.Enabled = input.TargetSubjectID, true
			}
			// Persist the correction as an overlay, including explicit disabled origins.
			// Never overwrite/delete the original subject relation, prices or members.
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"subject_id", "enabled"})}).Create(&association).Error; err != nil {
				return err
			}
			result = PricingIdentityMutationResult{BindingRef: association.ID, SubjectID: association.SubjectID, Enabled: association.Enabled}
			return nil
		}
		return ErrCredentialNotSelectable
	})
	if err != nil {
		return PricingIdentityMutationResult{}, err
	}
	result.SnapshotID = snapshot.ID()
	return result, nil
}

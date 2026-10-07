package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CredentialModelMultiplier struct {
	SubjectID  string   `json:"subject_id"`
	Model      string   `json:"model"`
	Multiplier *float64 `json:"multiplier"`
	SnapshotID string   `json:"snapshot_id"`
}

type CredentialModelMultipliers struct {
	SubjectID  string                      `json:"subject_id"`
	Models     []CredentialModelMultiplier `json:"models"`
	SnapshotID string                      `json:"snapshot_id"`
}

type PricingCredentialModelsProvider interface {
	ListCredentialModels(context.Context, string) (CredentialModelMultipliers, error)
	GetCredentialModel(context.Context, string, string) (CredentialModelMultiplier, error)
	SetCredentialModel(context.Context, string, string, string) (CredentialModelMultiplier, error)
	ClearCredentialModel(context.Context, string, string) (CredentialModelMultiplier, error)
}

func credentialModelDTO(snapshot *pricing.Snapshot, id, model string) (CredentialModelMultiplier, error) {
	if !snapshot.HasCredentialSubject(id) {
		return CredentialModelMultiplier{}, ErrPricingCredentialNotFound
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return CredentialModelMultiplier{}, fmt.Errorf("%w: model is required", ErrInvalidPricingInput)
	}
	result := CredentialModelMultiplier{SubjectID: id, Model: model, SnapshotID: snapshot.ID()}
	if value, exists := snapshot.CredentialModel(id, model); exists {
		result.Multiplier = &value
	}
	return result, nil
}

func (s *pricingService) ListCredentialModels(_ context.Context, id string) (CredentialModelMultipliers, error) {
	snapshot := s.catalog.Snapshot()
	if !snapshot.HasCredentialSubject(id) {
		return CredentialModelMultipliers{}, ErrPricingCredentialNotFound
	}
	result := CredentialModelMultipliers{SubjectID: id, Models: []CredentialModelMultiplier{}, SnapshotID: snapshot.ID()}
	for _, config := range snapshot.CredentialModelConfigs(id) {
		value := config.Multiplier
		result.Models = append(result.Models, CredentialModelMultiplier{SubjectID: id, Model: config.Model, Multiplier: &value, SnapshotID: snapshot.ID()})
	}
	return result, nil
}

func (s *pricingService) GetCredentialModel(_ context.Context, id, model string) (CredentialModelMultiplier, error) {
	return credentialModelDTO(s.catalog.Snapshot(), id, model)
}

func (s *pricingService) SetCredentialModel(ctx context.Context, id, model, text string) (CredentialModelMultiplier, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return CredentialModelMultiplier{}, fmt.Errorf("%w: model is required", ErrInvalidPricingInput)
	}
	value, err := pricing.ParseMultiplier(text)
	if err != nil {
		return CredentialModelMultiplier{}, fmt.Errorf("%w: invalid multiplier", ErrInvalidPricingInput)
	}
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		if err := validatePricingCredentialSubject(tx, id); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "subject_id"}, {Name: "model"}}, DoUpdates: clause.AssignmentColumns([]string{"multiplier", "updated_at"})}).Create(&entities.CredentialModelMultiplier{SubjectID: id, Model: model, Multiplier: value}).Error
	})
	if err != nil {
		if errors.Is(err, repository.ErrInvalidPricingSnapshot) {
			return CredentialModelMultiplier{}, fmt.Errorf("%w: unsafe multiplier", ErrInvalidPricingInput)
		}
		return CredentialModelMultiplier{}, err
	}
	return credentialModelDTO(snapshot, id, model)
}

func (s *pricingService) ClearCredentialModel(ctx context.Context, id, model string) (CredentialModelMultiplier, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return CredentialModelMultiplier{}, fmt.Errorf("%w: model is required", ErrInvalidPricingInput)
	}
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&entities.CredentialPricingSubject{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrPricingCredentialNotFound
		}
		return tx.Where("subject_id = ? AND model = ?", id, model).Delete(&entities.CredentialModelMultiplier{}).Error
	})
	if err != nil {
		return CredentialModelMultiplier{}, err
	}
	return credentialModelDTO(snapshot, id, model)
}

// Defaults and model exceptions accept the same exact bound/stale subject; no
// name, downstream key or provider category can create attribution evidence.
func validatePricingCredentialSubject(tx *gorm.DB, id string) error {
	identities, subjects, err := repository.LoadCredentialPricingDirectory(tx)
	if err != nil {
		return err
	}
	found := false
	for _, subject := range subjects {
		if subject.ID != id {
			continue
		}
		found = true
		if subject.BindingDisabled {
			continue
		}
		for _, identity := range identities {
			if identity.AuthType != subject.AuthType || identity.Identity != subject.Identity {
				continue
			}
			status := pricingCredential(identity, subjects).BindingStatus
			if status != "bound" && status != "stale" {
				return ErrCredentialNotSelectable
			}
		}
	}
	if found {
		return nil
	}
	return ErrPricingCredentialNotFound
}

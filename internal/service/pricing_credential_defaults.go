package service

import (
	"context"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPricingCredentialNotFound = errors.New("pricing credential not found")

type CredentialDefault struct {
	SubjectID  string   `json:"subject_id"`
	Multiplier *float64 `json:"multiplier"`
	SnapshotID string   `json:"snapshot_id"`
}

type PricingCredentialDefaultsProvider interface {
	GetCredentialDefault(context.Context, string) (CredentialDefault, error)
	SetCredentialDefault(context.Context, string, string) (CredentialDefault, error)
	ClearCredentialDefault(context.Context, string) (CredentialDefault, error)
}

func credentialDefaultDTO(snapshot *pricing.Snapshot, id string) (CredentialDefault, error) {
	if !snapshot.HasCredentialSubject(id) {
		return CredentialDefault{}, ErrPricingCredentialNotFound
	}
	result := CredentialDefault{SubjectID: id, SnapshotID: snapshot.ID()}
	if value, exists := snapshot.CredentialDefault(id); exists {
		result.Multiplier = &value
	}
	return result, nil
}

func (s *pricingService) GetCredentialDefault(_ context.Context, id string) (CredentialDefault, error) {
	return credentialDefaultDTO(s.catalog.Snapshot(), id)
}

func (s *pricingService) SetCredentialDefault(ctx context.Context, id, text string) (CredentialDefault, error) {
	value, err := pricing.ParseMultiplier(text)
	if err != nil {
		return CredentialDefault{}, fmt.Errorf("%w: invalid multiplier", ErrInvalidPricingInput)
	}
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		if err := validatePricingCredentialSubject(tx, id); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "subject_id"}}, DoUpdates: clause.AssignmentColumns([]string{"multiplier", "updated_at"})}).Create(&entities.CredentialPriceDefault{SubjectID: id, Multiplier: value}).Error
	})
	if err != nil {
		if errors.Is(err, repository.ErrInvalidPricingSnapshot) {
			return CredentialDefault{}, fmt.Errorf("%w: unsafe multiplier", ErrInvalidPricingInput)
		}
		return CredentialDefault{}, err
	}
	return credentialDefaultDTO(snapshot, id)
}

func (s *pricingService) ClearCredentialDefault(ctx context.Context, id string) (CredentialDefault, error) {
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&entities.CredentialPricingSubject{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrPricingCredentialNotFound
		}
		return tx.Where("subject_id = ?", id).Delete(&entities.CredentialPriceDefault{}).Error
	})
	if err != nil {
		return CredentialDefault{}, err
	}
	return credentialDefaultDTO(snapshot, id)
}

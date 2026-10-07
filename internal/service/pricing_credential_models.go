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
	SubjectID  string               `json:"subject_id"`
	Model      string               `json:"model"`
	Multiplier *float64             `json:"multiplier"`
	SnapshotID string               `json:"snapshot_id"`
	Mode       string               `json:"mode"`
	Fixed      *pricing.FixedTariff `json:"fixed,omitempty"`
}

type CredentialModelMultipliers struct {
	SubjectID  string                      `json:"subject_id"`
	Models     []CredentialModelMultiplier `json:"models"`
	SnapshotID string                      `json:"snapshot_id"`
}

type PricingCredentialFixedProvider interface {
	SetCredentialFixed(context.Context, string, string, pricing.FixedTariff) (CredentialModelMultiplier, error)
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
	result := CredentialModelMultiplier{SubjectID: id, Model: model, SnapshotID: snapshot.ID(), Mode: "inherit"}
	if config, exists := snapshot.CredentialModelPricing(id, model); exists {
		result.Mode, result.Fixed = config.Mode, config.Fixed
		if config.Mode == pricing.ModeMultiplier {
			value := config.Multiplier
			result.Multiplier = &value
		}
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
		entry, err := credentialModelDTO(snapshot, id, config.Model)
		if err != nil {
			return CredentialModelMultipliers{}, err
		}
		result.Models = append(result.Models, entry)
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
	return s.setCredentialModelPricing(ctx, entities.CredentialModelMultiplier{SubjectID: id, Model: model, Mode: pricing.ModeMultiplier, Multiplier: value})
}

func (s *pricingService) SetCredentialFixed(ctx context.Context, id, model string, fixed pricing.FixedTariff) (CredentialModelMultiplier, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return CredentialModelMultiplier{}, fmt.Errorf("%w: model required", ErrInvalidPricingInput)
	}
	if err := fixed.Validate(); err != nil {
		return CredentialModelMultiplier{}, fmt.Errorf("%w: invalid fixed tariff", ErrInvalidPricingInput)
	}
	return s.setCredentialModelPricing(ctx, entities.CredentialModelMultiplier{SubjectID: id, Model: model, Mode: pricing.ModeFixed, PromptPricePer1M: fixed.PromptPricePer1M, CompletionPricePer1M: fixed.CompletionPricePer1M, CacheReadPricePer1M: fixed.CacheReadPricePer1M, CacheWritePricePer1M: fixed.CacheWritePricePer1M, PricingStyle: fixed.PricingStyle})
}

func (s *pricingService) setCredentialModelPricing(ctx context.Context, row entities.CredentialModelMultiplier) (CredentialModelMultiplier, error) {
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		if err := validatePricingCredentialSubject(tx, row.SubjectID); err != nil {
			return err
		}
		// Replace the complete target mode in the same unique row, clearing every
		// inactive field. A failed candidate/COMMIT rolls back the whole replacement.
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "subject_id"}, {Name: "model"}}, DoUpdates: clause.AssignmentColumns([]string{"mode", "multiplier", "prompt_price_per1_m", "completion_price_per1_m", "cache_read_price_per1_m", "cache_write_price_per1_m", "pricing_style", "updated_at"})}).Create(&row).Error
	})
	if err != nil {
		if errors.Is(err, repository.ErrInvalidPricingSnapshot) {
			return CredentialModelMultiplier{}, fmt.Errorf("%w: unsafe multiplier", ErrInvalidPricingInput)
		}
		return CredentialModelMultiplier{}, err
	}
	return credentialModelDTO(snapshot, row.SubjectID, row.Model)
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
	for _, subject := range subjects {
		if subject.ID != id {
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
		return nil
	}
	return ErrPricingCredentialNotFound
}

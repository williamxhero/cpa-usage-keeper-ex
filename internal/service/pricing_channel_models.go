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

type ChannelModelPrice struct {
	ChannelID  string               `json:"channel_id"`
	Model      string               `json:"model"`
	Multiplier *float64             `json:"multiplier"`
	Mode       string               `json:"mode"`
	Fixed      *pricing.FixedTariff `json:"fixed,omitempty"`
	SnapshotID string               `json:"snapshot_id"`
}
type ChannelModelPrices struct {
	ChannelID  string              `json:"channel_id"`
	Models     []ChannelModelPrice `json:"models"`
	SnapshotID string              `json:"snapshot_id"`
}
type PricingChannelModelsProvider interface {
	ListChannelModels(context.Context, string) (ChannelModelPrices, error)
	GetChannelModel(context.Context, string, string) (ChannelModelPrice, error)
	SetChannelModel(context.Context, string, string, string) (ChannelModelPrice, error)
	SetChannelFixed(context.Context, string, string, pricing.FixedTariff) (ChannelModelPrice, error)
	ClearChannelModel(context.Context, string, string) (ChannelModelPrice, error)
}

func channelModelDTO(snapshot *pricing.Snapshot, id, model string) (ChannelModelPrice, error) {
	if _, err := channelDTO(snapshot, id); err != nil {
		return ChannelModelPrice{}, err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return ChannelModelPrice{}, fmt.Errorf("%w: model required", ErrInvalidPricingInput)
	}
	result := ChannelModelPrice{ChannelID: id, Model: model, Mode: "inherit", SnapshotID: snapshot.ID()}
	if config, exists := snapshot.ChannelModelPricing(id, model); exists {
		result.Mode, result.Fixed = config.Mode, config.Fixed
		if config.Mode == pricing.ModeMultiplier {
			value := config.Multiplier
			result.Multiplier = &value
		}
	}
	return result, nil
}
func (s *pricingService) ListChannelModels(_ context.Context, id string) (ChannelModelPrices, error) {
	snapshot := s.catalog.Snapshot()
	if _, err := channelDTO(snapshot, id); err != nil {
		return ChannelModelPrices{}, err
	}
	result := ChannelModelPrices{ChannelID: id, Models: []ChannelModelPrice{}, SnapshotID: snapshot.ID()}
	for _, config := range snapshot.ChannelModelConfigs(id) {
		entry, err := channelModelDTO(snapshot, id, config.Model)
		if err != nil {
			return ChannelModelPrices{}, err
		}
		result.Models = append(result.Models, entry)
	}
	return result, nil
}
func (s *pricingService) GetChannelModel(_ context.Context, id, model string) (ChannelModelPrice, error) {
	return channelModelDTO(s.catalog.Snapshot(), id, model)
}
func (s *pricingService) SetChannelModel(ctx context.Context, id, model, text string) (ChannelModelPrice, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ChannelModelPrice{}, fmt.Errorf("%w: model required", ErrInvalidPricingInput)
	}
	value, err := pricing.ParseMultiplier(text)
	if err != nil {
		return ChannelModelPrice{}, fmt.Errorf("%w: invalid multiplier", ErrInvalidPricingInput)
	}
	return s.setChannelModelPricing(ctx, entities.ChannelModelPrice{ChannelID: id, Model: model, Mode: pricing.ModeMultiplier, Multiplier: value})
}
func (s *pricingService) SetChannelFixed(ctx context.Context, id, model string, fixed pricing.FixedTariff) (ChannelModelPrice, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ChannelModelPrice{}, fmt.Errorf("%w: model required", ErrInvalidPricingInput)
	}
	if err := fixed.Validate(); err != nil {
		return ChannelModelPrice{}, fmt.Errorf("%w: invalid fixed tariff", ErrInvalidPricingInput)
	}
	return s.setChannelModelPricing(ctx, entities.ChannelModelPrice{ChannelID: id, Model: model, Mode: pricing.ModeFixed, PromptPricePer1M: fixed.PromptPricePer1M, CompletionPricePer1M: fixed.CompletionPricePer1M, CacheReadPricePer1M: fixed.CacheReadPricePer1M, CacheWritePricePer1M: fixed.CacheWritePricePer1M, PricingStyle: fixed.PricingStyle})
}
func (s *pricingService) setChannelModelPricing(ctx context.Context, row entities.ChannelModelPrice) (ChannelModelPrice, error) {
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		if err := requireChannel(tx, row.ChannelID); err != nil {
			return err
		}
		var members []entities.PricingChannelMember
		if err := tx.Where("channel_id = ?", row.ChannelID).Find(&members).Error; err != nil {
			return err
		}
		ids := make([]string, 0, len(members))
		for _, member := range members {
			ids = append(ids, member.SubjectID)
		}
		if err := validateChannelMembers(tx, row.ChannelID, ids); err != nil {
			return err
		}
		// Replace the complete mode; inactive fields are cleared in the same row.
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "channel_id"}, {Name: "model"}}, DoUpdates: clause.AssignmentColumns([]string{"mode", "multiplier", "prompt_price_per1_m", "completion_price_per1_m", "cache_read_price_per1_m", "cache_write_price_per1_m", "pricing_style", "updated_at"})}).Create(&row).Error
	})
	if errors.Is(err, repository.ErrInvalidPricingSnapshot) {
		return ChannelModelPrice{}, fmt.Errorf("%w: unsafe channel model pricing", ErrInvalidPricingInput)
	}
	if err != nil {
		return ChannelModelPrice{}, err
	}
	return channelModelDTO(snapshot, row.ChannelID, row.Model)
}
func (s *pricingService) ClearChannelModel(ctx context.Context, id, model string) (ChannelModelPrice, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ChannelModelPrice{}, fmt.Errorf("%w: model required", ErrInvalidPricingInput)
	}
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		if err := requireChannel(tx, id); err != nil {
			return err
		}
		return tx.Where("channel_id = ? AND model = ?", id, model).Delete(&entities.ChannelModelPrice{}).Error
	})
	if err != nil {
		return ChannelModelPrice{}, err
	}
	return channelModelDTO(snapshot, id, model)
}

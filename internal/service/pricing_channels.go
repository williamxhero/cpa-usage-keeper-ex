package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrPricingChannelNotFound     = errors.New("pricing channel not found")
	ErrPricingChannelConflict     = errors.New("channel members conflict or are not uniquely selectable")
	ErrPricingChannelDependencies = errors.New("channel has dependencies; confirm removal of members and default")
)

type PricingChannel struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	MemberSubjectIDs []string `json:"member_subject_ids"`
	Multiplier       *float64 `json:"multiplier"`
	SnapshotID       string   `json:"snapshot_id"`
}
type PricingChannelInput struct {
	Name             string   `json:"name"`
	MemberSubjectIDs []string `json:"member_subject_ids"`
}
type PricingChannelsProvider interface {
	ListPricingChannels(context.Context) ([]PricingChannel, error)
	GetPricingChannel(context.Context, string) (PricingChannel, error)
	CreatePricingChannel(context.Context, PricingChannelInput) (PricingChannel, error)
	UpdatePricingChannel(context.Context, string, PricingChannelInput) (PricingChannel, error)
	DeletePricingChannel(context.Context, string, bool) error
	SetChannelDefault(context.Context, string, string) (PricingChannel, error)
	ClearChannelDefault(context.Context, string) (PricingChannel, error)
}

func channelDTO(snapshot *pricing.Snapshot, id string) (PricingChannel, error) {
	for _, config := range snapshot.Channels() {
		if config.ID == id {
			return PricingChannel{ID: id, Name: config.Name, MemberSubjectIDs: config.MemberSubjectIDs, Multiplier: config.Multiplier, SnapshotID: snapshot.ID()}, nil
		}
	}
	return PricingChannel{}, ErrPricingChannelNotFound
}
func (s *pricingService) ListPricingChannels(_ context.Context) ([]PricingChannel, error) {
	snapshot := s.catalog.Snapshot()
	result := []PricingChannel{}
	for _, config := range snapshot.Channels() {
		item, _ := channelDTO(snapshot, config.ID)
		result = append(result, item)
	}
	return result, nil
}
func (s *pricingService) GetPricingChannel(_ context.Context, id string) (PricingChannel, error) {
	return channelDTO(s.catalog.Snapshot(), id)
}
func (s *pricingService) CreatePricingChannel(ctx context.Context, input PricingChannelInput) (PricingChannel, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return PricingChannel{}, err
	}
	return s.savePricingChannel(ctx, "chan_"+hex.EncodeToString(bytes), input, true)
}
func (s *pricingService) UpdatePricingChannel(ctx context.Context, id string, input PricingChannelInput) (PricingChannel, error) {
	return s.savePricingChannel(ctx, id, input, false)
}
func (s *pricingService) savePricingChannel(ctx context.Context, id string, input PricingChannelInput, create bool) (PricingChannel, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 128 || pricing.SafeCredentialText(input.Name, entities.UsageIdentity{}) != input.Name || input.MemberSubjectIDs == nil {
		return PricingChannel{}, fmt.Errorf("%w: invalid channel name or explicit members required", ErrInvalidPricingInput)
	}
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		if !create {
			if err := requireChannel(tx, id); err != nil {
				return err
			}
		}
		if err := validateChannelMembers(tx, id, input.MemberSubjectIDs); err != nil {
			return err
		}
		if create {
			if err := tx.Create(&entities.PricingChannel{ID: id, Name: input.Name}).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&entities.PricingChannel{}).Where("id = ?", id).Updates(map[string]any{"name": input.Name}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("channel_id = ?", id).Delete(&entities.PricingChannelMember{}).Error; err != nil {
			return err
		}
		for _, subject := range input.MemberSubjectIDs {
			if err := tx.Create(&entities.PricingChannelMember{SubjectID: subject, ChannelID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return PricingChannel{}, err
	}
	return channelDTO(snapshot, id)
}

func validateChannelMembers(tx *gorm.DB, channelID string, members []string) error {
	identities, subjects, err := repository.LoadCredentialPricingDirectory(tx)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, id := range members {
		if id == "" || seen[id] {
			return ErrPricingChannelConflict
		}
		seen[id] = true
		var existing entities.PricingChannelMember
		result := tx.Where("subject_id = ?", id).Limit(1).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 && existing.ChannelID != channelID {
			return ErrPricingChannelConflict
		}
		found := false
		for _, subject := range subjects {
			if subject.ID != id {
				continue
			}
			found = true
			// Historical exact subjects are preserved, but cannot be guessed from absent
			// or ambiguous directory evidence when saving a new membership assertion.
			selectable := false
			for _, identity := range identities {
				if identity.AuthType != subject.AuthType || identity.Identity != subject.Identity {
					continue
				}
				status := pricingCredential(identity, subjects).BindingStatus
				if status != "bound" && status != "stale" {
					return ErrPricingChannelConflict
				}
				if selectable {
					return ErrPricingChannelConflict
				}
				selectable = true
			}
			if !selectable {
				return ErrPricingChannelConflict
			}
		}
		if !found {
			return ErrPricingChannelConflict
		}
	}
	return nil
}
func requireChannel(tx *gorm.DB, id string) error {
	var count int64
	if err := tx.Model(&entities.PricingChannel{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrPricingChannelNotFound
	}
	return nil
}
func (s *pricingService) DeletePricingChannel(ctx context.Context, id string, confirm bool) error {
	_, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		if err := requireChannel(tx, id); err != nil {
			return err
		}
		var members, defaults int64
		if err := tx.Model(&entities.PricingChannelMember{}).Where("channel_id = ?", id).Count(&members).Error; err != nil {
			return err
		}
		if err := tx.Model(&entities.ChannelPriceDefault{}).Where("channel_id = ?", id).Count(&defaults).Error; err != nil {
			return err
		}
		if !confirm && (members > 0 || defaults > 0) {
			return ErrPricingChannelDependencies
		}
		if err := tx.Where("channel_id = ?", id).Delete(&entities.ChannelPriceDefault{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", id).Delete(&entities.PricingChannelMember{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&entities.PricingChannel{}).Error
	})
	return err
}
func (s *pricingService) SetChannelDefault(ctx context.Context, id, text string) (PricingChannel, error) {
	value, err := pricing.ParseMultiplier(text)
	if err != nil {
		return PricingChannel{}, fmt.Errorf("%w: invalid multiplier", ErrInvalidPricingInput)
	}
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		if err := requireChannel(tx, id); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "channel_id"}}, DoUpdates: clause.AssignmentColumns([]string{"multiplier", "updated_at"})}).Create(&entities.ChannelPriceDefault{ChannelID: id, Multiplier: value}).Error
	})
	if errors.Is(err, repository.ErrInvalidPricingSnapshot) {
		return PricingChannel{}, fmt.Errorf("%w: unsafe multiplier", ErrInvalidPricingInput)
	}
	if err != nil {
		return PricingChannel{}, err
	}
	return channelDTO(snapshot, id)
}
func (s *pricingService) ClearChannelDefault(ctx context.Context, id string) (PricingChannel, error) {
	snapshot, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		if err := requireChannel(tx, id); err != nil {
			return err
		}
		return tx.Where("channel_id = ?", id).Delete(&entities.ChannelPriceDefault{}).Error
	})
	if err != nil {
		return PricingChannel{}, err
	}
	return channelDTO(snapshot, id)
}

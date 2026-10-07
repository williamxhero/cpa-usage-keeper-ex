package repository

import (
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"fmt"
	"gorm.io/gorm"
)

func LoadPricingChannels(db *gorm.DB) ([]pricing.ChannelConfig, error) {
	var channels []entities.PricingChannel
	var members []entities.PricingChannelMember
	var defaults []entities.ChannelPriceDefault
	if err := db.Order("id").Find(&channels).Error; err != nil {
		return nil, err
	}
	if err := db.Find(&members).Error; err != nil {
		return nil, err
	}
	if err := db.Find(&defaults).Error; err != nil {
		return nil, err
	}
	configs := make([]pricing.ChannelConfig, len(channels))
	indexes := make(map[string]int)
	for i, channel := range channels {
		configs[i] = pricing.ChannelConfig{ID: channel.ID, Name: channel.Name, MemberSubjectIDs: []string{}}
		indexes[channel.ID] = i
	}
	for _, member := range members {
		i, exists := indexes[member.ChannelID]
		if !exists {
			return nil, fmt.Errorf("channel member references missing channel")
		}
		configs[i].MemberSubjectIDs = append(configs[i].MemberSubjectIDs, member.SubjectID)
	}
	for _, value := range defaults {
		i, exists := indexes[value.ChannelID]
		if !exists {
			return nil, fmt.Errorf("default references missing channel")
		}
		multiplier := value.Multiplier
		configs[i].Multiplier = &multiplier
	}
	return configs, nil
}

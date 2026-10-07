package repository

import (
	"context"
	"errors"
	"fmt"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"

	"gorm.io/gorm"
)

var ErrInvalidPricingSnapshot = errors.New("invalid pricing snapshot")

// LoadPricingSnapshot 从传入的 DB/transaction 一次加载并编译完整价格快照。
func LoadPricingSnapshot(ctx context.Context, db *gorm.DB) (*pricing.Snapshot, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := db.WithContext(ctx)
	settings, err := ListModelPriceSettings(query)
	if err != nil {
		return nil, err
	}
	rules, err := ListModelPriceRules(query)
	if err != nil {
		return nil, err
	}

	configIndexByID := make(map[int64]int, len(settings))
	configs := make([]pricing.ModelConfig, len(settings))
	for index := range settings {
		configs[index].Pricing = settings[index]
		configIndexByID[settings[index].ID] = index
	}
	for index := range rules {
		configIndex, ok := configIndexByID[rules[index].ModelPriceSettingID]
		if !ok {
			return nil, fmt.Errorf("model price rule %d references missing price %d", rules[index].ID, rules[index].ModelPriceSettingID)
		}
		configs[configIndex].Rules = append(configs[configIndex].Rules, pricing.RuleConfig{
			Key:        rules[index].Key,
			Value:      rules[index].Value,
			Multiplier: rules[index].Multiplier,
		})
	}
	identities, subjects, err := LoadCredentialPricingDirectory(query)
	if err != nil {
		return nil, err
	}
	var defaults []entities.CredentialPriceDefault
	if err := query.Find(&defaults).Error; err != nil {
		return nil, err
	}
	credentialConfigs := make([]pricing.CredentialConfig, 0, len(defaults))
	for _, value := range defaults {
		credentialConfigs = append(credentialConfigs, pricing.CredentialConfig{SubjectID: value.SubjectID, Multiplier: value.Multiplier})
	}
	channels, err := LoadPricingChannels(query)
	if err != nil {
		return nil, err
	}
	for index := range channels {
		// Metadata refreshes may reveal secret evidence after the label was saved.
		// Preserve ID, membership and prices, but never publish that label outward.
		channels[index].Name = pricing.SafeChannelText(channels[index].Name, identities, subjects)
		if channels[index].Name == "" {
			channels[index].Name = "Channel"
		}
	}
	var exceptions []entities.CredentialModelMultiplier
	if err := query.Find(&exceptions).Error; err != nil {
		return nil, err
	}
	modelConfigs := make([]pricing.CredentialModelConfig, 0, len(exceptions))
	for _, value := range exceptions {
		config := pricing.CredentialModelConfig{SubjectID: value.SubjectID, Model: value.Model, Multiplier: value.Multiplier, Mode: value.Mode}
		if value.Mode == pricing.ModeFixed {
			config.Fixed = &pricing.FixedTariff{PromptPricePer1M: value.PromptPricePer1M, CompletionPricePer1M: value.CompletionPricePer1M, CacheReadPricePer1M: value.CacheReadPricePer1M, CacheWritePricePer1M: value.CacheWritePricePer1M, PricingStyle: value.PricingStyle}
		}
		modelConfigs = append(modelConfigs, config)
	}
	var channelPrices []entities.ChannelModelPrice
	if err := query.Find(&channelPrices).Error; err != nil {
		return nil, err
	}
	channelModels := make([]pricing.ChannelModelConfig, 0, len(channelPrices))
	for _, value := range channelPrices {
		config := pricing.ChannelModelConfig{ChannelID: value.ChannelID, Model: value.Model, Multiplier: value.Multiplier, Mode: value.Mode}
		if value.Mode == pricing.ModeFixed {
			config.Fixed = &pricing.FixedTariff{PromptPricePer1M: value.PromptPricePer1M, CompletionPricePer1M: value.CompletionPricePer1M, CacheReadPricePer1M: value.CacheReadPricePer1M, CacheWritePricePer1M: value.CacheWritePricePer1M, PricingStyle: value.PricingStyle}
		}
		channelModels = append(channelModels, config)
	}
	snapshot, err := pricing.CompileSnapshotWithCredentials(configs, compileCredentialBindings(identities, subjects), credentialConfigs, pricing.OverrideConfig{Channels: channels, CredentialModels: modelConfigs, ChannelModels: channelModels})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPricingSnapshot, err)
	}
	return snapshot, nil
}

func compileCredentialBindings(identities []entities.UsageIdentity, subjects []entities.CredentialPricingSubject) []pricing.CredentialBinding {
	bindings := make([]pricing.CredentialBinding, 0, len(identities)+len(subjects))
	seen := make(map[string]bool)
	for _, identity := range identities {
		typeName := credentialAuthType(identity.AuthType)
		status := identity.BindingIdentityStatus
		if typeName == "" || identity.AuthTypeName != typeName {
			status = "unknown"
		}
		binding := pricing.CredentialBinding{AuthType: typeName, AuthIndex: identity.Identity, Status: status, SafeName: pricing.SafeCredentialText(identity.Name, identity)}
		for _, subject := range subjects {
			if !subject.BindingDisabled && subject.AuthType == identity.AuthType && subject.Identity == identity.Identity {
				binding.SubjectID = subject.ID
				if subject.AuthTypeName != typeName {
					binding.Status = "ambiguous"
				}
				seen[subject.BindingRef] = true
			}
		}
		bindings = append(bindings, binding)
	}
	// A directory entry can disappear after a successful scoped refresh. Saved
	// exact historical links remain evidence; display names are never used.
	for _, subject := range subjects {
		if subject.BindingDisabled {
			bindings = append(bindings, pricing.CredentialBinding{SubjectID: subject.ID})
			continue
		}
		if seen[subject.BindingRef] {
			continue
		}
		typeName := credentialAuthType(subject.AuthType)
		status := "unique"
		if typeName == "" || subject.AuthTypeName != typeName {
			status = "unknown"
		}
		bindings = append(bindings, pricing.CredentialBinding{SubjectID: subject.ID, AuthType: typeName, AuthIndex: subject.Identity, Status: status})
	}
	return bindings
}

func credentialAuthType(value entities.UsageIdentityAuthType) string {
	switch value {
	case entities.UsageIdentityAuthTypeAIProvider:
		return "apikey"
	case entities.UsageIdentityAuthTypeAuthFile:
		return "oauth"
	}
	return ""
}

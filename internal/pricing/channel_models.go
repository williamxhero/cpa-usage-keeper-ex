package pricing

import (
	"fmt"
	"sort"
	"strings"
)

type ChannelModelConfig struct {
	ChannelID  string
	Model      string
	Multiplier float64
	Mode       string
	Fixed      *FixedTariff
}

type channelModelKey struct{ channelID, model string }

func (s *Snapshot) compileChannelModels(configs []ChannelModelConfig) error {
	s.channelModels = make(map[channelModelKey]ChannelModelConfig, len(configs))
	for _, config := range configs {
		if _, exists := s.channels[config.ChannelID]; !exists {
			return fmt.Errorf("channel model references missing channel")
		}
		config.Model = strings.TrimSpace(config.Model)
		if config.Model == "" {
			return fmt.Errorf("channel model required")
		}
		if config.Mode == "" {
			config.Mode = ModeMultiplier
		}
		key := channelModelKey{config.ChannelID, config.Model}
		if _, exists := s.channelModels[key]; exists {
			return fmt.Errorf("duplicate channel model")
		}
		if err := s.validateModelOverride(config.Model, config.Mode, config.Multiplier, config.Fixed); err != nil {
			return err
		}
		if config.Mode == ModeFixed {
			config.Multiplier = 0
			config.Fixed = cloneFixed(config.Fixed)
		} else {
			config.Fixed = nil
		}
		s.channelModels[key] = config
	}
	return nil
}

func (s *Snapshot) ChannelModelPricing(id, model string) (ChannelModelConfig, bool) {
	if s == nil {
		return ChannelModelConfig{}, false
	}
	config, exists := s.channelModels[channelModelKey{id, model}]
	config.Fixed = cloneFixed(config.Fixed)
	return config, exists
}
func (s *Snapshot) ChannelModelConfigs(id string) []ChannelModelConfig {
	result := []ChannelModelConfig{}
	if s == nil {
		return result
	}
	for key, config := range s.channelModels {
		if key.channelID == id {
			config.Fixed = cloneFixed(config.Fixed)
			result = append(result, config)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Model < result[j].Model })
	return result
}
func (r Resolver) channelModel(subject CostSubject) (ChannelModelConfig, string, bool) {
	if r.snapshot == nil {
		return ChannelModelConfig{}, "", false
	}
	id := r.snapshot.subjectChannels[r.credentialSubject(subject)]
	if config, selected := r.snapshot.ChannelModelPricing(id, subject.Dimensions.Model); selected {
		return config, "model", true
	}
	if config, selected := r.snapshot.ChannelModelPricing(id, subject.Dimensions.ModelAlias); selected {
		return config, "model_alias", true
	}
	return ChannelModelConfig{}, "", false
}

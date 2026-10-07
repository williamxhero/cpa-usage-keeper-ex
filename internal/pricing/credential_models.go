package pricing

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"cpa-usage-keeper/internal/entities"
)

const (
	ModeMultiplier = "multiplier"
	ModeFixed      = "fixed"
)

// Pointers distinguish a complete known zero tariff from an omitted rate. The
// compiler clones all pointers so callers cannot mutate a published snapshot.
type FixedTariff struct {
	PromptPricePer1M     *float64 `json:"prompt_price_per_1m"`
	CompletionPricePer1M *float64 `json:"completion_price_per_1m"`
	CacheReadPricePer1M  *float64 `json:"cache_read_price_per_1m"`
	CacheWritePricePer1M *float64 `json:"cache_write_price_per_1m"`
	PricingStyle         string   `json:"pricing_style,omitempty"`
}

func ParseFixedRate(text string) (float64, error) {
	text = strings.TrimSpace(text)
	if !decimalMultiplier.MatchString(text) {
		return 0, fmt.Errorf("rate must be a non-negative decimal")
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || !isNonNegativeFinite(value) {
		return 0, fmt.Errorf("rate must be finite")
	}
	return value, nil
}

func (f FixedTariff) Validate() error {
	for _, rate := range []*float64{f.PromptPricePer1M, f.CompletionPricePer1M, f.CacheReadPricePer1M, f.CacheWritePricePer1M} {
		if rate == nil || !isNonNegativeFinite(*rate) {
			return fmt.Errorf("four finite non-negative rates required")
		}
	}
	if f.PricingStyle != "" && f.PricingStyle != entities.ModelPricingStyleOpenAI && f.PricingStyle != entities.ModelPricingStyleClaude {
		return fmt.Errorf("unsupported pricing style")
	}
	return validateWorstCaseCost(f.pricing(f.PricingStyle), nil)
}
func (f FixedTariff) pricing(style string) entities.ModelPriceSetting {
	return entities.ModelPriceSetting{PricingStyle: style, PromptPricePer1M: *f.PromptPricePer1M, CompletionPricePer1M: *f.CompletionPricePer1M, CacheReadPricePer1M: *f.CacheReadPricePer1M, CacheWritePricePer1M: *f.CacheWritePricePer1M}
}
func cloneFixed(f *FixedTariff) *FixedTariff {
	if f == nil {
		return nil
	}
	result := *f
	result.PromptPricePer1M = cloneRate(f.PromptPricePer1M)
	result.CompletionPricePer1M = cloneRate(f.CompletionPricePer1M)
	result.CacheReadPricePer1M = cloneRate(f.CacheReadPricePer1M)
	result.CacheWritePricePer1M = cloneRate(f.CacheWritePricePer1M)
	return &result
}

func cloneRate(rate *float64) *float64 {
	if rate == nil {
		return nil
	}
	value := *rate
	return &value
}

type CredentialModelConfig struct {
	SubjectID  string
	Model      string
	Multiplier float64
	Mode       string
	Fixed      *FixedTariff
}

type credentialModelKey struct{ subjectID, model string }

func (s *Snapshot) compileCredentialModels(configs []CredentialModelConfig) error {
	s.credentialModels = make(map[credentialModelKey]CredentialModelConfig, len(configs))
	for _, config := range configs {
		if !s.HasCredentialSubject(config.SubjectID) {
			return fmt.Errorf("credential model references missing subject")
		}
		config.Model = strings.TrimSpace(config.Model)
		if config.Model == "" {
			return fmt.Errorf("credential model required")
		}
		if config.Mode == "" {
			config.Mode = ModeMultiplier
		}
		key := credentialModelKey{config.SubjectID, config.Model}
		if _, exists := s.credentialModels[key]; exists {
			return fmt.Errorf("duplicate credential model")
		}
		if err := s.validateModelOverride(config.Model, config.Mode, config.Multiplier, config.Fixed); err != nil {
			return err
		}
		if config.Mode == ModeFixed {
			config.Multiplier = 0 // inactive mode never contributes
			config.Fixed = cloneFixed(config.Fixed)
		} else {
			config.Fixed = nil
		}
		s.credentialModels[key] = config
	}
	return nil
}

// Both credential and channel scopes validate complete modes against the same
// independent baseline and fixed-tariff safety rules.
func (s *Snapshot) validateModelOverride(model, mode string, multiplier float64, fixed *FixedTariff) error {
	switch mode {
	case ModeFixed:
		if fixed == nil {
			return fmt.Errorf("fixed tariff required")
		}
		if err := fixed.Validate(); err != nil {
			return err
		}
		if fixed.PricingStyle == "" {
			baseline, ok := s.modelsByName[model]
			if !ok || (baseline.pricing.PricingStyle != entities.ModelPricingStyleOpenAI && baseline.pricing.PricingStyle != entities.ModelPricingStyleClaude) {
				return fmt.Errorf("explicit pricing style required without baseline")
			}
		}
	case ModeMultiplier:
		if !isNonNegativeFinite(multiplier) {
			return fmt.Errorf("finite non-negative multiplier required")
		}
	default:
		return fmt.Errorf("invalid model pricing mode")
	}
	// Selection and baseline lookup are independent, including Model/Alias.
	for _, baselineModel := range s.modelsByName {
		baseline := unadjustedPricing(baselineModel)
		if err := validateWorstCaseCost(baseline, nil); err != nil {
			return fmt.Errorf("unsafe unadjusted baseline: %w", err)
		}
		if mode == ModeMultiplier {
			baseline.PriceMultiplier = &multiplier
			if err := validateWorstCaseCost(baseline, nil); err != nil {
				return fmt.Errorf("unsafe model multiplier: %w", err)
			}
		}
	}
	return nil
}

// CredentialModel retains the multiplier-only lookup for existing callers.
func (s *Snapshot) CredentialModel(id, model string) (float64, bool) {
	config, ok := s.CredentialModelPricing(id, model)
	return config.Multiplier, ok && config.Mode == ModeMultiplier
}
func (s *Snapshot) CredentialModelPricing(id, model string) (CredentialModelConfig, bool) {
	if s == nil {
		return CredentialModelConfig{}, false
	}
	config, exists := s.credentialModels[credentialModelKey{id, model}]
	config.Fixed = cloneFixed(config.Fixed)
	return config, exists
}

// CredentialModelConfigs returns deep value copies in deterministic model order.
func (s *Snapshot) CredentialModelConfigs(id string) []CredentialModelConfig {
	result := []CredentialModelConfig{}
	if s == nil {
		return result
	}
	for key, config := range s.credentialModels {
		if key.subjectID == id {
			config.Fixed = cloneFixed(config.Fixed)
			result = append(result, config)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Model < result[j].Model })
	return result
}

func (r Resolver) credentialModel(subject CostSubject) (config CredentialModelConfig, by string, selected bool) {
	id := r.credentialSubject(subject)
	if config, selected = r.snapshot.CredentialModelPricing(id, subject.Dimensions.Model); selected {
		return config, "model", true
	}
	if config, selected = r.snapshot.CredentialModelPricing(id, subject.Dimensions.ModelAlias); selected {
		return config, "model_alias", true
	}
	return CredentialModelConfig{}, "", false
}

package pricing

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"cpa-usage-keeper/internal/entities"
)

// OverrideConfig extends the same immutable candidate, not a second catalog.
type OverrideConfig struct {
	Channels         []ChannelConfig
	CredentialModels []CredentialModelConfig
	ChannelModels    []ChannelModelConfig
}

type ChannelConfig struct {
	ID               string
	Name             string
	MemberSubjectIDs []string
	Multiplier       *float64
}

func (s *Snapshot) compileChannels(configs []ChannelConfig) error {
	s.channels = make(map[string]ChannelConfig)
	s.subjectChannels = make(map[string]string)
	for _, input := range configs {
		if input.ID == "" || input.Name == "" || strings.TrimSpace(input.Name) != input.Name || utf8.RuneCountInString(input.Name) > 128 || SafeCredentialText(input.Name, entities.UsageIdentity{}) != input.Name {
			return fmt.Errorf("channel ID and safe name required")
		}
		if _, exists := s.channels[input.ID]; exists {
			return fmt.Errorf("duplicate channel")
		}
		config := input
		config.MemberSubjectIDs = append([]string{}, input.MemberSubjectIDs...)
		sort.Strings(config.MemberSubjectIDs)
		for _, id := range config.MemberSubjectIDs {
			if !s.HasCredentialSubject(id) {
				return fmt.Errorf("channel member references missing credential")
			}
			if _, exists := s.subjectChannels[id]; exists {
				return fmt.Errorf("duplicate or conflicting channel member")
			}
			s.subjectChannels[id] = config.ID
		}
		if input.Multiplier != nil {
			value := *input.Multiplier
			if err := s.validateDefaultMultiplier(value); err != nil {
				return err
			}
			config.Multiplier = &value
		}
		s.channels[config.ID] = config
	}
	return nil
}

func (s *Snapshot) validateDefaultMultiplier(value float64) error {
	if !isNonNegativeFinite(value) {
		return fmt.Errorf("multiplier must be finite and non-negative")
	}
	for _, model := range s.modelsByName {
		baseline := unadjustedPricing(model)
		if err := validateWorstCaseCost(baseline, nil); err != nil {
			return fmt.Errorf("unsafe unadjusted baseline: %w", err)
		}
		baseline.PriceMultiplier = &value
		if err := validateWorstCaseCost(baseline, nil); err != nil {
			return fmt.Errorf("unsafe default multiplier: %w", err)
		}
	}
	return nil
}

func (s *Snapshot) Channels() []ChannelConfig {
	result := []ChannelConfig{}
	if s == nil {
		return result
	}
	for _, config := range s.channels {
		config.MemberSubjectIDs = append([]string{}, config.MemberSubjectIDs...)
		if config.Multiplier != nil {
			value := *config.Multiplier
			config.Multiplier = &value
		}
		result = append(result, config)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// SafeChannelText checks all known directory and retained subject evidence, not
// just chosen members: a friendly channel label must never echo credential data.
func SafeChannelText(value string, identities []entities.UsageIdentity, subjects []entities.CredentialPricingSubject) string {
	value = SafeCredentialText(value, entities.UsageIdentity{})
	for _, identity := range identities {
		value = SafeCredentialText(value, identity)
	}
	for _, subject := range subjects {
		value = SafeCredentialText(value, entities.UsageIdentity{Identity: subject.Identity})
	}
	return value
}

// Exact evidence is selected independently of normalized legacy dimensions.
func (r Resolver) credentialSubject(subject CostSubject) string {
	if r.snapshot == nil {
		return ""
	}
	if subject.AuthType == "" && !subject.ObservedIdentity {
		return r.snapshot.credentialIndexes[subject.IdentityAuthIndex]
	}
	return r.snapshot.credentials[credentialIdentity{subject.AuthType, subject.IdentityAuthIndex}]
}

func (r Resolver) channelDefault(subject CostSubject) (string, float64, bool) {
	if r.snapshot == nil {
		return "", 0, false
	}
	id := r.snapshot.subjectChannels[r.credentialSubject(subject)]
	config := r.snapshot.channels[id]
	if config.Multiplier == nil {
		return id, 0, false
	}
	return id, *config.Multiplier, true
}

// Attribution projections keep exact cohort dimensions without changing the
// legacy grouping of cost-only endpoints when all defaults have been cleared.
func (r Resolver) AttributionFields() ActiveFields {
	fields := r.ActiveFields()
	if r.HasChannels() {
		for field := RuleFieldAPIGroupKey; field < ruleFieldCount; field++ {
			fields = fields.with(field)
		}
	}
	return fields
}

// EvidenceFields retains physical membership coordinates without changing the
// legacy fields used to group configured estimates.
func (r Resolver) EvidenceFields() ActiveFields {
	var fields ActiveFields
	for field := RuleFieldAPIGroupKey; field < ruleFieldCount; field++ {
		fields = fields.with(field)
	}
	return fields
}

func (r Resolver) HasChannels() bool { return r.snapshot != nil && len(r.snapshot.channels) > 0 }
func (r Resolver) HasPricingOverrides() bool {
	if r.HasCredentialDefaults() || (r.snapshot != nil && (len(r.snapshot.credentialModels) > 0 || len(r.snapshot.channelModels) > 0)) {
		return true
	}
	if r.snapshot != nil {
		for _, channel := range r.snapshot.channels {
			if channel.Multiplier != nil {
				return true
			}
		}
	}
	return false
}
func (r Resolver) UsesPricingOverride(subject CostSubject) bool {
	if _, _, selected := r.credentialModel(subject); selected {
		return true
	}
	if r.UsesCredentialDefault(subject) {
		return true
	}
	if _, _, selected := r.channelModel(subject); selected {
		return true
	}
	_, _, selected := r.channelDefault(subject)
	return selected
}
func (r Resolver) MayUsePricingOverride(subject CostSubject) bool {
	if r.snapshot == nil {
		return false
	}
	for key, id := range r.snapshot.credentials {
		// Conservative coverage guard only; never select from a normalized index.
		if strings.TrimSpace(key.authIndex) != subject.Dimensions.AuthIndex {
			continue
		}
		if _, ok := r.snapshot.credentialDefaults[id]; ok {
			return true
		}
		if _, ok := r.snapshot.CredentialModelPricing(id, subject.Dimensions.Model); ok {
			return true
		}
		if _, ok := r.snapshot.CredentialModelPricing(id, subject.Dimensions.ModelAlias); ok {
			return true
		}
		channelID := r.snapshot.subjectChannels[id]
		if _, ok := r.snapshot.ChannelModelPricing(channelID, subject.Dimensions.Model); ok {
			return true
		}
		if _, ok := r.snapshot.ChannelModelPricing(channelID, subject.Dimensions.ModelAlias); ok {
			return true
		}
		if channel := r.snapshot.channels[channelID]; channel.Multiplier != nil {
			return true
		}
	}
	return false
}

// ChannelAttribution never equates a provider, name, or endpoint to a channel.
func (r Resolver) ChannelAttribution(subject CostSubject) (string, string, string) {
	if r.snapshot == nil || !r.HasChannels() {
		return "", "", ""
	}
	id := r.snapshot.subjectChannels[r.credentialSubject(subject)]
	if id != "" {
		return id, r.snapshot.channels[id].Name, ""
	}
	if subject.IdentityAuthIndex == "" || (subject.ObservedIdentity && subject.AuthType != "apikey" && subject.AuthType != "oauth") {
		return "", "", "unknown_identity"
	}
	if r.credentialSubject(subject) == "" {
		return "", "", "unresolved_identity"
	}
	return "", "", "unbound_channel"
}

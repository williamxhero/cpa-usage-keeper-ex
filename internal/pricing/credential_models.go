package pricing

import (
	"fmt"
	"sort"
	"strings"
)

// OverrideConfig extends complete-candidate compilation without changing callers
// that have only legacy prices or credential defaults.
type OverrideConfig struct {
	CredentialModels []CredentialModelConfig
}

type CredentialModelConfig struct {
	SubjectID  string
	Model      string
	Multiplier float64
}

type credentialModelKey struct{ subjectID, model string }

func (s *Snapshot) compileCredentialModels(configs []CredentialModelConfig) error {
	s.credentialModels = make(map[credentialModelKey]float64, len(configs))
	for _, config := range configs {
		if !s.HasCredentialSubject(config.SubjectID) {
			return fmt.Errorf("credential model references missing subject")
		}
		model := strings.TrimSpace(config.Model)
		if model == "" || !isNonNegativeFinite(config.Multiplier) {
			return fmt.Errorf("credential model and finite non-negative multiplier required")
		}
		key := credentialModelKey{config.SubjectID, model}
		if _, exists := s.credentialModels[key]; exists {
			return fmt.Errorf("duplicate credential model")
		}
		// Override selection is independent of baseline Model/Alias selection.
		// Any existing baseline can be paired with this exception by request
		// evidence; future baseline saves are validated by this same compilation.
		for _, baselineModel := range s.modelsByName {
			baseline := unadjustedPricing(baselineModel)
			if err := validateWorstCaseCost(baseline, nil); err != nil {
				return fmt.Errorf("unsafe unadjusted baseline: %w", err)
			}
			baseline.PriceMultiplier = &config.Multiplier
			if err := validateWorstCaseCost(baseline, nil); err != nil {
				return fmt.Errorf("unsafe credential model multiplier: %w", err)
			}
		}
		s.credentialModels[key] = config.Multiplier
	}
	return nil
}

func (s *Snapshot) CredentialModel(id, model string) (float64, bool) {
	if s == nil {
		return 0, false
	}
	value, exists := s.credentialModels[credentialModelKey{id, model}]
	return value, exists
}

// CredentialModelConfigs returns value copies in deterministic model order.
func (s *Snapshot) CredentialModelConfigs(id string) []CredentialModelConfig {
	result := []CredentialModelConfig{}
	if s == nil {
		return result
	}
	for key, multiplier := range s.credentialModels {
		if key.subjectID == id {
			result = append(result, CredentialModelConfig{SubjectID: id, Model: key.model, Multiplier: multiplier})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Model < result[j].Model })
	return result
}

func (r Resolver) credentialSubject(subject CostSubject) string {
	if r.snapshot == nil {
		return ""
	}
	if subject.AuthType == "" && !subject.ObservedIdentity {
		return r.snapshot.credentialIndexes[subject.IdentityAuthIndex]
	}
	return r.snapshot.credentials[credentialIdentity{subject.AuthType, subject.IdentityAuthIndex}]
}

func (r Resolver) credentialModel(subject CostSubject) (id string, multiplier float64, model, by string, selected bool) {
	id = r.credentialSubject(subject)
	if multiplier, selected = r.snapshot.CredentialModel(id, subject.Dimensions.Model); selected {
		return id, multiplier, subject.Dimensions.Model, "model", true
	}
	if multiplier, selected = r.snapshot.CredentialModel(id, subject.Dimensions.ModelAlias); selected {
		return id, multiplier, subject.Dimensions.ModelAlias, "model_alias", true
	}
	return id, 0, "", "", false
}

func (r Resolver) HasPricingOverrides() bool {
	return r.snapshot != nil && (len(r.snapshot.credentialDefaults) > 0 || len(r.snapshot.credentialModels) > 0)
}
func (r Resolver) UsesPricingOverride(subject CostSubject) bool {
	if _, _, _, _, ok := r.credentialModel(subject); ok {
		return true
	}
	return r.UsesCredentialDefault(subject)
}

// A conservative retained-evidence guard, not an identity selection. Model-only
// configuration still needs typed facts even with no credential defaults.
func (r Resolver) MayUsePricingOverride(subject CostSubject) bool {
	if r.snapshot == nil {
		return false
	}
	for key, id := range r.snapshot.credentials {
		if strings.TrimSpace(key.authIndex) != subject.Dimensions.AuthIndex {
			continue
		}
		if _, ok := r.snapshot.credentialDefaults[id]; ok {
			return true
		}
		if _, ok := r.snapshot.CredentialModel(id, subject.Dimensions.Model); ok {
			return true
		}
		if _, ok := r.snapshot.CredentialModel(id, subject.Dimensions.ModelAlias); ok {
			return true
		}
	}
	return false
}

package pricing

import (
	"fmt"
	"sort"
	"strings"
)

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

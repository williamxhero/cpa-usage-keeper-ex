package pricing

import (
	"fmt"
	"sort"
	"strings"

	"cpa-usage-keeper/internal/entities"
)

const CredentialMatrixSource = "credential_matrix"

type CredentialMatrixConfig struct {
	SubjectID        string
	CanonicalModelID string
	Fixed            FixedTariff
	Source           string
}

// CanonicalCredentialModelID defines the persisted matrix key without guessing
// across providers or model namespaces.
func CanonicalCredentialModelID(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func (s *Snapshot) SetCredentialMatrix(configs []CredentialMatrixConfig) error {
	if s == nil {
		return fmt.Errorf("snapshot is nil")
	}
	compiled := make(map[credentialModelKey]CredentialMatrixConfig, len(configs))
	for index, config := range configs {
		config.SubjectID = strings.TrimSpace(config.SubjectID)
		config.CanonicalModelID = CanonicalCredentialModelID(config.CanonicalModelID)
		if config.SubjectID == "" || config.CanonicalModelID == "" {
			return fmt.Errorf("credential matrix row %d requires subject and model", index)
		}
		if !s.HasCredentialSubject(config.SubjectID) {
			return fmt.Errorf("credential matrix row %d references missing subject", index)
		}
		if err := config.Fixed.Validate(); err != nil {
			return fmt.Errorf("credential matrix row %d: %w", index, err)
		}
		if config.Fixed.PricingStyle == "" {
			config.Fixed.PricingStyle = entities.ModelPricingStyleOpenAI
		}
		if strings.TrimSpace(config.Source) == "" {
			config.Source = CredentialMatrixSource
		}
		key := credentialModelKey{config.SubjectID, config.CanonicalModelID}
		if _, exists := compiled[key]; exists {
			return fmt.Errorf("duplicate credential matrix row")
		}
		config.Fixed = *cloneFixed(&config.Fixed)
		compiled[key] = config
	}
	s.matrixOverrides = compiled
	return nil
}

func (s *Snapshot) CredentialMatrixConfig(subjectID, model string) (CredentialMatrixConfig, bool) {
	if s == nil {
		return CredentialMatrixConfig{}, false
	}
	config, ok := s.matrixOverrides[credentialModelKey{subjectID, CanonicalCredentialModelID(model)}]
	if !ok {
		return CredentialMatrixConfig{}, false
	}
	config.Fixed = *cloneFixed(&config.Fixed)
	return config, true
}

func (s *Snapshot) CredentialMatrixConfigs() []CredentialMatrixConfig {
	if s == nil {
		return []CredentialMatrixConfig{}
	}
	result := make([]CredentialMatrixConfig, 0, len(s.matrixOverrides))
	for _, config := range s.matrixOverrides {
		config.Fixed = *cloneFixed(&config.Fixed)
		result = append(result, config)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SubjectID == result[j].SubjectID {
			return result[i].CanonicalModelID < result[j].CanonicalModelID
		}
		return result[i].SubjectID < result[j].SubjectID
	})
	return result
}

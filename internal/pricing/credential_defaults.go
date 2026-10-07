package pricing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"cpa-usage-keeper/internal/entities"
)

var decimalMultiplier = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)$`)

// ParseMultiplier accepts decimal notation only. Blank is not inheritance: callers
// must explicitly delete an override. Keeping this at the domain boundary also
// gives numeric JSON and form text the same validation.
func ParseMultiplier(text string) (float64, error) {
	text = strings.TrimSpace(text)
	percent := strings.HasSuffix(text, "%")
	if percent || strings.HasSuffix(text, "x") || strings.HasSuffix(text, "X") {
		text = text[:len(text)-1]
	}
	if !decimalMultiplier.MatchString(text) {
		return 0, fmt.Errorf("multiplier must be a non-negative decimal, optionally followed by x or %%")
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || !isNonNegativeFinite(value) {
		return 0, fmt.Errorf("multiplier must be finite")
	}
	if percent {
		value /= 100
	}
	return value, nil
}

// CredentialBinding contains exact internal evidence, never a downstream group
// or a label. None of these identity fields are exported by the pricing API.
type CredentialBinding struct {
	SubjectID string
	SafeName  string
	AuthType  string
	AuthIndex string
	Status    string
}

type CredentialConfig struct {
	SubjectID  string
	Multiplier float64
}

type credentialIdentity struct{ authType, authIndex string }

// CompileSnapshotWithCredentials validates the whole candidate, including
// unadjusted baseline safety even when legacy model multipliers are zero.
func CompileSnapshotWithCredentials(models []ModelConfig, bindings []CredentialBinding, defaults []CredentialConfig) (*Snapshot, error) {
	snapshot, err := CompileSnapshot(models)
	if err != nil {
		return nil, err
	}
	snapshot.credentials = make(map[credentialIdentity]string)
	snapshot.credentialIndexes = make(map[string]string)
	snapshot.credentialDefaults = make(map[string]float64)
	snapshot.credentialSubjects = make(map[string]string)
	byIndex := make(map[string][]CredentialBinding)
	for _, binding := range bindings {
		// A saved subject remains configurable even if its attribution becomes
		// unknown. Invalid evidence disables selection, not subject existence.
		if binding.SubjectID != "" {
			name := binding.SafeName
			if name == "" {
				name = "Credential"
			}
			snapshot.credentialSubjects[binding.SubjectID] = name
		}
		if binding.AuthIndex == "" || (binding.AuthType != "apikey" && binding.AuthType != "oauth") {
			continue
		}
		byIndex[binding.AuthIndex] = append(byIndex[binding.AuthIndex], binding)
	}
	for index, records := range byIndex {
		for _, binding := range records {
			key := credentialIdentity{binding.AuthType, index}
			count := 0
			for _, other := range records {
				if other.AuthType == binding.AuthType {
					count++
				}
			}
			if count == 1 && binding.Status == "unique" && binding.SubjectID != "" {
				snapshot.credentials[key] = binding.SubjectID
			}
		}
		// Rollups have no type. Even an unbound claim in the other type makes
		// index-only attribution unsafe.
		if len(records) == 1 && records[0].Status == "unique" && records[0].SubjectID != "" {
			snapshot.credentialIndexes[index] = records[0].SubjectID
		}
	}
	for _, config := range defaults {
		if _, ok := snapshot.credentialSubjects[config.SubjectID]; !ok {
			return nil, fmt.Errorf("credential default references missing subject")
		}
		if !isNonNegativeFinite(config.Multiplier) {
			return nil, fmt.Errorf("credential multiplier must be finite and non-negative")
		}
		if _, exists := snapshot.credentialDefaults[config.SubjectID]; exists {
			return nil, fmt.Errorf("duplicate credential default")
		}
		for _, model := range snapshot.modelsByName {
			baseline := unadjustedPricing(model)
			if err := validateWorstCaseCost(baseline, nil); err != nil {
				return nil, fmt.Errorf("unsafe unadjusted baseline: %w", err)
			}
			baseline.PriceMultiplier = &config.Multiplier
			if err := validateWorstCaseCost(baseline, nil); err != nil {
				return nil, fmt.Errorf("unsafe credential multiplier: %w", err)
			}
		}
		snapshot.credentialDefaults[config.SubjectID] = config.Multiplier
	}
	snapshot.legacyActiveFields = snapshot.activeFields
	if len(defaults) > 0 {
		// Retained-event reconciliation must identify the original rollup cohorts,
		// including dimensions not used by current legacy rules.
		for field := RuleFieldAPIGroupKey; field < ruleFieldCount; field++ {
			snapshot.activeFields = snapshot.activeFields.with(field)
		}
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	snapshot.id = hex.EncodeToString(id)
	return snapshot, nil
}

func (s *Snapshot) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}
func (s *Snapshot) CredentialDefault(id string) (float64, bool) {
	if s == nil {
		return 0, false
	}
	value, ok := s.credentialDefaults[id]
	return value, ok
}

// CredentialName returns only sanitized display evidence copied at compilation.
func (s *Snapshot) CredentialName(id string) string {
	if s == nil {
		return ""
	}
	return s.credentialSubjects[id]
}

func (s *Snapshot) HasCredentialSubject(id string) bool {
	if s == nil {
		return false
	}
	_, ok := s.credentialSubjects[id]
	return ok
}
func (r Resolver) HasCredentialDefaults() bool {
	return r.snapshot != nil && len(r.snapshot.credentialDefaults) > 0
}
func (r Resolver) SnapshotID() string { return r.snapshot.ID() }
func (r Resolver) LegacyActiveFields() ActiveFields {
	if r.snapshot == nil {
		return 0
	}
	if r.HasCredentialDefaults() {
		return r.snapshot.legacyActiveFields
	}
	return r.snapshot.activeFields
}
func (r Resolver) credentialDefault(subject CostSubject) (string, float64, bool) {
	if r.snapshot == nil {
		return "", 0, false
	}
	id := ""
	if subject.AuthType == "" && !subject.ObservedIdentity {
		id = r.snapshot.credentialIndexes[subject.IdentityAuthIndex]
	} else {
		id = r.snapshot.credentials[credentialIdentity{subject.AuthType, subject.IdentityAuthIndex}]
	}
	multiplier, ok := r.snapshot.credentialDefaults[id]
	return id, multiplier, ok
}

// UsesCredentialDefault permits query projections to choose exact retained
// event evidence without embedding another price engine.
func (r Resolver) UsesCredentialDefault(subject CostSubject) bool {
	_, _, ok := r.credentialDefault(subject)
	return ok
}

// MayUseCredentialDefault answers only whether an exact typed constituent
// could select an override; it does not attribute a type-less cohort.
func (r Resolver) MayUseCredentialDefault(subject CostSubject) bool {
	if r.snapshot == nil {
		return false
	}
	for key, id := range r.snapshot.credentials {
		// Existing rollups normalize optional dimensions. This is only a
		// conservative coverage guard, never an identity selection.
		if strings.TrimSpace(key.authIndex) == subject.Dimensions.AuthIndex {
			if _, ok := r.snapshot.credentialDefaults[id]; ok {
				return true
			}
		}
	}
	return false
}

// WithoutCredentialAttribution is a failure-safe immutable publication when
// directory commits succeeded but refreshing their evidence failed. Existing
// baseline prices/default configuration remain readable; no stale binding can
// select a credential fee until a successful complete refresh.
func (s *Snapshot) WithoutCredentialAttribution() *Snapshot {
	if s == nil {
		return &Snapshot{}
	}
	candidate := *s
	candidate.credentials = make(map[credentialIdentity]string)
	candidate.credentialIndexes = make(map[string]string)
	candidate.id = strings.TrimSuffix(s.id, "-unattributed") + "-unattributed"
	return &candidate
}

// SafeCredentialText is shared by directory DTOs and immutable pricing evidence.
// Untrusted metadata must not expose identities, lookup keys or credential syntax.
func SafeCredentialText(value string, identity entities.UsageIdentity) string {
	value = strings.TrimSpace(value)
	if strings.IndexFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(" -_.@():[]+", r)
	}) >= 0 || strings.Contains(strings.ToLower(value), "bearer ") {
		return ""
	}
	for _, segment := range strings.Fields(value) {
		lower := strings.ToLower(segment)
		if len(segment) >= 32 || strings.HasPrefix(segment, "eyJ") {
			return ""
		}
		for _, prefix := range []string{"sk-", "ghp_", "gho_", "github_pat_", "glpat-", "xoxb-", "xoxp-"} {
			if strings.Contains(lower, prefix) {
				return ""
			}
		}
	}
	secrets := []string{identity.LookupKey, identity.Identity}
	for _, file := range []*string{identity.FileName, identity.FilePath} {
		if file != nil && *file != "" {
			basename := path.Base(strings.ReplaceAll(*file, `\`, "/"))
			secrets = append(secrets, *file, basename, strings.TrimSuffix(basename, path.Ext(basename)))
		}
	}
	if endpoint, err := url.Parse(identity.BaseURL); err == nil {
		if endpoint.User != nil {
			secrets = append(secrets, endpoint.User.Username())
			if password, ok := endpoint.User.Password(); ok {
				secrets = append(secrets, password)
			}
		}
		for _, values := range endpoint.Query() {
			secrets = append(secrets, values...)
		}
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(value, secret) {
			return ""
		}
	}
	return value
}

func unadjustedPricing(model compiledModel) entities.ModelPriceSetting {
	baseline := model.pricing
	one := 1.0
	baseline.PriceMultiplier = &one
	return baseline
}

package repository

import (
	"context"
	"encoding/hex"
	"encoding/json"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"gorm.io/gorm"
)

// Credential indexes and rule values can contain private evidence. Only hashes
// are persisted; exact values are rehydrated privately from retained source rows.
type migrationSavedBinding struct{ SubjectID, AuthType, IndexHash, Status string }
type migrationSavedRule struct {
	Key, ValueHash string
	Multiplier     float64
}
type migrationSavedModel struct {
	Pricing entities.ModelPriceSetting
	Rules   []migrationSavedRule
}
type migrationSavedSnapshot struct {
	Strict    bool
	Models    []migrationSavedModel
	Bindings  []migrationSavedBinding
	Defaults  []pricing.CredentialConfig
	Overrides pricing.OverrideConfig
	Official  []entities.OfficialModelPrice
	Matrix    []pricing.CredentialMatrixConfig
}

func migrationPrivateHash(value string) string { return hex.EncodeToString(hashBytes(value)) }
func captureMigrationPreviousSnapshot(db *gorm.DB, mode string) (string, error) {
	pointer, err := pricingMigrationSnapshotBatch(db)
	if err != nil {
		return "", migrationError("snapshot_capture_failed")
	}
	if pointer != "" {
		var previous entities.PricingMigrationBatch
		if err := db.Where("id = ?", pointer).First(&previous).Error; err != nil || previous.PreviousSnapshotJSON == "" {
			return "", migrationError("snapshot_capture_failed")
		}
		return previous.PreviousSnapshotJSON, nil
	}
	snapshot, err := LoadPricingSnapshot(db.Statement.Context, db)
	if err != nil {
		return "", migrationError("snapshot_capture_failed")
	}
	identities, subjects, err := LoadCredentialPricingDirectory(db)
	if err != nil {
		return "", migrationError("snapshot_capture_failed")
	}
	saved := migrationSavedSnapshot{Strict: mode == strictPricingModeValue, Official: snapshot.OfficialPrices(), Matrix: snapshot.CredentialMatrixConfigs()}
	// Legacy loaders already adopt strict facts when either new catalog is populated.
	if len(saved.Official) > 0 || len(saved.Matrix) > 0 {
		saved.Strict = true
	}
	for _, config := range snapshot.ModelConfigs() {
		model := migrationSavedModel{Pricing: config.Pricing}
		for _, rule := range config.Rules {
			model.Rules = append(model.Rules, migrationSavedRule{Key: rule.Key, ValueHash: migrationPrivateHash(rule.Value), Multiplier: rule.Multiplier})
		}
		saved.Models = append(saved.Models, model)
	}
	for _, binding := range compileCredentialBindings(identities, subjects) {
		hash := ""
		if binding.AuthIndex != "" {
			hash = migrationPrivateHash(binding.AuthIndex)
		}
		saved.Bindings = append(saved.Bindings, migrationSavedBinding{SubjectID: binding.SubjectID, AuthType: binding.AuthType, IndexHash: hash, Status: binding.Status})
	}
	seen := map[string]bool{}
	for _, subject := range subjects {
		if seen[subject.ID] {
			continue
		}
		seen[subject.ID] = true
		if multiplier, ok := snapshot.CredentialDefault(subject.ID); ok {
			saved.Defaults = append(saved.Defaults, pricing.CredentialConfig{SubjectID: subject.ID, Multiplier: multiplier})
		}
		saved.Overrides.CredentialModels = append(saved.Overrides.CredentialModels, snapshot.CredentialModelConfigs(subject.ID)...)
	}
	for _, channel := range snapshot.Channels() {
		// Names do not affect pricing; avoid saving mutable or credential-bearing labels.
		channel.Name = "Channel"
		saved.Overrides.Channels = append(saved.Overrides.Channels, channel)
		saved.Overrides.ChannelModels = append(saved.Overrides.ChannelModels, snapshot.ChannelModelConfigs(channel.ID)...)
	}
	for i := range saved.Official {
		saved.Official[i] = safeMigrationOfficial(saved.Official[i])
	}
	data, err := json.Marshal(saved)
	if err != nil {
		return "", migrationError("snapshot_capture_failed")
	}
	return string(data), nil
}

// LoadPricingMigrationPreviousSnapshot reconstructs the exact saved pricing
// configuration while keeping raw identity evidence out of the durable snapshot.
// It is called by LoadPricingSnapshot when rollback selects a frozen batch.
func LoadPricingMigrationPreviousSnapshot(ctx context.Context, db *gorm.DB, batchID string) (*pricing.Snapshot, error) {
	query, err := migrationDB(ctx, db)
	if err != nil {
		return nil, err
	}
	if _, err = migrationBatchID(batchID); err != nil {
		return nil, err
	}
	var batch entities.PricingMigrationBatch
	if err = query.Where("id = ?", batchID).First(&batch).Error; err != nil {
		return nil, migrationError("previous_snapshot_invalid")
	}
	var saved migrationSavedSnapshot
	if batch.PreviousSnapshotJSON == "" || json.Unmarshal([]byte(batch.PreviousSnapshotJSON), &saved) != nil {
		return nil, migrationError("previous_snapshot_invalid")
	}
	indices := map[string]string{}
	// Read all retained exact relations, independently of the currently selected
	// mode. Association corrections and disabled bindings must survive rollback.
	var identities []entities.UsageIdentity
	if err = query.Select("identity").Find(&identities).Error; err != nil {
		return nil, migrationError("previous_snapshot_invalid")
	}
	for _, row := range identities {
		indices[migrationPrivateHash(row.Identity)] = row.Identity
	}
	var subjects []entities.CredentialPricingSubject
	if err = query.Select("identity").Find(&subjects).Error; err != nil {
		return nil, migrationError("previous_snapshot_invalid")
	}
	for _, row := range subjects {
		indices[migrationPrivateHash(row.Identity)] = row.Identity
	}
	var associations []entities.CredentialPricingAssociation
	if err = query.Select("identity").Find(&associations).Error; err != nil {
		return nil, migrationError("previous_snapshot_invalid")
	}
	for _, row := range associations {
		indices[migrationPrivateHash(row.Identity)] = row.Identity
	}
	bindings := make([]pricing.CredentialBinding, 0, len(saved.Bindings))
	for _, row := range saved.Bindings {
		index := ""
		if row.IndexHash != "" {
			var ok bool
			index, ok = indices[row.IndexHash]
			if !ok {
				return nil, migrationError("previous_snapshot_invalid")
			}
		}
		bindings = append(bindings, pricing.CredentialBinding{SubjectID: row.SubjectID, SafeName: "Credential", AuthType: row.AuthType, AuthIndex: index, Status: row.Status})
	}
	var rules []entities.ModelPriceRule
	ruleValues := map[string]string{}
	if !saved.Strict {
		if err = query.Select("value").Find(&rules).Error; err != nil {
			return nil, migrationError("previous_snapshot_invalid")
		}
		for _, rule := range rules {
			ruleValues[migrationPrivateHash(rule.Value)] = rule.Value
		}
	}
	configs := make([]pricing.ModelConfig, 0, len(saved.Models))
	for _, model := range saved.Models {
		config := pricing.ModelConfig{Pricing: model.Pricing}
		for _, rule := range model.Rules {
			value, ok := ruleValues[rule.ValueHash]
			if !ok && !saved.Strict {
				return nil, migrationError("previous_snapshot_invalid")
			}
			if !saved.Strict {
				config.Rules = append(config.Rules, pricing.RuleConfig{Key: rule.Key, Value: value, Multiplier: rule.Multiplier})
			}
		}
		configs = append(configs, config)
	}
	var snapshot *pricing.Snapshot
	if saved.Strict {
		snapshot, err = pricing.CompileSnapshotWithCredentials(nil, bindings, nil)
	} else {
		snapshot, err = pricing.CompileSnapshotWithCredentials(configs, bindings, saved.Defaults, saved.Overrides)
	}
	if err != nil {
		return nil, migrationError("previous_snapshot_invalid")
	}
	if saved.Strict {
		snapshot.SetOfficialPrices(saved.Official)
		if err = snapshot.SetCredentialMatrix(saved.Matrix); err != nil {
			return nil, migrationError("previous_snapshot_invalid")
		}
	}
	return snapshot, nil
}

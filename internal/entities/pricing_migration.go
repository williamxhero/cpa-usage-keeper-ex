package entities

import "time"

const (
	PricingMigrationKindLegacyPricing = "legacy_pricing"

	PricingMigrationStatusRunning    = "running"
	PricingMigrationStatusSucceeded  = "succeeded"
	PricingMigrationStatusFailed     = "failed"
	PricingMigrationStatusRolledBack = "rolled_back"

	PricingMigrationOutcomeMigrated = "migrated"
	PricingMigrationOutcomeSkipped  = "skipped"
	PricingMigrationOutcomeRejected = "rejected"
	PricingMigrationOutcomeConflict = "conflict"

	PricingMigrationCategoryModelPrices          = "model_price_settings"
	PricingMigrationCategoryCredentialFixed      = "credential_model_fixed"
	PricingMigrationCategoryCredentialMultiplier = "credential_model_multiplier"
	PricingMigrationCategoryCredentialDefaults   = "credential_price_defaults"
	PricingMigrationCategoryChannels             = "pricing_channels"
	PricingMigrationCategoryChannelMembers       = "pricing_channel_members"
	PricingMigrationCategoryChannelDefaults      = "channel_price_defaults"
	PricingMigrationCategoryChannelModels        = "channel_model_prices"
	PricingMigrationCategoryAssociations         = "credential_pricing_associations"
	PricingMigrationCategoryModelRules           = "model_price_rules"
)

// PricingMigrationBatch is the durable control record for one legacy-price
// adoption attempt. It contains only safe metadata and aggregate counters.
type PricingMigrationBatch struct {
	ID                  string `gorm:"primaryKey;size:96"`
	Kind                string `gorm:"not null;index"`
	Status              string `gorm:"not null;index"`
	PreviousPricingMode string `json:"-"`
	// Private frozen state is never part of an administrator report.
	PreviousSnapshotJSON    string `gorm:"type:text" json:"-"`
	PreviousSnapshotBatchID string `json:"-"`
	InputFrozen             bool   `gorm:"not null;default:false"`
	BatchSize               int    `gorm:"not null;default:250"`
	StagedCount             int    `gorm:"not null;default:0"`
	ImportedCount           int    `gorm:"not null;default:0"`
	SkippedCount            int    `gorm:"not null;default:0"`
	RejectedCount           int    `gorm:"not null;default:0"`
	ConflictCount           int    `gorm:"not null;default:0"`
	FailureReason           string
	StartedAt               time.Time  `gorm:"serializer:storageTime;not null"`
	FinishedAt              *time.Time `gorm:"serializer:storageTime"`
	CreatedAt               time.Time  `gorm:"serializer:storageTime;not null"`
	UpdatedAt               time.Time  `gorm:"serializer:storageTime;not null"`
}

// PricingMigrationBatchCount makes the report queryable without exposing a
// serialized source row or any credential-bearing metadata.
type PricingMigrationBatchCount struct {
	BatchID   string `gorm:"primaryKey;size:96"`
	Category  string `gorm:"primaryKey;size:64"`
	Migrated  int    `gorm:"not null;default:0"`
	Skipped   int    `gorm:"not null;default:0"`
	Rejected  int    `gorm:"not null;default:0"`
	Conflicts int    `gorm:"not null;default:0"`
}

// PricingMigrationItem is a safe per-source-row audit record. SourceRef and
// TargetRef are opaque/deterministic references; raw keys, endpoints and paths
// are deliberately never persisted in a migration report.
type PricingMigrationItem struct {
	ID                int64  `gorm:"primaryKey"`
	BatchID           string `gorm:"not null;index;uniqueIndex:uniq_pricing_migration_items,priority:1;size:96"`
	Category          string `gorm:"not null;index;uniqueIndex:uniq_pricing_migration_items,priority:2;size:64"`
	SourceRef         string `gorm:"not null;uniqueIndex:uniq_pricing_migration_items,priority:3;size:160"`
	TargetRef         string `gorm:"size:160"`
	Outcome           string `gorm:"not null;index;size:16"`
	Reason            string `gorm:"size:255"`
	SourceFingerprint string `gorm:"size:64"`
	TargetFingerprint string `gorm:"size:64"`
	// InputJSON contains only pricing candidates and opaque references, never
	// serialized credential directory rows. It survives retry and is private.
	InputJSON string    `gorm:"type:text" json:"-"`
	Sequence  int64     `gorm:"not null;default:0;index" json:"-"`
	Staged    bool      `gorm:"not null;default:false" json:"-"`
	CreatedAt time.Time `gorm:"serializer:storageTime;not null"`
	UpdatedAt time.Time `gorm:"serializer:storageTime;not null"`
}

// PricingMigrationLegacyState records why a retained legacy row is inactive.
// The original legacy tables remain untouched and are the rollback source.
type PricingMigrationLegacyState struct {
	ID        int64     `gorm:"primaryKey"`
	BatchID   string    `gorm:"not null;index;uniqueIndex:uniq_pricing_legacy_state,priority:1;size:96"`
	Category  string    `gorm:"not null;uniqueIndex:uniq_pricing_legacy_state,priority:2;size:64"`
	SourceRef string    `gorm:"not null;uniqueIndex:uniq_pricing_legacy_state,priority:3;size:160"`
	Inactive  bool      `gorm:"not null;default:true"`
	Reason    string    `gorm:"not null;size:255"`
	CreatedAt time.Time `gorm:"serializer:storageTime;not null"`
	UpdatedAt time.Time `gorm:"serializer:storageTime;not null"`
}

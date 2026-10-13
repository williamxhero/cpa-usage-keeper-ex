package repository

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"gorm.io/gorm"
)

// Each SELECT and INSERT is bounded by BatchSize. The freeze transaction gives
// all pages one consistent source view; no target facts are written here.
func migrationSourcePages[T any](db *gorm.DB, size int, order string, columns []string, visit func([]T) error) error {
	for offset := 0; ; offset += size {
		var rows []T
		q := db.Order(order).Limit(size).Offset(offset)
		if len(columns) > 0 {
			q = q.Select(columns)
		}
		if err := q.Find(&rows).Error; err != nil {
			return migrationError("input_read_failed")
		}
		if len(rows) == 0 {
			return nil
		}
		if err := visit(rows); err != nil {
			return err
		}
	}
}
func freezePricingMigration(db *gorm.DB, batch *entities.PricingMigrationBatch) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var current entities.PricingMigrationBatch
		if err := tx.Where("id = ?", batch.ID).First(&current).Error; err != nil {
			return migrationError("batch_read_failed")
		}
		if current.InputFrozen {
			*batch = current
			return nil
		}
		mode, pointer, err := CurrentPricingMigrationState(tx)
		if err != nil {
			return migrationError("mode_read_failed")
		}
		if mode != "" && mode != strictPricingModeValue {
			return migrationError("mode_read_failed")
		}
		batch.PreviousPricingMode, batch.PreviousSnapshotBatchID = mode, pointer
		// Partial freezes are never committed. Re-entering after process termination
		// either sees the complete frozen batch or starts this transaction again.
		var sequence int64
		add := func(plan migrationPlan) error {
			sequence++
			// The source ordinal disambiguates physically duplicated rows, while
			// TargetRef retains normalized identity for conflict auditing.
			plan.SourceRef = safeMigrationRef(plan.Category, plan.SourceRef+fmt.Sprintf("\x00%d", sequence))
			if plan.Outcome == "" {
				plan.Outcome = entities.PricingMigrationOutcomeMigrated
			}
			item, err := migrationItem(plan, batch.ID, sequence)
			if err != nil {
				return err
			}
			if err := tx.Create(&item).Error; err != nil {
				return migrationError("stage_write_failed")
			}
			return nil
		}
		official := map[string]entities.OfficialModelPrice{}
		ambiguousOfficial := map[string]bool{}
		targetConflicts := map[string]migrationPlan{}
		if err := migrationSourcePages[entities.OfficialModelPrice](tx, batch.BatchSize, "canonical_model_id asc, id asc", nil, func(rows []entities.OfficialModelPrice) error {
			for _, row := range rows {
				key := canonicalCredentialModelID(row.CanonicalModelID)
				if old, ok := official[key]; ok {
					if !sameOfficialMigrationPrice(old, row) && !ambiguousOfficial[key] {
						ambiguousOfficial[key] = true
						targetConflicts[key] = migrationPlan{Category: entities.PricingMigrationCategoryModelPrices, SourceRef: "target_catalog\x00" + key, TargetRef: safeMigrationRef("official", key), Outcome: entities.PricingMigrationOutcomeConflict, Reason: "target_conflict", SourceFP: migrationFingerprint(safeMigrationOfficial(row)), TargetFP: migrationFingerprint(old)}
					}
					continue
				}
				official[key] = safeMigrationOfficial(row)
			}
			return nil
		}); err != nil {
			return err
		}
		// Official rows (including legacy-seed) are retained targets and multiplier
		// bases, never extra source items. Duplicate targets must not synthesize a
		// model_price_settings rejection when the legacy source table is empty.
		if err := migrationSourcePages[entities.ModelPriceSetting](tx, batch.BatchSize, "model asc, id asc", modelPriceSettingColumns, func(rows []entities.ModelPriceSetting) error {
			for _, row := range rows {
				model := strings.TrimSpace(row.Model)
				// A source row already audits this target; do not add a second
				// catalog-only conflict for the same canonical group.
				delete(targetConflicts, canonicalCredentialModelID(model))
				style, e := normalizeModelPricingStyle(row.PricingStyle)
				plan := migrationPlan{Category: entities.PricingMigrationCategoryModelPrices, SourceRef: fmt.Sprintf("%d", row.ID), TargetRef: safeMigrationRef("official", canonicalCredentialModelID(model))}
				price := entities.OfficialModelPrice{CanonicalModelID: model, DisplayModelID: model, Source: entities.OfficialModelPriceSourceLegacySeed, PricingStyle: style, PromptPricePer1M: floatPtr(row.PromptPricePer1M), CompletionPricePer1M: floatPtr(row.CompletionPricePer1M), CacheReadPricePer1M: floatPtr(row.CacheReadPricePer1M), CacheWritePricePer1M: floatPtr(row.CacheWritePricePer1M)}
				switch {
				case model == "":
					plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeRejected, "missing_model"
				case e != nil:
					plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeRejected, "invalid_style"
				case !completeMigrationOfficial(price):
					plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeRejected, "invalid_price"
				default:
					plan.Official = &price
					plan.SourceFP = migrationFingerprint(price)
					if old, ok := official[canonicalCredentialModelID(model)]; ok {
						plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeConflict, "target_conflict"
						plan.TargetFP = migrationFingerprint(old)
						if sameOfficialMigrationPrice(old, price) && !ambiguousOfficial[canonicalCredentialModelID(model)] {
							plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeSkipped, "target_exists"
						}
					} else {
						official[canonicalCredentialModelID(model)] = price
					}
				}
				if err := add(plan); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		// Differing existing targets still need an audit when there is no legacy
		// source for that group. These conflict-only items never import a seed.
		keys := make([]string, 0, len(targetConflicts))
		for key := range targetConflicts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := add(targetConflicts[key]); err != nil {
				return err
			}
		}
		subjects := map[string]bool{}
		if err := migrationSourcePages[entities.CredentialPricingSubject](tx, batch.BatchSize, "id asc", []string{"id"}, func(rows []entities.CredentialPricingSubject) error {
			for _, row := range rows {
				subjects[row.ID] = true
			}
			return nil
		}); err != nil {
			return err
		}
		if err := migrationSourcePages[entities.CredentialModelMultiplier](tx, batch.BatchSize, "subject_id asc, model asc", nil, func(rows []entities.CredentialModelMultiplier) error {
			for _, row := range rows {
				category := entities.PricingMigrationCategoryCredentialMultiplier
				if row.Mode == pricing.ModeFixed {
					category = entities.PricingMigrationCategoryCredentialFixed
				}
				subject, model := strings.TrimSpace(row.SubjectID), canonicalCredentialModelID(row.Model)
				plan := migrationPlan{Category: category, SourceRef: row.SubjectID + "\x00" + row.Model, TargetRef: safeMigrationRef("matrix", subject+"\x00"+model)}
				var price entities.CredentialModelPriceOverride
				switch {
				case subject == "" || !subjects[subject]:
					plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeRejected, "missing_subject"
				case model == "":
					plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeRejected, "missing_model"
				case row.Mode != pricing.ModeFixed && row.Mode != "" && row.Mode != pricing.ModeMultiplier:
					plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeRejected, "invalid_mode"
				case row.Mode == pricing.ModeFixed:
					style, e := normalizeModelPricingStyle(row.PricingStyle)
					if e != nil {
						plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeRejected, "invalid_style"
					} else if !allFiniteMigrationPointers([]*float64{row.PromptPricePer1M, row.CompletionPricePer1M, row.CacheReadPricePer1M, row.CacheWritePricePer1M}) {
						plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeRejected, "complete_fixed_tariff_required"
					} else {
						price = entities.CredentialModelPriceOverride{SubjectID: subject, CanonicalModelID: model, PricingStyle: style, PromptPricePer1M: *row.PromptPricePer1M, CompletionPricePer1M: *row.CompletionPricePer1M, CacheReadPricePer1M: *row.CacheReadPricePer1M, CacheWritePricePer1M: *row.CacheWritePricePer1M, Source: "migrated_from_fixed"}
						plan.Matrix = &price
					}
				case !finiteMigrationPrice(row.Multiplier):
					plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeRejected, "invalid_multiplier"
				default:
					base, known := official[model]
					if !known {
						plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeSkipped, "missing_official_model"
					} else if ambiguousOfficial[model] {
						plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeConflict, "target_conflict"
						plan.TargetFP = migrationFingerprint(base)
					} else if !completeMigrationOfficial(base) {
						plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeSkipped, "official_price_missing_segment"
					} else {
						price = entities.CredentialModelPriceOverride{SubjectID: subject, CanonicalModelID: model, PricingStyle: base.PricingStyle, PromptPricePer1M: *base.PromptPricePer1M * row.Multiplier, CompletionPricePer1M: *base.CompletionPricePer1M * row.Multiplier, CacheReadPricePer1M: *base.CacheReadPricePer1M * row.Multiplier, CacheWritePricePer1M: *base.CacheWritePricePer1M * row.Multiplier, Source: "migrated_from_multiplier"}
						if !validMigrationMatrix(price) {
							plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeRejected, "materialized_price_overflow"
						} else {
							plan.Matrix = &price
						}
					}
				}
				if plan.Matrix != nil {
					plan.SourceFP = migrationFingerprint(*plan.Matrix)
					var err error
					plan, err = classifyMigrationMatrix(tx, plan, false)
					if err != nil {
						return err
					}
				}
				// Skipped/rejected rows still freeze their outcome and opaque source identity.
				if plan.SourceFP == "" {
					plan.SourceFP = migrationFingerprint(struct {
						Mode   string
						Reason string
					}{row.Mode, plan.Reason})
				}
				if err := add(plan); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		if err := freezeInactiveMigrationRows(tx, batch.BatchSize, add); err != nil {
			return err
		}
		snapshotJSON, err := captureMigrationPreviousSnapshot(tx, batch.PreviousPricingMode)
		// Invalid source facts still deserve a durable frozen audit. A successful
		// publication requires the previous snapshot to be restorable.
		if err != nil {
			snapshotJSON = ""
		}
		if err := tx.Model(&entities.PricingMigrationBatch{}).Where("id = ?", batch.ID).Updates(map[string]any{"input_frozen": true, "previous_pricing_mode": mode, "previous_snapshot_batch_id": pointer, "previous_snapshot_json": snapshotJSON, "updated_at": time.Now()}).Error; err != nil {
			return migrationError("stage_write_failed")
		}
		batch.InputFrozen = true
		batch.PreviousSnapshotJSON = snapshotJSON
		return nil
	})
}
func freezeInactiveMigrationRows(db *gorm.DB, size int, add func(migrationPlan) error) error {
	inactive := func(category, key string) error {
		return add(migrationPlan{Category: category, SourceRef: key, SourceFP: migrationFingerprint(key), Outcome: entities.PricingMigrationOutcomeSkipped, Reason: "legacy_layer_disabled"})
	}
	if err := migrationSourcePages[entities.CredentialPriceDefault](db, size, "subject_id asc", []string{"subject_id"}, func(rows []entities.CredentialPriceDefault) error {
		for _, r := range rows {
			if err := inactive(entities.PricingMigrationCategoryCredentialDefaults, r.SubjectID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := migrationSourcePages[entities.PricingChannel](db, size, "id asc", []string{"id"}, func(rows []entities.PricingChannel) error {
		for _, r := range rows {
			if err := inactive(entities.PricingMigrationCategoryChannels, r.ID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := migrationSourcePages[entities.PricingChannelMember](db, size, "subject_id asc", []string{"subject_id", "channel_id"}, func(rows []entities.PricingChannelMember) error {
		for _, r := range rows {
			if err := inactive(entities.PricingMigrationCategoryChannelMembers, r.SubjectID+"\x00"+r.ChannelID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := migrationSourcePages[entities.ChannelPriceDefault](db, size, "channel_id asc", []string{"channel_id"}, func(rows []entities.ChannelPriceDefault) error {
		for _, r := range rows {
			if err := inactive(entities.PricingMigrationCategoryChannelDefaults, r.ChannelID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := migrationSourcePages[entities.ChannelModelPrice](db, size, "channel_id asc, model asc", []string{"channel_id", "model"}, func(rows []entities.ChannelModelPrice) error {
		for _, r := range rows {
			if err := inactive(entities.PricingMigrationCategoryChannelModels, r.ChannelID+"\x00"+r.Model); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := migrationSourcePages[entities.CredentialPricingAssociation](db, size, "id asc", []string{"id"}, func(rows []entities.CredentialPricingAssociation) error {
		for _, r := range rows {
			if err := inactive(entities.PricingMigrationCategoryAssociations, r.ID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return migrationSourcePages[entities.ModelPriceRule](db, size, "id asc", []string{"id"}, func(rows []entities.ModelPriceRule) error {
		for _, r := range rows {
			if err := inactive(entities.PricingMigrationCategoryModelRules, fmt.Sprintf("%d", r.ID)); err != nil {
				return err
			}
		}
		return nil
	})
}
func validMigrationMatrix(row entities.CredentialModelPriceOverride) bool {
	return row.SubjectID != "" && row.CanonicalModelID != "" && finiteMigrationPrice(row.PromptPricePer1M) && finiteMigrationPrice(row.CompletionPricePer1M) && finiteMigrationPrice(row.CacheReadPricePer1M) && finiteMigrationPrice(row.CacheWritePricePer1M) && (row.PricingStyle == entities.ModelPricingStyleOpenAI || row.PricingStyle == entities.ModelPricingStyleClaude)
}
func validateCurrentMigrationTargets(db *gorm.DB, size int) error {
	if size < 1 {
		size = defaultPricingMigrationBatchSize
	}
	if err := migrationSourcePages[entities.OfficialModelPrice](db, size, "canonical_model_id asc, id asc", nil, func(rows []entities.OfficialModelPrice) error {
		for _, row := range rows {
			if canonicalCredentialModelID(row.CanonicalModelID) == "" {
				return migrationError("candidate_invalid")
			}
			// The official snapshot can retain alias-duplicate targets. Validate
			// every value, but leave ambiguity handling to the existing resolver.
			if !finiteNonNegativePrices(row) {
				return migrationError("candidate_invalid")
			}
			if _, err := normalizeModelPricingStyle(row.PricingStyle); err != nil {
				return migrationError("candidate_invalid")
			}
		}
		return nil
	}); err != nil {
		return err
	}
	seen := map[string]bool{}
	return migrationSourcePages[entities.CredentialModelPriceOverride](db, size, "subject_id asc, canonical_model_id asc", nil, func(rows []entities.CredentialModelPriceOverride) error {
		for _, row := range rows {
			key := row.SubjectID + "\x00" + canonicalCredentialModelID(row.CanonicalModelID)
			if seen[key] {
				return migrationError("duplicate_canonical_target")
			}
			seen[key] = true
			if !validMigrationMatrix(row) {
				return migrationError("candidate_invalid")
			}
			var count int64
			if err := db.Model(&entities.CredentialPricingSubject{}).Where("id = ?", row.SubjectID).Count(&count).Error; err != nil {
				return migrationError("target_read_failed")
			}
			if count != 1 {
				return migrationError("candidate_invalid")
			}
		}
		return nil
	})
}

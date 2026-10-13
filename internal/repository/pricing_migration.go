package repository

import (
	"context"
	"cpa-usage-keeper/internal/pricing"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
)

const defaultPricingMigrationBatchSize = 250

var ErrPricingMigrationNotFound = errors.New("pricing_migration_not_found")
var migrationBatchIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$`)

// Publication is additionally serialized by the service catalog mutation mutex.
// This mutex protects direct repository callers and concurrent local resumes.
var pricingMigrationMutex sync.Mutex

type PricingMigrationOptions struct {
	BatchID   string
	BatchSize int
}
type PricingMigrationReport struct {
	Batch  entities.PricingMigrationBatch
	Counts []entities.PricingMigrationBatchCount
	Items  []entities.PricingMigrationItem
}

// migrationPlan is a frozen pricing-only input. Inactive rows contain only
// fingerprints; directory metadata, labels, endpoints and keys are never stored.
type migrationPlan struct {
	Category  string
	SourceRef string
	TargetRef string
	Outcome   string
	Reason    string
	SourceFP  string
	TargetFP  string
	Official  *entities.OfficialModelPrice           `json:",omitempty"`
	Matrix    *entities.CredentialModelPriceOverride `json:",omitempty"`
}

func migrationDB(ctx context.Context, db *gorm.DB) (*gorm.DB, error) {
	if db == nil {
		return nil, errors.New("database_nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).WithContext(ctx).Clauses(dbresolver.Write).Session(&gorm.Session{}), nil
}
func migrationBatchID(value string) (string, error) {
	if !migrationBatchIDPattern.MatchString(value) {
		return "", errors.New("invalid_batch_id")
	}
	return value, nil
}
func migrationError(code string) error { return errors.New(code) }

// RunLegacyPricingMigration freezes source inputs once. Staging commits bounded
// pages and can resume after a crash; target facts are published only at the end.
func RunLegacyPricingMigration(ctx context.Context, db *gorm.DB, options PricingMigrationOptions, publishers ...func(*pricing.Snapshot)) (PricingMigrationReport, error) {
	pricingMigrationMutex.Lock()
	defer pricingMigrationMutex.Unlock()
	query, err := migrationDB(ctx, db)
	if err != nil {
		return PricingMigrationReport{}, err
	}
	id := options.BatchID
	if id == "" {
		id = newPricingMigrationID()
	}
	if _, err = migrationBatchID(id); err != nil {
		return PricingMigrationReport{}, err
	}
	size := options.BatchSize
	if size <= 0 {
		size = defaultPricingMigrationBatchSize
	}
	if size > 1000 {
		size = 1000
	}
	var batch entities.PricingMigrationBatch
	err = query.Where("id = ?", id).First(&batch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = query.Transaction(func(tx *gorm.DB) error {
			mode, pointer, e := CurrentPricingMigrationState(tx)
			if e != nil {
				return migrationError("mode_read_failed")
			}
			batch = entities.PricingMigrationBatch{ID: id, Kind: entities.PricingMigrationKindLegacyPricing, Status: entities.PricingMigrationStatusRunning, PreviousPricingMode: mode, PreviousSnapshotBatchID: pointer, BatchSize: size, StartedAt: time.Now()}
			if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&batch).Error; e != nil {
				return migrationError("batch_create_failed")
			}
			if e = tx.Where("id = ?", id).First(&batch).Error; e != nil {
				return migrationError("batch_read_failed")
			}
			return nil
		})
	} else if err != nil {
		err = migrationError("batch_read_failed")
	}
	if err != nil {
		return PricingMigrationReport{}, err
	}
	if batch.Status == entities.PricingMigrationStatusSucceeded || batch.Status == entities.PricingMigrationStatusRolledBack {
		return GetPricingMigrationReport(ctx, query, id)
	}
	// Even a failed retry keeps every original frozen input and previous snapshot.
	if !batch.InputFrozen {
		err = freezePricingMigration(query, &batch)
		if err != nil {
			return failPricingMigration(ctx, query, id, err)
		}
	}
	if batch.Status == entities.PricingMigrationStatusFailed {
		if err = query.Model(&entities.PricingMigrationBatch{}).Where("id = ?", id).Updates(map[string]any{"status": entities.PricingMigrationStatusRunning, "failure_reason": "", "finished_at": nil, "updated_at": time.Now()}).Error; err != nil {
			return PricingMigrationReport{}, migrationError("batch_update_failed")
		}
	}
	if err = stagePricingMigration(query, &batch); err != nil {
		return failPricingMigration(ctx, query, id, err)
	}
	var candidate *pricing.Snapshot
	if err = publishPricingMigration(ctx, query, &batch, &candidate); err != nil {
		return failPricingMigration(ctx, query, id, err)
	}
	publishMigrationSnapshot(candidate, publishers)
	return GetPricingMigrationReport(ctx, query, id)
}
func ResumePricingMigration(ctx context.Context, db *gorm.DB, batchID string) (PricingMigrationReport, error) {
	// A resume never silently starts a new batch.
	if _, err := GetPricingMigrationReport(ctx, db, batchID); err != nil {
		return PricingMigrationReport{}, err
	}
	return RunLegacyPricingMigration(ctx, db, PricingMigrationOptions{BatchID: batchID})
}

func migrationItem(plan migrationPlan, batchID string, sequence int64) (entities.PricingMigrationItem, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return entities.PricingMigrationItem{}, migrationError("input_encoding_failed")
	}
	return entities.PricingMigrationItem{BatchID: batchID, Sequence: sequence, Category: plan.Category, SourceRef: plan.SourceRef, TargetRef: plan.TargetRef, Outcome: plan.Outcome, Reason: plan.Reason, SourceFingerprint: plan.SourceFP, TargetFingerprint: plan.TargetFP, InputJSON: string(data)}, nil
}
func readMigrationPlan(item entities.PricingMigrationItem) (migrationPlan, error) {
	var plan migrationPlan
	if item.InputJSON == "" || json.Unmarshal([]byte(item.InputJSON), &plan) != nil {
		return plan, migrationError("frozen_input_invalid")
	}
	return plan, nil
}
func migrationItemPages(db *gorm.DB, batch *entities.PricingMigrationBatch, visit func([]entities.PricingMigrationItem) error) error {
	var after int64
	for {
		var rows []entities.PricingMigrationItem
		if err := db.Where("batch_id = ? AND sequence > ?", batch.ID, after).Order("sequence asc").Limit(batch.BatchSize).Find(&rows).Error; err != nil {
			return migrationError("input_read_failed")
		}
		if len(rows) == 0 {
			return nil
		}
		if err := visit(rows); err != nil {
			return err
		}
		after = rows[len(rows)-1].Sequence
	}
}
func stagePricingMigration(db *gorm.DB, batch *entities.PricingMigrationBatch) error {
	return migrationItemPages(db, batch, func(rows []entities.PricingMigrationItem) error {
		return db.Transaction(func(tx *gorm.DB) error {
			var current entities.PricingMigrationBatch
			if err := tx.Where("id = ?", batch.ID).First(&current).Error; err != nil {
				return migrationError("batch_read_failed")
			}
			if current.Status == entities.PricingMigrationStatusSucceeded || current.Status == entities.PricingMigrationStatusRolledBack {
				return nil
			}
			for _, item := range rows {
				plan, err := readMigrationPlan(item)
				if err != nil {
					return err
				}
				// Duplicate sources are classified against the retained target during
				// final publication. Do not turn valid conflicts (or invalid source
				// reasons) into batch-fatal duplicate rejections here.
				if err := tx.Model(&entities.PricingMigrationItem{}).Where("id = ?", item.ID).Updates(map[string]any{"outcome": plan.Outcome, "reason": plan.Reason, "staged": true, "updated_at": time.Now()}).Error; err != nil {
					return migrationError("stage_write_failed")
				}
			}
			var staged int64
			if err := tx.Model(&entities.PricingMigrationItem{}).Where("batch_id = ? AND staged = ?", batch.ID, true).Count(&staged).Error; err != nil {
				return migrationError("stage_read_failed")
			}
			if err := tx.Model(&entities.PricingMigrationBatch{}).Where("id = ?", batch.ID).Update("staged_count", staged).Error; err != nil {
				return migrationError("stage_write_failed")
			}
			return nil
		})
	})
}
func publishPricingMigration(ctx context.Context, db *gorm.DB, batch *entities.PricingMigrationBatch, candidate **pricing.Snapshot) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var current entities.PricingMigrationBatch
		if err := tx.Where("id = ?", batch.ID).First(&current).Error; err != nil {
			return migrationError("batch_read_failed")
		}
		if current.Status == entities.PricingMigrationStatusSucceeded || current.Status == entities.PricingMigrationStatusRolledBack {
			return nil
		}
		var rejected int64
		if err := tx.Model(&entities.PricingMigrationItem{}).Where("batch_id = ? AND outcome = ?", batch.ID, entities.PricingMigrationOutcomeRejected).Count(&rejected).Error; err != nil {
			return migrationError("stage_read_failed")
		}
		if rejected > 0 {
			return migrationError("candidate_invalid")
		}
		if batch.PreviousSnapshotJSON == "" {
			return migrationError("previous_snapshot_invalid")
		}
		if err := validateCurrentMigrationTargets(tx, batch.BatchSize); err != nil {
			return err
		}
		if err := migrationItemPages(tx, batch, func(items []entities.PricingMigrationItem) error {
			for _, item := range items {
				plan, err := readMigrationPlan(item)
				if err != nil {
					return err
				}
				if plan.Official != nil {
					plan, err = classifyMigrationOfficial(tx, plan, true)
				}
				if plan.Matrix != nil {
					plan, err = classifyMigrationMatrix(tx, plan, true)
				}
				if err != nil {
					return err
				}
				if err := tx.Model(&entities.PricingMigrationItem{}).Where("id = ?", item.ID).Updates(map[string]any{"outcome": plan.Outcome, "reason": plan.Reason, "target_fingerprint": plan.TargetFP, "updated_at": time.Now()}).Error; err != nil {
					return migrationError("report_write_failed")
				}
				state := entities.PricingMigrationLegacyState{BatchID: batch.ID, Category: plan.Category, SourceRef: plan.SourceRef, Inactive: true, Reason: "legacy_layer_disabled"}
				if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "batch_id"}, {Name: "category"}, {Name: "source_ref"}}, DoUpdates: clause.AssignmentColumns([]string{"inactive", "reason", "updated_at"})}).Create(&state).Error; err != nil {
					return migrationError("state_write_failed")
				}
			}
			return nil
		}); err != nil {
			return err
		}
		if err := validateCurrentMigrationTargets(tx, batch.BatchSize); err != nil {
			return err
		}
		if err := ActivatePricingMigration(tx, batch.ID); err != nil {
			return migrationError("activation_failed")
		}
		snapshot, err := LoadPricingSnapshot(ctx, tx)
		if err != nil {
			return migrationError("candidate_invalid")
		}
		*candidate = snapshot
		return finishMigrationReport(tx, batch.ID, entities.PricingMigrationStatusSucceeded, "")
	})
}

func classifyMigrationOfficial(db *gorm.DB, plan migrationPlan, publish bool) (migrationPlan, error) {
	var found bool
	matches := true
	query := db.Where("LOWER(TRIM(canonical_model_id)) = ?", canonicalCredentialModelID(plan.Official.CanonicalModelID))
	if err := migrationSourcePages[entities.OfficialModelPrice](query, defaultPricingMigrationBatchSize, "canonical_model_id asc, id asc", nil, func(rows []entities.OfficialModelPrice) error {
		for _, row := range rows {
			if !found {
				plan.TargetFP = migrationFingerprint(safeMigrationOfficial(row))
				found = true
			}
			matches = matches && sameOfficialMigrationPrice(row, *plan.Official)
		}
		return nil
	}); err != nil {
		return plan, migrationError("target_read_failed")
	}
	if found {
		// Startup seeds can contain case/whitespace variants of one canonical
		// target. Preserve every existing tariff; a differing row is a conflict,
		// even if another existing row matches the proposed import.
		plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeConflict, "target_conflict"
		if matches {
			plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeSkipped, "target_exists"
		}
		return plan, nil
	}
	plan.Outcome, plan.Reason, plan.TargetFP = entities.PricingMigrationOutcomeMigrated, "", migrationFingerprint(*plan.Official)
	if publish {
		result := db.Omit("Aliases").Clauses(clause.OnConflict{DoNothing: true}).Create(plan.Official)
		if result.Error != nil {
			return plan, migrationError("target_write_failed")
		}
		if result.RowsAffected == 0 {
			return classifyMigrationOfficial(db, plan, false)
		}
	}
	return plan, nil
}
func classifyMigrationMatrix(db *gorm.DB, plan migrationPlan, publish bool) (migrationPlan, error) {
	var rows []entities.CredentialModelPriceOverride
	if err := db.Where("subject_id = ? AND LOWER(TRIM(canonical_model_id)) = ?", plan.Matrix.SubjectID, plan.Matrix.CanonicalModelID).Limit(2).Find(&rows).Error; err != nil {
		return plan, migrationError("target_read_failed")
	}
	if len(rows) > 1 {
		return plan, migrationError("duplicate_canonical_target")
	}
	if len(rows) == 1 {
		plan.TargetFP = migrationFingerprint(rows[0])
		plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeConflict, "target_conflict"
		if sameMatrixMigrationPrice(rows[0], *plan.Matrix) {
			plan.Outcome, plan.Reason = entities.PricingMigrationOutcomeSkipped, "target_exists"
		}
		return plan, nil
	}
	plan.Outcome, plan.Reason, plan.TargetFP = entities.PricingMigrationOutcomeMigrated, "", migrationFingerprint(*plan.Matrix)
	if publish {
		result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(plan.Matrix)
		if result.Error != nil {
			return plan, migrationError("target_write_failed")
		}
		if result.RowsAffected == 0 {
			return classifyMigrationMatrix(db, plan, false)
		}
	}
	return plan, nil
}
func finishMigrationReport(tx *gorm.DB, id, status, reason string) error {
	var groups []struct {
		Category string
		Outcome  string
		Total    int
	}
	if err := tx.Model(&entities.PricingMigrationItem{}).Select("category, outcome, COUNT(*) AS total").Where("batch_id = ?", id).Group("category, outcome").Scan(&groups).Error; err != nil {
		return migrationError("report_read_failed")
	}
	counts := map[string]*entities.PricingMigrationBatchCount{}
	var imported, skipped, rejected, conflicts int
	for _, group := range groups {
		count := counts[group.Category]
		if count == nil {
			count = &entities.PricingMigrationBatchCount{BatchID: id, Category: group.Category}
			counts[group.Category] = count
		}
		switch group.Outcome {
		case entities.PricingMigrationOutcomeMigrated:
			count.Migrated += group.Total
			imported += group.Total
		case entities.PricingMigrationOutcomeSkipped:
			count.Skipped += group.Total
			skipped += group.Total
		case entities.PricingMigrationOutcomeRejected:
			count.Rejected += group.Total
			rejected += group.Total
		case entities.PricingMigrationOutcomeConflict:
			count.Conflicts += group.Total
			conflicts += group.Total
		}
	}
	for _, count := range counts {
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(count).Error; err != nil {
			return migrationError("report_write_failed")
		}
	}
	if err := tx.Model(&entities.PricingMigrationBatch{}).Where("id = ?", id).Updates(map[string]any{"status": status, "failure_reason": reason, "imported_count": imported, "skipped_count": skipped, "rejected_count": rejected, "conflict_count": conflicts, "finished_at": time.Now(), "updated_at": time.Now()}).Error; err != nil {
		return migrationError("report_write_failed")
	}
	return nil
}
func failPricingMigration(ctx context.Context, db *gorm.DB, id string, cause error) (PricingMigrationReport, error) {
	// Cancellation must not erase the durable failure record.
	failureCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	query, _ := migrationDB(failureCtx, db)
	reason := safeMigrationError(cause)
	err := query.Transaction(func(tx *gorm.DB) error {
		var current entities.PricingMigrationBatch
		if err := tx.Where("id = ?", id).First(&current).Error; err != nil {
			return migrationError("batch_read_failed")
		}
		if current.Status == entities.PricingMigrationStatusSucceeded || current.Status == entities.PricingMigrationStatusRolledBack {
			return nil
		}
		if err := tx.Model(&entities.PricingMigrationItem{}).Where("batch_id = ? AND outcome = ?", id, entities.PricingMigrationOutcomeMigrated).Updates(map[string]any{"outcome": entities.PricingMigrationOutcomeRejected, "reason": "batch_rejected"}).Error; err != nil {
			return migrationError("report_write_failed")
		}
		return finishMigrationReport(tx, id, entities.PricingMigrationStatusFailed, reason)
	})
	if err != nil {
		return PricingMigrationReport{}, migrationError("failure_report_failed")
	}
	report, err := GetPricingMigrationReport(failureCtx, query, id)
	if err != nil {
		return report, err
	}
	if report.Batch.Status == entities.PricingMigrationStatusSucceeded || report.Batch.Status == entities.PricingMigrationStatusRolledBack {
		return report, nil
	}
	return report, migrationError(reason)
}

func GetPricingMigrationReport(ctx context.Context, db *gorm.DB, batchID string) (PricingMigrationReport, error) {
	query, err := migrationDB(ctx, db)
	if err != nil {
		return PricingMigrationReport{}, err
	}
	if _, err = migrationBatchID(batchID); err != nil {
		return PricingMigrationReport{}, err
	}
	var report PricingMigrationReport
	if err = query.Where("id = ?", batchID).First(&report.Batch).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return report, ErrPricingMigrationNotFound
	} else if err != nil {
		return PricingMigrationReport{}, migrationError("report_read_failed")
	}
	if err = query.Where("batch_id = ?", batchID).Order("category asc").Find(&report.Counts).Error; err != nil {
		return PricingMigrationReport{}, migrationError("report_read_failed")
	}
	// Never select the private source snapshot into a public report.
	if err = query.Omit("input_json").Where("batch_id = ?", batchID).Order("category asc, source_ref asc").Find(&report.Items).Error; err != nil {
		return PricingMigrationReport{}, migrationError("report_read_failed")
	}
	report.Batch.PreviousPricingMode = ""
	report.Batch.PreviousSnapshotJSON = ""
	report.Batch.PreviousSnapshotBatchID = ""
	sanitizePricingMigrationReport(&report)
	return report, nil
}
func ListPricingMigrationBatches(ctx context.Context, db *gorm.DB) ([]entities.PricingMigrationBatch, error) {
	query, err := migrationDB(ctx, db)
	if err != nil {
		return nil, err
	}
	var rows []entities.PricingMigrationBatch
	if err = query.Omit("previous_snapshot_json", "previous_snapshot_batch_id").Order("created_at desc, id desc").Find(&rows).Error; err != nil {
		return nil, migrationError("report_read_failed")
	}
	for index := range rows {
		report := PricingMigrationReport{Batch: rows[index]}
		sanitizePricingMigrationReport(&report)
		rows[index] = report.Batch
	}
	return rows, nil
}

func sanitizePricingMigrationReport(report *PricingMigrationReport) {
	if report == nil {
		return
	}
	if _, err := migrationBatchID(report.Batch.ID); err != nil {
		report.Batch.ID = "invalid"
	}
	report.Batch.PreviousPricingMode = ""
	report.Batch.PreviousSnapshotJSON = ""
	report.Batch.PreviousSnapshotBatchID = ""
	report.Batch.Kind = entities.PricingMigrationKindLegacyPricing
	switch report.Batch.Status {
	case entities.PricingMigrationStatusRunning, entities.PricingMigrationStatusSucceeded, entities.PricingMigrationStatusFailed, entities.PricingMigrationStatusRolledBack:
	default:
		report.Batch.Status = "unknown"
	}
	if !knownMigrationReason(report.Batch.FailureReason) {
		report.Batch.FailureReason = "redacted"
	}
	for index := range report.Counts {
		report.Counts[index].BatchID = report.Batch.ID
		if !knownMigrationCategory(report.Counts[index].Category) {
			report.Counts[index].Category = "unknown"
		}
	}
	for index := range report.Items {
		item := &report.Items[index]
		item.BatchID = report.Batch.ID
		item.InputJSON = ""
		if !knownMigrationCategory(item.Category) {
			item.Category = "unknown"
		}
		if !opaqueMigrationReference(item.SourceRef, item.Category) {
			item.SourceRef = "redacted"
		}
		if !opaqueMigrationReference(item.TargetRef, "") {
			item.TargetRef = "redacted"
		}
		if !knownMigrationOutcome(item.Outcome) {
			item.Outcome = "unknown"
		}
		if !knownMigrationReason(item.Reason) {
			item.Reason = "redacted"
		}
		if !migrationFingerprintValue(item.SourceFingerprint) {
			item.SourceFingerprint = ""
		}
		if !migrationFingerprintValue(item.TargetFingerprint) {
			item.TargetFingerprint = ""
		}
	}
}

func knownMigrationCategory(value string) bool {
	switch value {
	case entities.PricingMigrationCategoryModelPrices, entities.PricingMigrationCategoryCredentialFixed, entities.PricingMigrationCategoryCredentialMultiplier, entities.PricingMigrationCategoryCredentialDefaults, entities.PricingMigrationCategoryChannels, entities.PricingMigrationCategoryChannelMembers, entities.PricingMigrationCategoryChannelDefaults, entities.PricingMigrationCategoryChannelModels, entities.PricingMigrationCategoryAssociations, entities.PricingMigrationCategoryModelRules:
		return true
	default:
		return false
	}
}
func knownMigrationOutcome(value string) bool {
	return value == entities.PricingMigrationOutcomeMigrated || value == entities.PricingMigrationOutcomeSkipped || value == entities.PricingMigrationOutcomeRejected || value == entities.PricingMigrationOutcomeConflict
}
func knownMigrationReason(value string) bool {
	if value == "" {
		return true
	}
	switch value {
	case "missing_model", "missing_subject", "invalid_style", "invalid_price", "invalid_mode", "invalid_multiplier", "complete_fixed_tariff_required", "missing_official_model", "official_price_missing_segment", "materialized_price_overflow", "target_exists", "target_conflict", "legacy_layer_disabled", "candidate_invalid", "duplicate_canonical_target", "previous_snapshot_invalid", "input_encoding_failed", "input_read_failed", "frozen_input_invalid", "target_read_failed", "target_write_failed", "stage_write_failed", "stage_read_failed", "report_read_failed", "report_write_failed", "state_write_failed", "activation_failed", "snapshot_capture_failed", "mode_read_failed", "batch_update_failed", "migration_failed", "batch_rejected", "previous_snapshot_selected":
		return true
	default:
		return false
	}
}
func migrationFingerprintValue(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}
func opaqueMigrationReference(value, category string) bool {
	if value == "" && category == "" {
		return true
	}
	prefix, hash, found := strings.Cut(value, ":")
	if !found || len(hash) != 24 || (category != "" && prefix != category) {
		return false
	}
	if prefix != "official" && prefix != "matrix" && !knownMigrationCategory(prefix) {
		return false
	}
	for _, char := range hash {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func RollbackPricingMigration(ctx context.Context, db *gorm.DB, batchID string, publishers ...func(*pricing.Snapshot)) (PricingMigrationReport, error) {
	return switchPricingMigration(ctx, db, batchID, false, publishers)
}

// RestorePricingMigration selects current strict facts explicitly. It never
// reapplies frozen legacy rows over administrator edits made since publication.
func RestorePricingMigration(ctx context.Context, db *gorm.DB, batchID string, publishers ...func(*pricing.Snapshot)) (PricingMigrationReport, error) {
	return switchPricingMigration(ctx, db, batchID, true, publishers)
}
func switchPricingMigration(ctx context.Context, db *gorm.DB, id string, restore bool, publishers []func(*pricing.Snapshot)) (PricingMigrationReport, error) {
	pricingMigrationMutex.Lock()
	defer pricingMigrationMutex.Unlock()
	query, err := migrationDB(ctx, db)
	if err != nil {
		return PricingMigrationReport{}, err
	}
	if _, err = migrationBatchID(id); err != nil {
		return PricingMigrationReport{}, err
	}
	var candidate *pricing.Snapshot
	err = query.Transaction(func(tx *gorm.DB) error {
		var batch entities.PricingMigrationBatch
		if err := tx.Where("id = ?", id).First(&batch).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPricingMigrationNotFound
		} else if err != nil {
			return migrationError("batch_read_failed")
		}
		if batch.Status != entities.PricingMigrationStatusSucceeded && batch.Status != entities.PricingMigrationStatusRolledBack {
			return migrationError("batch_not_successful")
		}
		active, err := pricingMigrationReference(tx, strictPricingMigrationBatchKey)
		if err != nil || active != id {
			return migrationError("rollback_not_active")
		}
		pointer, err := pricingMigrationSnapshotBatch(tx)
		if err != nil {
			return migrationError("previous_snapshot_invalid")
		}
		if batch.Status == entities.PricingMigrationStatusRolledBack {
			expected := batch.PreviousSnapshotBatchID
			if expected == "" {
				expected = id
			}
			if pointer != expected {
				return migrationError("rollback_not_active")
			}
		} else if pointer != "" {
			return migrationError("rollback_not_active")
		}
		if (restore && batch.Status == entities.PricingMigrationStatusSucceeded) || (!restore && batch.Status == entities.PricingMigrationStatusRolledBack) {
			return nil
		}
		status := entities.PricingMigrationStatusRolledBack
		if restore {
			if err := validateCurrentMigrationTargets(tx, batch.BatchSize); err != nil {
				return err
			}
			if err := ActivatePricingMigration(tx, id); err != nil {
				return migrationError("activation_failed")
			}
			status = entities.PricingMigrationStatusSucceeded
		} else {
			if err := RestorePricingMigrationMode(tx, id, batch.PreviousPricingMode, batch.PreviousSnapshotBatchID); err != nil {
				return migrationError("rollback_not_active")
			}
		}
		candidate, err = LoadPricingSnapshot(ctx, tx)
		if err != nil {
			return migrationError("candidate_invalid")
		}
		inactive, reason := true, "legacy_layer_disabled"
		if !restore {
			var previous migrationSavedSnapshot
			if json.Unmarshal([]byte(batch.PreviousSnapshotJSON), &previous) != nil {
				return migrationError("previous_snapshot_invalid")
			}
			inactive, reason = previous.Strict, "previous_snapshot_selected"
		}
		if err := tx.Model(&entities.PricingMigrationLegacyState{}).Where("batch_id = ?", id).Updates(map[string]any{"inactive": inactive, "reason": reason, "updated_at": time.Now()}).Error; err != nil {
			return migrationError("state_write_failed")
		}
		if err := tx.Model(&entities.PricingMigrationBatch{}).Where("id = ?", id).Updates(map[string]any{"status": status, "updated_at": time.Now()}).Error; err != nil {
			return migrationError("batch_update_failed")
		}
		return nil
	})
	if err != nil {
		return PricingMigrationReport{}, err
	}
	publishMigrationSnapshot(candidate, publishers)
	return GetPricingMigrationReport(ctx, query, id)
}
func publishMigrationSnapshot(candidate *pricing.Snapshot, publishers []func(*pricing.Snapshot)) {
	if candidate == nil {
		return
	}
	for _, publish := range publishers {
		if publish != nil {
			publish(candidate)
		}
	}
}
func newPricingMigrationID() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "pmig_" + hex.EncodeToString(hashBytes(time.Now().String()))[:24]
	}
	return "pmig_" + hex.EncodeToString(data)
}
func hashBytes(value string) []byte { sum := sha256.Sum256([]byte(value)); return sum[:] }
func safeMigrationRef(category, value string) string {
	return category + ":" + hex.EncodeToString(hashBytes(value))[:24]
}
func migrationFingerprint(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(hashBytes(string(data)))
}
func finiteMigrationPrice(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
func floatPtr(value float64) *float64 { return &value }
func allFiniteMigrationPointers(values []*float64) bool {
	for _, v := range values {
		if v == nil || !finiteMigrationPrice(*v) {
			return false
		}
	}
	return true
}
func completeMigrationOfficial(row entities.OfficialModelPrice) bool {
	return allFiniteMigrationPointers([]*float64{row.PromptPricePer1M, row.CompletionPricePer1M, row.CacheReadPricePer1M, row.CacheWritePricePer1M})
}
func sameOfficialMigrationPrice(a, b entities.OfficialModelPrice) bool {
	return canonicalCredentialModelID(a.CanonicalModelID) == canonicalCredentialModelID(b.CanonicalModelID) && a.PricingStyle == b.PricingStyle && ptrEqual(a.PromptPricePer1M, b.PromptPricePer1M) && ptrEqual(a.CompletionPricePer1M, b.CompletionPricePer1M) && ptrEqual(a.CacheReadPricePer1M, b.CacheReadPricePer1M) && ptrEqual(a.CacheWritePricePer1M, b.CacheWritePricePer1M)
}
func sameMatrixMigrationPrice(a, b entities.CredentialModelPriceOverride) bool {
	return a.SubjectID == b.SubjectID && canonicalCredentialModelID(a.CanonicalModelID) == b.CanonicalModelID && a.PricingStyle == b.PricingStyle && a.PromptPricePer1M == b.PromptPricePer1M && a.CompletionPricePer1M == b.CompletionPricePer1M && a.CacheReadPricePer1M == b.CacheReadPricePer1M && a.CacheWritePricePer1M == b.CacheWritePricePer1M
}
func ptrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func safeMigrationError(err error) string {
	if err == nil {
		return ""
	}
	code := err.Error()
	switch code {
	case "candidate_invalid", "duplicate_canonical_target", "previous_snapshot_invalid", "input_encoding_failed", "input_read_failed", "frozen_input_invalid", "target_read_failed", "target_write_failed", "stage_write_failed", "stage_read_failed", "report_read_failed", "report_write_failed", "state_write_failed", "activation_failed", "snapshot_capture_failed", "mode_read_failed", "batch_update_failed":
		return code
	default:
		return "migration_failed"
	}
}

// Metadata fields may contain attacker-controlled URLs or labels. Frozen pricing
// inputs and fingerprints intentionally exclude them.
func safeMigrationOfficial(row entities.OfficialModelPrice) entities.OfficialModelPrice {
	row.SourceURL = ""
	row.ResponseHash = ""
	row.SourceVersion = ""
	row.FetchedAt = nil
	row.StaleAt = nil
	row.CreatedAt = time.Time{}
	row.UpdatedAt = time.Time{}
	row.ID = 0
	for i := range row.Aliases {
		row.Aliases[i] = entities.OfficialModelPriceAlias{Alias: row.Aliases[i].Alias}
	}
	return row
}

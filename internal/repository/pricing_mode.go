package repository

import (
	"encoding/json"
	"errors"
	"fmt"

	"cpa-usage-keeper/internal/entities"

	"gorm.io/gorm"
)

const strictPricingModeKey = "pricing_mode"
const strictPricingModeValue = `"three_layer"`
const strictPricingMigrationBatchKey = "pricing_migration_active_batch"
const pricingMigrationSnapshotKey = "pricing_migration_previous_snapshot"

var ErrPricingMigrationNotActive = errors.New("pricing migration batch is not active")

// ActivateStrictPricing records that the database has adopted SPEC-003 facts.
// An explicit administrator mutation supersedes migration rollback/restore
// pointers; even clearing the final matrix row must not resurrect legacy prices.
func ActivateStrictPricing(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := savePricingModeSetting(tx, strictPricingModeKey, strictPricingModeValue); err != nil {
			return err
		}
		return tx.Where("setting_key IN ?", []string{strictPricingMigrationBatchKey, pricingMigrationSnapshotKey}).Delete(&entities.AppSetting{}).Error
	})
}

// ActivatePricingMigration selects current strict facts and remembers the batch
// that owns the switch. The caller commits fact writes and this switch together.
func ActivatePricingMigration(db *gorm.DB, batchID string) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if _, err := migrationBatchID(batchID); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := savePricingModeSetting(tx, strictPricingModeKey, strictPricingModeValue); err != nil {
			return err
		}
		encoded, err := json.Marshal(batchID)
		if err != nil {
			return errors.New("invalid pricing migration reference")
		}
		if err := savePricingModeSetting(tx, strictPricingMigrationBatchKey, string(encoded)); err != nil {
			return err
		}
		return tx.Where("setting_key = ?", pricingMigrationSnapshotKey).Delete(&entities.AppSetting{}).Error
	})
}

// CurrentPricingMigrationState captures the persisted mode and any previous
// frozen snapshot selected by an earlier rollback.
func CurrentPricingMigrationState(db *gorm.DB) (string, string, error) {
	mode, err := pricingModeSetting(db, strictPricingModeKey)
	if err != nil {
		return "", "", err
	}
	previous, err := pricingMigrationSnapshotBatch(db)
	return mode, previous, err
}

// RestorePricingMigrationMode selects the saved pre-migration snapshot without
// modifying any pricing rows, including later administrator matrix writes.
func RestorePricingMigrationMode(db *gorm.DB, batchID, previousMode, previousSnapshotBatchID string) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		active, err := pricingMigrationReference(tx, strictPricingMigrationBatchKey)
		if err != nil {
			return err
		}
		if active != batchID || batchID == "" {
			return ErrPricingMigrationNotActive
		}
		if previousMode == "" {
			if err := tx.Where("setting_key = ?", strictPricingModeKey).Delete(&entities.AppSetting{}).Error; err != nil {
				return err
			}
		} else if err := savePricingModeSetting(tx, strictPricingModeKey, previousMode); err != nil {
			return err
		}
		previous := previousSnapshotBatchID
		if previous == "" {
			previous = batchID
		}
		encoded, err := json.Marshal(previous)
		if err != nil {
			return errors.New("invalid pricing migration reference")
		}
		return savePricingModeSetting(tx, pricingMigrationSnapshotKey, string(encoded))
	})
}

func pricingMigrationSnapshotBatch(db *gorm.DB) (string, error) {
	return pricingMigrationReference(db, pricingMigrationSnapshotKey)
}

func pricingMigrationReference(db *gorm.DB, key string) (string, error) {
	value, err := pricingModeSetting(db, key)
	if err != nil || value == "" {
		return "", err
	}
	var reference string
	if err := json.Unmarshal([]byte(value), &reference); err != nil || reference == "" {
		return "", errors.New("invalid pricing migration reference")
	}
	return reference, nil
}

func pricingModeSetting(db *gorm.DB, key string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("database is nil")
	}
	var setting entities.AppSetting
	err := db.Where("setting_key = ?", key).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("read pricing mode failed")
	}
	if setting.Value == nil {
		return "", nil
	}
	return *setting.Value, nil
}

func savePricingModeSetting(db *gorm.DB, key, value string) error {
	setting := entities.AppSetting{SettingKey: key, Value: &value, ValueType: entities.AppSettingValueTypeJSON}
	return db.Where("setting_key = ?", key).Assign(setting).FirstOrCreate(&setting).Error
}

// IsStrictPricingActivated reports the persisted SPEC-003 adoption mode.
func IsStrictPricingActivated(db *gorm.DB) bool {
	return strictPricingActivated(db)
}

func strictPricingActivated(db *gorm.DB) bool {
	value, err := pricingModeSetting(db, strictPricingModeKey)
	return err == nil && value == strictPricingModeValue
}

package test

import (
	"errors"
	"path/filepath"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPricingMigrationModeOwnershipAndAdministratorSupersession(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mode.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entities.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := repository.ActivatePricingMigration(db, "batch-first"); err != nil {
		t.Fatal(err)
	}
	if !repository.IsStrictPricingActivated(db) {
		t.Fatal("migration did not activate strict pricing")
	}
	if err := repository.RestorePricingMigrationMode(db, "batch-other", "", ""); !errors.Is(err, repository.ErrPricingMigrationNotActive) {
		t.Fatalf("unrelated rollback = %v", err)
	}
	if !repository.IsStrictPricingActivated(db) {
		t.Fatal("unrelated rollback changed mode")
	}
	if err := repository.RestorePricingMigrationMode(db, "batch-first", "", ""); err != nil {
		t.Fatal(err)
	}
	mode, previous, err := repository.CurrentPricingMigrationState(db)
	if err != nil || mode != "" || previous != "batch-first" {
		t.Fatalf("restored state = %q, %q, %v", mode, previous, err)
	}
	if err := repository.ActivateStrictPricing(db); err != nil {
		t.Fatal(err)
	}
	mode, previous, err = repository.CurrentPricingMigrationState(db)
	if err != nil || mode != `"three_layer"` || previous != "" {
		t.Fatalf("administrator state = %q, %q, %v", mode, previous, err)
	}
	if err := repository.RestorePricingMigrationMode(db, "batch-first", "", ""); !errors.Is(err, repository.ErrPricingMigrationNotActive) {
		t.Fatalf("stale rollback = %v", err)
	}
	if !repository.IsStrictPricingActivated(db) {
		t.Fatal("stale rollback undid administrator activation")
	}
}

func TestPricingMigrationModeSwitchRollsBackWithOuterTransaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mode-atomic.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entities.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("synthetic commit failure")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := repository.ActivatePricingMigration(tx, "batch-atomic"); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("transaction = %v", err)
	}
	mode, previous, err := repository.CurrentPricingMigrationState(db)
	if err != nil || mode != "" || previous != "" {
		t.Fatalf("failed transaction changed mode = %q, %q, %v", mode, previous, err)
	}
	if err := repository.RestorePricingMigrationMode(db, "batch-atomic", "", ""); !errors.Is(err, repository.ErrPricingMigrationNotActive) {
		t.Fatalf("failed transaction left active batch: %v", err)
	}
}

package repository

import (
	"fmt"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ListCredentialModelPriceOverrides(db *gorm.DB) ([]entities.CredentialModelPriceOverride, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	rows := make([]entities.CredentialModelPriceOverride, 0)
	if err := db.Order("subject_id asc, canonical_model_id asc").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list credential model price overrides: %w", err)
	}
	return rows, nil
}

func UpsertCredentialModelPriceOverrides(db *gorm.DB, rows []entities.CredentialModelPriceOverride) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if len(rows) == 0 {
		return nil
	}

	normalized := make([]entities.CredentialModelPriceOverride, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for index, row := range rows {
		row.SubjectID = strings.TrimSpace(row.SubjectID)
		row.CanonicalModelID = canonicalCredentialModelID(row.CanonicalModelID)
		if row.SubjectID == "" || row.CanonicalModelID == "" {
			return fmt.Errorf("credential model override %d requires subject and model", index)
		}
		key := row.SubjectID + "\x00" + row.CanonicalModelID
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate credential model override %q", key)
		}
		seen[key] = struct{}{}
		normalized[index] = row
	}
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "subject_id"}, {Name: "canonical_model_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"pricing_style", "prompt_price_per1_m", "completion_price_per1_m",
			"cache_read_price_per1_m", "cache_write_price_per1_m", "source", "updated_at",
		}),
	}).CreateInBatches(&normalized, 100).Error
	if err == nil {
		copy(rows, normalized)
	}
	return err
}

func canonicalCredentialModelID(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func DeleteCredentialModelPriceOverrides(db *gorm.DB, keys []struct{ SubjectID, CanonicalModelID string }) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, key := range keys {
			if err := tx.Where("subject_id = ? AND canonical_model_id = ?", strings.TrimSpace(key.SubjectID), canonicalCredentialModelID(key.CanonicalModelID)).Delete(&entities.CredentialModelPriceOverride{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

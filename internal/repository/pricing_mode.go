package repository

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"

	"gorm.io/gorm"
)

const strictPricingModeKey = "pricing_mode"
const strictPricingModeValue = `"three_layer"`

// ActivateStrictPricing records that the database has adopted SPEC-003 facts.
// It survives clearing the last matrix row, so a restart cannot resurrect the
// legacy pricing layers after an explicit matrix or official-price mutation.
func ActivateStrictPricing(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	value := strictPricingModeValue
	setting := entities.AppSetting{SettingKey: strictPricingModeKey, Value: &value, ValueType: entities.AppSettingValueTypeJSON}
	return db.Where("setting_key = ?", strictPricingModeKey).Assign(setting).FirstOrCreate(&setting).Error
}

func strictPricingActivated(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	var setting entities.AppSetting
	if err := db.Where("setting_key = ? AND value = ?", strictPricingModeKey, strictPricingModeValue).First(&setting).Error; err != nil {
		return false
	}
	return true
}

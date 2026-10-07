package entities

import "time"

// CredentialModelMultiplier is an exact model exception, independent of legacy
// prices/rules. Removing the row restores inheritance; zero is an active choice.
type CredentialModelMultiplier struct {
	SubjectID  string    `gorm:"primaryKey"`
	Model      string    `gorm:"primaryKey"`
	Multiplier float64   `gorm:"not null"`
	UpdatedAt  time.Time `gorm:"serializer:storageTime"`
}

package entities

import "time"

// CredentialPriceDefault is independent of legacy model prices and rules.
// Absence means inheritance; zero and one are both explicit active overrides.
type CredentialPriceDefault struct {
	SubjectID  string    `gorm:"primaryKey"`
	Multiplier float64   `gorm:"not null"`
	UpdatedAt  time.Time `gorm:"serializer:storageTime"`
}

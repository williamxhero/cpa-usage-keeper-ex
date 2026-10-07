package entities

import "time"

// CredentialPricingSubject is Keeper-owned. Identity is the exact observed upstream
// relation, not a secret lookup key or a promise that the upstream index never rotates.
type CredentialPricingSubject struct {
	ID              string `gorm:"primaryKey"`
	UsageIdentityID int64
	AuthType        UsageIdentityAuthType `gorm:"uniqueIndex:uniq_credential_pricing_identity"`
	AuthTypeName    string
	Identity        string    `gorm:"uniqueIndex:uniq_credential_pricing_identity"`
	CreatedAt       time.Time `gorm:"serializer:storageTime"`
	// Read-model fields describe the effective association, never persisted over
	// the original relation. Disabled origins remain private sanitization evidence.
	BindingRef      string `gorm:"-"`
	BindingDisabled bool   `gorm:"-"`
}

package entities

import "time"

// CredentialPricingAssociation is an additive exact relation. Explicit correction
// can disable or reassign it; the subject's original stored relation is untouched.
type CredentialPricingAssociation struct {
	ID              string `gorm:"primaryKey"`
	SubjectID       string `gorm:"index;not null"`
	UsageIdentityID int64
	AuthType        UsageIdentityAuthType `gorm:"uniqueIndex:uniq_credential_pricing_association"`
	AuthTypeName    string
	Identity        string `gorm:"uniqueIndex:uniq_credential_pricing_association"`
	Enabled         bool
	CreatedAt       time.Time `gorm:"serializer:storageTime"`
}

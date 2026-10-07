package dto

// PricingCredential is an explicit public allowlist. DirectoryID is only a
// selection reference; SubjectID is a Keeper ID, neither is a permanent upstream ID.
type PricingCredential struct {
	DirectoryID   int64  `json:"directory_id"`
	SubjectID     string `json:"subject_id,omitempty"`
	Name          string `json:"name"`
	Alias         string `json:"alias,omitempty"`
	ProviderType  string `json:"provider_type"`
	AuthType      string `json:"auth_type"`
	Endpoint      string `json:"endpoint,omitempty"`
	Status        string `json:"status"`
	BindingStatus string `json:"binding_status"`
}

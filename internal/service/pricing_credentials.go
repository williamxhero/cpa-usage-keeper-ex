package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ErrCredentialBindingConflict = errors.New("credential binding conflicts with an existing or ambiguous identity")
	ErrCredentialNotSelectable   = errors.New("credential is not uniquely selectable")
)

// Separate from the legacy pricing interface so existing callers retain their contract.
type PricingCredentialProvider interface {
	ListPricingCredentials(context.Context) ([]servicedto.PricingCredential, error)
	ListCredentialPricingSubjects(context.Context) ([]servicedto.PricingCredential, error)
	BindPricingCredential(context.Context, int64) (servicedto.PricingCredential, error)
	ResolvePricingCredential(context.Context, string, string) (servicedto.PricingCredential, error)
}

func (s *pricingService) ListPricingCredentials(ctx context.Context) ([]servicedto.PricingCredential, error) {
	result := []servicedto.PricingCredential{}
	err := repository.ReadCredentialPricingDirectory(ctx, s.db, func(identities []entities.UsageIdentity, subjects []entities.CredentialPricingSubject) error {
		for _, identity := range identities {
			result = append(result, pricingCredential(identity, subjects))
		}
		return nil
	})
	return result, err
}

// ResolvePricingCredential accepts observed upstream fields only, never a downstream
// API group, display name, endpoint, LookupKey or directory position. It is not a price engine.
func (s *pricingService) ResolvePricingCredential(ctx context.Context, authTypeName, authIndex string) (servicedto.PricingCredential, error) {
	result := servicedto.PricingCredential{Name: "Credential", AuthType: "unknown", ProviderType: "unknown", Status: "unknown", BindingStatus: "unknown"}
	if authIndex == "" || (authTypeName != "" && authTypeName != "oauth" && authTypeName != "apikey") {
		return result, nil
	}
	err := repository.ReadCredentialPricingDirectory(ctx, s.db, func(identities []entities.UsageIdentity, subjects []entities.CredentialPricingSubject) error {
		matches := 0
		for _, identity := range identities {
			if identity.Identity != authIndex || (authTypeName != "" && identity.AuthTypeName != authTypeName) {
				continue
			}
			matches++
			result = pricingCredential(identity, subjects)
		}
		if matches > 1 {
			result = servicedto.PricingCredential{Name: "Credential", AuthType: "unknown", ProviderType: "unknown", Status: "unknown", BindingStatus: "ambiguous"}
		}
		return nil
	})
	return result, err
}

func (s *pricingService) ListCredentialPricingSubjects(ctx context.Context) ([]servicedto.PricingCredential, error) {
	result := []servicedto.PricingCredential{}
	err := repository.ReadCredentialPricingDirectory(ctx, s.db, func(identities []entities.UsageIdentity, subjects []entities.CredentialPricingSubject) error {
		for _, subject := range subjects {
			item := servicedto.PricingCredential{SubjectID: subject.ID, Name: "Credential", AuthType: "unknown", ProviderType: "unknown", Status: "stale", BindingStatus: "unknown"}
			matches := 0
			for _, identity := range identities {
				if identity.AuthType == subject.AuthType && identity.Identity == subject.Identity {
					matches++
					item = pricingCredential(identity, subjects)
				}
			}
			if matches > 1 {
				item = servicedto.PricingCredential{Name: "Credential", AuthType: "unknown", ProviderType: "unknown", Status: "unknown", BindingStatus: "ambiguous"}
			}
			// This is the saved subject, not a claim that an ambiguous request resolves to it.
			item.SubjectID = subject.ID
			result = append(result, item)
		}
		return nil
	})
	return result, err
}

func (s *pricingService) BindPricingCredential(ctx context.Context, directoryID int64) (servicedto.PricingCredential, error) {
	var result servicedto.PricingCredential
	_, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		tx = tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		identities, subjects, err := repository.LoadCredentialPricingDirectory(tx)
		if err != nil {
			return err
		}
		for _, identity := range identities {
			if identity.ID != directoryID {
				continue
			}
			result = pricingCredential(identity, subjects)
			if result.BindingStatus == "bound" {
				return ErrCredentialBindingConflict
			}
			if result.BindingStatus != "unbound" {
				return ErrCredentialNotSelectable
			}
			idBytes := make([]byte, 16)
			if _, err := rand.Read(idBytes); err != nil {
				return err
			}
			subject := &entities.CredentialPricingSubject{ID: "cred_" + hex.EncodeToString(idBytes), UsageIdentityID: identity.ID, AuthType: identity.AuthType, AuthTypeName: identity.AuthTypeName, Identity: identity.Identity}
			result.SubjectID = subject.ID
			result.BindingStatus = "bound"
			return tx.Create(subject).Error
		}
		return ErrCredentialNotSelectable
	})
	if err != nil {
		return servicedto.PricingCredential{}, err
	}
	return result, nil
}

func safeCredentialProviderType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai", "codex", "gemini", "gemini-cli", "gemini-interactions", "claude", "vertex", "meta", "xai", "antigravity", "kimi", "kimi-ai", "kimi.ai", "kimi.com":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func safeCredentialText(value string, identity entities.UsageIdentity) string {
	return pricing.SafeCredentialText(value, identity)
}

func pricingCredential(identity entities.UsageIdentity, subjects []entities.CredentialPricingSubject) servicedto.PricingCredential {
	authType := "unknown"
	switch identity.AuthType {
	case entities.UsageIdentityAuthTypeAuthFile:
		authType = "oauth"
	case entities.UsageIdentityAuthTypeAIProvider:
		authType = "apikey"
	}
	providerType := safeCredentialProviderType(identity.Type)
	result := servicedto.PricingCredential{DirectoryID: identity.ID, Name: safeCredentialText(identity.Name, identity), ProviderType: providerType, AuthType: authType, Status: "active", BindingStatus: "unbound"}
	if result.Name == "" {
		result.Name = "Credential"
	}
	if identity.Alias != nil {
		result.Alias = safeCredentialText(*identity.Alias, identity)
	}
	if endpoint, err := url.Parse(identity.BaseURL); err == nil && endpoint.Host != "" && (endpoint.Scheme == "https" || endpoint.Scheme == "http") {
		// Only scheme and authority are allowlisted. Paths, query and fragment may all contain secrets.
		if safeCredentialText(endpoint.Host, identity) != "" {
			result.Endpoint = endpoint.Scheme + "://" + endpoint.Host
		}
	}
	if identity.Disabled != nil && *identity.Disabled {
		result.Status = "disabled"
	}
	if identity.IsDeleted {
		result.Status = "stale"
	}
	if strings.TrimSpace(identity.Identity) == "" || authType == "unknown" || identity.AuthTypeName != authType {
		result.BindingStatus = "unknown"
		return result
	}
	matches := 0
	for _, subject := range subjects {
		if subject.AuthType == identity.AuthType && subject.Identity == identity.Identity {
			matches++
			result.SubjectID = subject.ID
			result.BindingStatus = "bound"
			if subject.AuthTypeName != authType {
				result.BindingStatus = "ambiguous"
			}
		}
	}
	if matches > 1 || result.BindingStatus == "ambiguous" {
		result.BindingStatus = "ambiguous"
		result.SubjectID = ""
		return result
	}
	// Historical visibility cannot turn known ambiguity into attribution evidence.
	if identity.BindingIdentityStatus == "ambiguous" {
		result.BindingStatus = "ambiguous"
		result.SubjectID = ""
	} else if identity.BindingIdentityStatus != "unique" {
		result.BindingStatus = "unknown"
		result.SubjectID = ""
	} else if identity.IsDeleted {
		result.BindingStatus = "stale"
	}
	return result
}

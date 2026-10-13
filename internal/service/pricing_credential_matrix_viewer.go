package service

import (
	"context"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type credentialMatrixPair struct{ subjectID, model string }

type ViewerCredentialModelMatrixProvider interface {
	ListViewerCredentialModelMatrix(context.Context, int64, CredentialMatrixQuery) (CredentialMatrixPage, error)
}

// Only exact credential/model pairs observed in this active downstream key's
// retained usage are visible. Names, providers and endpoint similarity grant no access.
func (s *pricingService) ListViewerCredentialModelMatrix(ctx context.Context, keyID int64, query CredentialMatrixQuery) (CredentialMatrixPage, error) {
	query.allowedPairs = make(map[credentialMatrixPair]bool)
	err := s.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Transaction(func(tx *gorm.DB) error {
		key, err := repository.FindActiveCPAAPIKeyByID(tx, keyID)
		if err != nil {
			return err
		}
		if key.APIKey == "" {
			return ErrInvalidPricingInput
		}
		identities, subjects, err := repository.LoadCredentialPricingDirectory(tx)
		if err != nil {
			return err
		}
		secrets := credentialMatrixSecrets(identities, subjects)
		query.viewerSecrets = []string{key.APIKey}
		secrets = append(secrets, key.APIKey)
		var observed []struct{ AuthType, AuthIndex, Model, ModelAlias string }
		if err := tx.Raw("SELECT DISTINCT auth_type, auth_index, model, COALESCE(model_alias, '') AS model_alias FROM usage_events WHERE api_group_key = ? UNION SELECT DISTINCT auth_type, auth_index, model, COALESCE(model_alias, '') AS model_alias FROM usage_events_archive WHERE api_group_key = ?", key.APIKey, key.APIKey).Scan(&observed).Error; err != nil {
			return err
		}
		for _, event := range observed {
			var subjectID string
			matches := 0
			for _, identity := range identities {
				authType := ""
				switch identity.AuthType {
				case entities.UsageIdentityAuthTypeAuthFile:
					authType = "oauth"
				case entities.UsageIdentityAuthTypeAIProvider:
					authType = "apikey"
				}
				if authType == "" || event.AuthType != authType || event.AuthIndex != identity.Identity || identity.AuthTypeName != authType || identity.BindingIdentityStatus != "unique" || identity.IsDeleted || (identity.Disabled != nil && *identity.Disabled) {
					continue
				}
				for _, subject := range subjects {
					if !subject.BindingDisabled && subject.AuthType == identity.AuthType && subject.AuthTypeName == authType && subject.Identity == identity.Identity {
						subjectID = subject.ID
						matches++
					}
				}
			}
			if matches != 1 {
				continue
			}
			for _, model := range []string{event.Model, event.ModelAlias} {
				canonical := pricing.CanonicalCredentialModelID(model)
				if safeCredentialMatrixModel(canonical, secrets) {
					query.allowedPairs[credentialMatrixPair{subjectID, canonical}] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		return CredentialMatrixPage{}, err
	}
	return s.ListCredentialModelMatrix(ctx, query)
}

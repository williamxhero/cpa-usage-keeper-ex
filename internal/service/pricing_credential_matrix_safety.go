package service

import (
	"net/url"
	"path"
	"strings"
	"unicode"

	"cpa-usage-keeper/internal/entities"
)

// Matrix labels and model IDs are checked against the whole directory, including
// stale identity evidence. Metadata changes must not expose another account's key.
func credentialMatrixSecrets(identities []entities.UsageIdentity, subjects []entities.CredentialPricingSubject) []string {
	secrets := make([]string, 0, len(identities)*4+len(subjects))
	for _, identity := range identities {
		secrets = append(secrets, identity.Identity, identity.LookupKey)
		for _, file := range []*string{identity.FileName, identity.FilePath} {
			if file != nil && *file != "" {
				basename := path.Base(strings.ReplaceAll(*file, `\`, "/"))
				secrets = append(secrets, *file, basename, strings.TrimSuffix(basename, path.Ext(basename)))
			}
		}
		if endpoint, err := url.Parse(identity.BaseURL); err == nil {
			secrets = append(secrets, endpoint.Path, endpoint.RawPath, endpoint.Fragment)
			if endpoint.User != nil {
				secrets = append(secrets, endpoint.User.Username())
				if password, ok := endpoint.User.Password(); ok {
					secrets = append(secrets, password)
				}
			}
			for _, values := range endpoint.Query() {
				secrets = append(secrets, values...)
			}
		}
	}
	for _, subject := range subjects {
		secrets = append(secrets, subject.Identity)
	}
	return secrets
}

func safeCredentialMatrixText(value string, secrets []string) bool {
	lower := strings.ToLower(value)
	for _, secret := range secrets {
		if secret != "" && strings.Contains(lower, strings.ToLower(secret)) {
			return false
		}
	}
	return true
}

func safeCredentialMatrixModel(model string, secrets []string) bool {
	if model == "" || len(model) > 256 || !safeCredentialMatrixText(model, secrets) || strings.Contains(model, ":/") || strings.Contains(model, "..") {
		return false
	}
	first := true
	for _, char := range model {
		if first && !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			return false
		}
		first = false
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) && !strings.ContainsRune("-_.:/@+", char) {
			return false
		}
	}
	lower := strings.ToLower(model)
	for _, suffix := range []string{".json", ".yaml", ".yml", ".pem", ".key"} {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}
	for _, segment := range strings.FieldsFunc(lower, func(r rune) bool { return r == '/' || r == ':' || r == '@' }) {
		for _, prefix := range []string{"sk-", "ghp_", "gho_", "github_pat_", "glpat-", "xoxb-", "xoxp-", "eyj"} {
			if strings.HasPrefix(segment, prefix) {
				return false
			}
		}
	}
	return true
}

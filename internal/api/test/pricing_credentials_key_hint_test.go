package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/entities"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

func TestPricingCredentialAPIsExposeOnlyMaskedKeyHints(t *testing.T) {
	for _, tt := range []struct {
		name     string
		key      string
		hint     string
		authType entities.UsageIdentityAuthType
		authName string
	}{
		{name: "normal", key: "qxbd-synthetic-key-xdgy", hint: "qxbd....xdgy", authType: entities.UsageIdentityAuthTypeAIProvider, authName: "apikey"},
		{name: "short", key: "abcdefgh", hint: "abc....", authType: entities.UsageIdentityAuthTypeAIProvider, authName: "apikey"},
		{name: "too short", key: "abcdefg", authType: entities.UsageIdentityAuthTypeAIProvider, authName: "apikey"},
		{name: "empty", authType: entities.UsageIdentityAuthTypeAIProvider, authName: "apikey"},
		{name: "oauth", key: "qxbd-synthetic-key-xdgy", authType: entities.UsageIdentityAuthTypeAuthFile, authName: "oauth"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := openAPITestDatabase(t)
			row := entities.UsageIdentity{
				Name: "Synthetic account", Type: "openai", LookupKey: tt.key,
				AuthType: tt.authType, AuthTypeName: tt.authName,
				Identity: "synthetic-auth-index", BindingIdentityStatus: "unique",
			}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal("create synthetic credential failed")
			}
			router, cookie := credentialPricingRouter(t, db)
			check := func(body []byte, single bool) {
				t.Helper()
				if tt.key != "" && strings.Contains(string(body), tt.key) {
					t.Fatal("pricing API exposed the complete lookup key")
				}
				var item servicedto.PricingCredential
				if single {
					if err := json.Unmarshal(body, &item); err != nil {
						t.Fatal("decode credential failed")
					}
				} else {
					var result struct {
						Credentials []servicedto.PricingCredential `json:"credentials"`
					}
					if err := json.Unmarshal(body, &result); err != nil || len(result.Credentials) != 1 {
						t.Fatal("expected one credential")
					}
					item = result.Credentials[0]
				}
				if item.KeyHint != tt.hint || item.AuthType != tt.authName {
					t.Fatal("unexpected credential hint or auth type")
				}
				if strings.Contains(string(body), `"key_hint"`) != (tt.hint != "") {
					t.Fatal("empty key_hint must be omitted from API responses")
				}
				if strings.Contains(string(body), `"lookup_key"`) {
					t.Fatal("raw lookup_key field must not be returned")
				}
			}
			directory := serveAPIGet(router, "/api/v1/pricing/credentials", cookie)
			if directory.Code != http.StatusOK {
				t.Fatal("directory request failed")
			}
			check(directory.Body.Bytes(), false)
			bound := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, row.ID), cookie)
			if bound.Code != http.StatusCreated {
				t.Fatal("binding request failed")
			}
			check(bound.Body.Bytes(), true)
			saved := serveAPIGet(router, "/api/v1/pricing/credential-subjects", cookie)
			if saved.Code != http.StatusOK {
				t.Fatal("saved subjects request failed")
			}
			check(saved.Body.Bytes(), false)
		})
	}
}

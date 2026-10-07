package service

import (
	"encoding/json"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/entities"
)

func TestPricingCredentialKeyHint(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
		hint string
	}{
		{name: "normal", key: "qxbd-synthetic-key-xdgy", hint: "qxbd....xdgy"},
		{name: "sixteen characters", key: "abcdefghijklmnop", hint: "abcd....mnop"},
		{name: "fifteen characters", key: "abcdefghijklmno", hint: "abc...."},
		{name: "eight characters", key: "abcdefgh", hint: "abc...."},
		{name: "seven characters", key: "abcdefg"},
		{name: "empty"},
		{name: "unicode short", key: "甲乙丙丁戊己庚"},
		{name: "unicode eight characters", key: "甲乙丙丁戊己庚辛", hint: "甲乙丙...."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			identity := entities.UsageIdentity{
				Name: "Synthetic credential", Type: "openai",
				AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey",
				Identity: "synthetic-auth-index", BindingIdentityStatus: "unique", LookupKey: tt.key,
			}
			result := pricingCredential(identity, nil)
			body, err := json.Marshal(result)
			if err != nil {
				t.Fatal("marshal pricing credential failed")
			}
			if tt.key != "" && (strings.Contains(string(body), tt.key) || strings.Contains(result.KeyHint, tt.key)) {
				t.Fatal("pricing credential exposed the complete lookup key")
			}
			if result.KeyHint != tt.hint {
				t.Fatal("unexpected key hint")
			}
			if strings.Contains(string(body), `"key_hint"`) != (tt.hint != "") {
				t.Fatal("key_hint must be omitted when empty")
			}
		})
	}
}

func TestPricingCredentialKeyHintOnlyForAPIKeys(t *testing.T) {
	for _, tt := range []struct {
		name     string
		authType entities.UsageIdentityAuthType
		authName string
	}{
		{name: "oauth", authType: entities.UsageIdentityAuthTypeAuthFile, authName: "oauth"},
		{name: "unknown", authType: -1, authName: "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			identity := entities.UsageIdentity{
				Name: "Synthetic account", Type: "openai", AuthType: tt.authType, AuthTypeName: tt.authName,
				LookupKey: "qxbd-synthetic-key-xdgy",
			}
			result := pricingCredential(identity, nil)
			body, err := json.Marshal(result)
			if err != nil {
				t.Fatal("marshal pricing credential failed")
			}
			if result.KeyHint != "" || strings.Contains(string(body), `"key_hint"`) {
				t.Fatal("non-apikey credential must not expose a key hint")
			}
			if strings.Contains(string(body), identity.LookupKey) {
				t.Fatal("pricing credential exposed the complete lookup key")
			}
		})
	}
}

func TestPricingCredentialKeyHintSample(t *testing.T) {
	identity := entities.UsageIdentity{
		AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey",
		LookupKey: "qxbd-synthetic-key-xdgy",
	}
	hint := pricingCredential(identity, nil).KeyHint
	if hint != "qxbd....xdgy" || hint == identity.LookupKey {
		t.Fatal("sample masking failed")
	}
	t.Logf("key_hint=%s; equals original=false", hint)
}

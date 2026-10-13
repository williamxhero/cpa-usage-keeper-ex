package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
)

func TestCredentialMatrixAdminHTTPAtomicSparsePersistenceAndRestart(t *testing.T) {
	db := openAPITestDatabase(t)
	identity := entities.UsageIdentity{Name: "Matrix account", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "matrix-synthetic-index", LookupKey: "matrix-secret-key", Type: "openai", BindingIdentityStatus: "unique"}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatal(err)
	}
	subject := entities.CredentialPricingSubject{ID: "cred_matrix_synthetic", UsageIdentityID: identity.ID, AuthType: identity.AuthType, AuthTypeName: identity.AuthTypeName, Identity: identity.Identity}
	if err := db.Create(&subject).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.LoadPricingSnapshot(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	catalog := pricing.NewCatalog(snapshot)
	router, cookie := credentialPricingRouter(t, db, catalog)
	path := "/api/v1/pricing/credential-model-matrix"
	body := `{"cells":[{"subject_id":"cred_matrix_synthetic","canonical_model_id":" OpenAI/GPT-4.1 ","mode":"fixed","fixed":{"pricing_style":"openai","prompt_price_per_1m":0,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4}}]}`
	saved := serveCredentialMutation(router, http.MethodPut, path, body, cookie)
	if saved.Code != http.StatusOK || !strings.Contains(saved.Body.String(), `"covered":true`) || !strings.Contains(saved.Body.String(), `"prompt_price_per_1m":0`) {
		t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
	}
	for _, secret := range []string{identity.Identity, identity.LookupKey, "lookup_key"} {
		if strings.Contains(saved.Body.String(), secret) {
			t.Fatalf("unsafe response leaked %q", secret)
		}
	}
	before := catalog.Snapshot()
	for _, invalid := range []string{
		`{"cells":[{"subject_id":"cred_matrix_synthetic","model":"m","fixed":{"prompt_price_per_1m":1,"completion_price_per_1m":2,"cache_read_price_per_1m":3}}]}`,
		`{"cells":[{"subject_id":"cred_matrix_synthetic","model":"m","mode":"multiplier","fixed":{"prompt_price_per_1m":1,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4}}]}`,
		`{"cells":[{"subject_id":"cred_matrix_synthetic","model":"m","fixed":{"prompt_price_per_1m":1,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4},"unknown":"secret"}]}`,
		`{"cells":[{"subject_id":"cred_matrix_synthetic","model":"m","clear":true,"mode":"fixed"}]}`,
		`{"cells":[{"subject_id":"cred_matrix_synthetic","model":"m","clear":true,"fixed":{"prompt_price_per_1m":1}}]}`,
		`{"cells":[{"subject_id":"cred_matrix_synthetic","model":"m","prompt_price_per_1m":0,"fixed":{"prompt_price_per_1m":1,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4}}]}`,
		`{"cells":[],"updates":[{"subject_id":"cred_matrix_synthetic","model":"m","clear":true}]}`,
		`{"cells":[{"subject_id":"cred_matrix_synthetic","model":"matrix-secret-key","clear":true}]}`,
		`{"cells":[{"subject_id":"cred_matrix_synthetic","model":"C:/auth/config.json","clear":true}]}`,
		`{"cells":[{"subject_id":"cred_matrix_synthetic","model":"m","fixed":{"prompt_price_per_1m":1,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4,"unknown":1}}]}`,
		`{"cells":[{"subject_id":"cred_matrix_synthetic","model":"m","clear":true},{"subject_id":"cred_matrix_synthetic","model":" M ","clear":true}]}`,
		`{"cells":[]} {"cells":[]}`,
	} {
		response := serveCredentialMutation(router, http.MethodPut, path, invalid, cookie)
		if response.Code != http.StatusBadRequest || catalog.Snapshot() != before {
			t.Fatalf("invalid batch changed state: %d %s", response.Code, response.Body.String())
		}
	}
	read := serveAPIGet(router, path+"?model=gpt", cookie)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"canonical_model_id":"openai/gpt-4.1"`) || !strings.Contains(read.Body.String(), `"snapshot_id":"`+catalog.Snapshot().ID()+`"`) {
		t.Fatalf("sparse query: %d %s", read.Code, read.Body.String())
	}
	restarted, err := repository.LoadPricingSnapshot(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	router, cookie = credentialPricingRouter(t, db, pricing.NewCatalog(restarted))
	read = serveAPIGet(router, path, cookie)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"prompt_price_per_1m":0`) {
		t.Fatalf("restart: %d %s", read.Code, read.Body.String())
	}
	cleared := serveCredentialMutation(router, http.MethodPut, path, `{"cells":[{"subject_id":"cred_matrix_synthetic","model":"openai/gpt-4.1","clear":true}]}`, cookie)
	if cleared.Code != http.StatusOK || !strings.Contains(cleared.Body.String(), `"covered":false`) {
		t.Fatalf("clear: %d %s", cleared.Code, cleared.Body.String())
	}
	var count int64
	if err := db.Model(&entities.CredentialModelPriceOverride{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("clear row count=%d err=%v", count, err)
	}
	tooMany := make([]map[string]any, service.CredentialMatrixMaxCells+1)
	for index := range tooMany {
		tooMany[index] = map[string]any{"subject_id": "cred_matrix_synthetic", "model": fmt.Sprintf("m-%d", index), "clear": true}
	}
	encoded, _ := json.Marshal(map[string]any{"cells": tooMany})
	response := serveCredentialMutation(router, http.MethodPut, path, string(encoded), cookie)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("batch limit: %d %s", response.Code, response.Body.String())
	}
}

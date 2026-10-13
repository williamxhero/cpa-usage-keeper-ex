package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	keeperapi "cpa-usage-keeper/internal/api"
	"cpa-usage-keeper/internal/auth"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
)

func TestCredentialMatrixHTTP5000FixedCellsPaginationAndBodyLimit(t *testing.T) {
	db := openAPITestDatabase(t)
	identity := entities.UsageIdentity{Name: "Batch account", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "batch-account-index", BindingIdentityStatus: "unique"}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatal(err)
	}
	subject := entities.CredentialPricingSubject{ID: "cred_batch", UsageIdentityID: identity.ID, AuthType: identity.AuthType, AuthTypeName: identity.AuthTypeName, Identity: identity.Identity}
	if err := db.Create(&subject).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.LoadPricingSnapshot(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	catalog := pricing.NewCatalog(snapshot)
	router, cookie := credentialPricingRouter(t, db, catalog)
	cells := make([]map[string]any, service.CredentialMatrixMaxCells)
	for index := range cells {
		cells[index] = map[string]any{"subject_id": subject.ID, "model": fmt.Sprintf("synthetic-provider/model-%04d", index), "mode": "fixed", "fixed": map[string]any{"pricing_style": "openai", "prompt_price_per_1m": 0, "completion_price_per_1m": 0, "cache_read_price_per_1m": 0, "cache_write_price_per_1m": 0}}
	}
	encoded, err := json.Marshal(map[string]any{"cells": cells})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) <= 1<<20 {
		t.Fatal("fixture must exercise a fixed batch larger than one MiB")
	}
	path := "/api/v1/pricing/credential-model-matrix"
	response := serveCredentialMutation(router, http.MethodPut, path, string(encoded), cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("5000 fixed cells: %d %s", response.Code, response.Body.String())
	}
	var saved struct {
		Cells []service.CredentialMatrixCell `json:"cells"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil || len(saved.Cells) != 5000 {
		t.Fatalf("saved cells=%d err=%v", len(saved.Cells), err)
	}
	for _, query := range []string{"", "?model_page=2", "?model=model-004&model_page_size=5", "?model_page_size=5000&credential_page_size=5000", "?model_page=9223372036854775807&credential_page=9223372036854775807"} {
		response = serveAPIGet(router, path+query, cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("page %s: %d %s", query, response.Code, response.Body.String())
		}
		var page service.CredentialMatrixPage
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Cells) > 50 || len(page.Models) > 50 || page.CredentialPageSize > 50 || page.ModelPageSize > 50 {
			t.Fatalf("unbounded page: %+v", page)
		}
		if query == "" && (len(page.Credentials) != 1 || len(page.Cells) != 50 || page.TotalModels != 5000) {
			t.Fatalf("default page totals=%d credentials=%d cells=%d", page.TotalModels, len(page.Credentials), len(page.Cells))
		}
		if strings.Contains(query, "922337") && (len(page.Cells) != 0 || len(page.Credentials) != 0 || len(page.Models) != 0) {
			t.Fatal("out-of-range page was not empty")
		}
	}
	before := catalog.Snapshot()
	response = serveCredentialMutation(router, http.MethodPut, path, `{"cells":[]}`+strings.Repeat(" ", 4<<20), cookie)
	if response.Code != http.StatusRequestEntityTooLarge || catalog.Snapshot() != before {
		t.Fatalf("body limit: %d", response.Code)
	}
}

func TestCredentialMatrixViewerSeesOnlyExactAuthorizedPairs(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	key := entities.CPAAPIKey{ID: 42, APIKey: "synthetic-downstream-viewer-key", DisplayKey: "viewer"}
	if err := db.Create(&key).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c"} {
		identity := entities.UsageIdentity{Name: "Same safe account", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "matrix-viewer-index-" + id, LookupKey: "synthetic-upstream-key-" + id, Type: "openai", BindingIdentityStatus: "unique"}
		if err := db.Create(&identity).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&entities.CredentialPricingSubject{ID: "cred_viewer_" + id, UsageIdentityID: identity.ID, AuthType: identity.AuthType, AuthTypeName: identity.AuthTypeName, Identity: identity.Identity}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, event := range []entities.UsageEvent{
		{APIGroupKey: key.APIKey, AuthType: "apikey", AuthIndex: "matrix-viewer-index-a", Model: "m1", Timestamp: time.Now()},
		{APIGroupKey: key.APIKey, AuthType: "apikey", AuthIndex: "matrix-viewer-index-b", Model: "m2", Timestamp: time.Now()},
		{APIGroupKey: "another-downstream-key", AuthType: "apikey", AuthIndex: "matrix-viewer-index-c", Model: "m3", Timestamp: time.Now()},
	} {
		if err := db.Create(&event).Error; err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := repository.LoadPricingSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	catalog := pricing.NewCatalog(snapshot)
	prices := service.NewPricingService(db, catalog)
	matrix := prices.(service.CredentialModelMatrixProvider)
	rate := 1.0
	inputs := []service.CredentialMatrixCellInput{}
	for _, id := range []string{"a", "b", "c"} {
		for _, model := range []string{"m1", "m2", "m3"} {
			inputs = append(inputs, service.CredentialMatrixCellInput{SubjectID: "cred_viewer_" + id, CanonicalModelID: model, PromptPricePer1M: &rate, CompletionPricePer1M: &rate, CacheReadPricePer1M: &rate, CacheWritePricePer1M: &rate})
		}
	}
	if _, err := matrix.UpdateCredentialModelMatrix(ctx, inputs); err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessionManager(time.Hour)
	token, _, err := sessions.CreateAPIKeyViewerWithSource(key.ID, auth.SessionSourceStandard)
	if err != nil {
		t.Fatal(err)
	}
	cfg := keeperapi.AuthConfig{Enabled: true, LoginPassword: "synthetic-password", SessionTTL: time.Hour}
	router := keeperapi.NewRouter(nil, nil, nil, prices, cfg, keeperapi.NewAuthHandler(cfg, sessions), "", keeperapi.OptionalProviders{CPAAPIKeys: service.NewCPAAPIKeyService(db)})
	cookie := &http.Cookie{Name: standardSessionCookieName, Value: token}
	viewerPath := "/api/v1/key/pricing/credential-model-matrix"
	response := serveAPIGet(router, viewerPath+"?api_key_id=999&subject_id=cred_viewer_c", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("viewer read: %d %s", response.Code, response.Body.String())
	}
	var page service.CredentialMatrixPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Cells) != 2 || page.TotalCredentials != 2 || page.TotalModels != 2 {
		t.Fatalf("scope totals: cells=%d credentials=%d models=%d", len(page.Cells), page.TotalCredentials, page.TotalModels)
	}
	for _, cell := range page.Cells {
		if !((cell.SubjectID == "cred_viewer_a" && cell.CanonicalModelID == "m1") || (cell.SubjectID == "cred_viewer_b" && cell.CanonicalModelID == "m2")) {
			t.Fatalf("unauthorized cross-pair: %+v", cell)
		}
		if cell.SnapshotID != page.SnapshotID {
			t.Fatal("mixed snapshot IDs")
		}
	}
	for _, secret := range []string{key.APIKey, "synthetic-upstream-key", "matrix-viewer-index", "cred_viewer_c", "key_hint", "endpoint"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("viewer leaked %q", secret)
		}
	}
	response = serveAPIGet(router, viewerPath+"?model=m1&credential_page_size=1", cookie)
	if response.Code != http.StatusOK {
		t.Fatal(response.Code)
	}
	if anonymous := serveAPIGet(router, viewerPath); anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous viewer read=%d", anonymous.Code)
	}
	adminPath := "/api/v1/pricing/credential-model-matrix"
	for _, test := range []struct {
		cookie *http.Cookie
		want   int
	}{{nil, http.StatusUnauthorized}, {cookie, http.StatusForbidden}} {
		var cookies []*http.Cookie
		if test.cookie != nil {
			cookies = append(cookies, test.cookie)
		}
		response := serveCredentialMutation(router, http.MethodPut, adminPath, `{"cells":[]}`, cookies...)
		if response.Code != test.want {
			t.Fatalf("unauthorized write=%d want=%d", response.Code, test.want)
		}
	}
	if err := db.Model(&key).Update("is_deleted", true).Error; err != nil {
		t.Fatal(err)
	}
	if revoked := serveAPIGet(router, viewerPath, cookie); revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked read=%d", revoked.Code)
	}
}

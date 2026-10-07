package test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	keeperapi "cpa-usage-keeper/internal/api"
	"cpa-usage-keeper/internal/auth"
	"cpa-usage-keeper/internal/cpa/dto/providerconfig"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/service"
)

func TestPricingCredentialDirectoryRejectsSharedUpstreamConfiguration(t *testing.T) {
	db := openMetadataTestDatabase(t, "shared-identity.db")
	fetcher := newMetadataTestFetcher()
	config := providerconfig.OpenAICompatibilityConfig{Name: "First configuration", BaseURL: "https://synthetic.example/v1", APIKeyEntries: []providerconfig.OpenAIApiKeyEntry{{APIKey: "synthetic-secret", AuthIndex: "shared-index"}}}
	second := config
	second.Name = "Second configuration"
	fetcher.openAIResult = &response.OpenAICompatibilityResult{StatusCode: 200, Payload: []providerconfig.OpenAICompatibilityConfig{config, second}}
	if err := newMetadataTestSyncer(db, fetcher, time.Now).SyncMetadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider := service.NewPricingService(db, pricing.NewCatalog(pricing.EmptySnapshot()))
	directory, err := provider.(service.PricingCredentialProvider).ListPricingCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(directory) != 1 || directory[0].BindingStatus != "ambiguous" || directory[0].SubjectID != "" {
		t.Fatalf("first-item dedup falsely claims a unique configuration: %+v", directory)
	}
	sessions := auth.NewSessionManager(time.Hour)
	token, _, err := sessions.Create()
	if err != nil {
		t.Fatal(err)
	}
	cfg := keeperapi.AuthConfig{Enabled: true, LoginPassword: "synthetic-password", SessionTTL: time.Hour}
	router := keeperapi.NewRouter(nil, nil, nil, provider, cfg, keeperapi.NewAuthHandler(cfg, sessions), "")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pricing/credentials", strings.NewReader(fmt.Sprintf(`{"directory_id":%d}`, directory[0].DirectoryID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CPA-Usage-Keeper-Request", "fetch")
	req.AddCookie(&http.Cookie{Name: "cpa_usage_keeper_session", Value: token})
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("ambiguous save status = %d: %s", res.Code, res.Body.String())
	}
}

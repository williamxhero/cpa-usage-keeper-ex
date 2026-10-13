package test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	keeperapi "cpa-usage-keeper/internal/api"
	"cpa-usage-keeper/internal/auth"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/pricingmetadata"
	"cpa-usage-keeper/internal/service"
)

type officialPricingHarness struct {
	router http.Handler
	admin  *http.Cookie
	viewer *http.Cookie
}

func newOfficialPricingHarness(t *testing.T, metadataClient *pricingmetadata.Client) officialPricingHarness {
	t.Helper()
	db := openAPITestDatabase(t)
	catalog := pricing.NewCatalog(pricing.EmptySnapshot())
	provider := service.NewPricingServiceWithMetadataClient(db, catalog, metadataClient)
	return newOfficialAuthHarness(t, provider)
}

func newOfficialAuthHarness(t *testing.T, provider service.PricingProvider) officialPricingHarness {
	t.Helper()
	sessions := auth.NewSessionManager(time.Hour)
	adminToken, _, err := sessions.Create()
	if err != nil {
		t.Fatalf("create admin session: %v", err)
	}
	viewerToken, _, err := sessions.CreateAPIKeyViewerWithSource(42, auth.SessionSourceStandard)
	if err != nil {
		t.Fatalf("create viewer session: %v", err)
	}
	config := keeperapi.AuthConfig{Enabled: true, LoginPassword: "secret", SessionTTL: time.Hour}
	return officialPricingHarness{
		router: keeperapi.NewRouter(nil, nil, nil, provider, config, keeperapi.NewAuthHandler(config, sessions), ""),
		admin:  &http.Cookie{Name: standardSessionCookieName, Value: adminToken},
		viewer: &http.Cookie{Name: standardSessionCookieName, Value: viewerToken},
	}
}

func officialPricingRequest(method, target, body string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if method != http.MethodGet {
		request.Header.Set(requestIntentHeaderName, requestIntentHeaderValueFetch)
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func serveOfficialPricing(h officialPricingHarness, method, target, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	h.router.ServeHTTP(response, officialPricingRequest(method, target, body, cookie))
	return response
}

func completeOfficialPricingJSON(canonical string) string {
	return `{"canonical_model_id":"` + canonical + `","display_model_id":"Display","source":"official","pricing_style":"openai","prompt_price_per_1m":1,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4}`
}

func TestOfficialPricingRoutesRequireAdminAuthentication(t *testing.T) {
	harness := newOfficialPricingHarness(t, nil)
	anonymous := serveOfficialPricing(harness, http.MethodGet, "/api/v1/pricing/official", "", nil)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want %d: %s", anonymous.Code, http.StatusUnauthorized, anonymous.Body.String())
	}
	viewer := serveOfficialPricing(harness, http.MethodGet, "/api/v1/pricing/official", "", harness.viewer)
	if viewer.Code != http.StatusForbidden {
		t.Fatalf("viewer status = %d, want %d: %s", viewer.Code, http.StatusForbidden, viewer.Body.String())
	}
}

func TestOfficialPricingSinglePutAcceptsCanonicalIDWithSlashAndGetExposesUpdatedAt(t *testing.T) {
	harness := newOfficialPricingHarness(t, nil)
	canonical := "openai/gpt-4o"
	response := serveOfficialPricing(harness, http.MethodPut, "/api/v1/pricing/official", completeOfficialPricingJSON(canonical), harness.admin)
	if response.Code != http.StatusOK {
		t.Fatalf("single PUT status = %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"canonical_model_id":"openai/gpt-4o"`) || !strings.Contains(response.Body.String(), `"updated_at":`) {
		t.Fatalf("single PUT response omitted canonical ID or updated_at: %s", response.Body.String())
	}

	response = serveOfficialPricing(harness, http.MethodGet, "/api/v1/pricing/official", "", harness.admin)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Pricing []map[string]json.RawMessage `json:"pricing"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if len(payload.Pricing) != 1 || string(payload.Pricing[0]["canonical_model_id"]) != `"openai/gpt-4o"` {
		t.Fatalf("unexpected GET payload: %s", response.Body.String())
	}
	if _, ok := payload.Pricing[0]["updated_at"]; !ok {
		t.Fatalf("GET response lacks updated_at: %s", response.Body.String())
	}
	for _, forbidden := range []string{"ID", "CanonicalModelID", "CreatedAt", "UpdatedAt"} {
		if _, ok := payload.Pricing[0][forbidden]; ok {
			t.Fatalf("GET response leaked entity field %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestOfficialPricingBatchRejectsInvalidRequestsAtomically(t *testing.T) {
	harness := newOfficialPricingHarness(t, nil)
	seed := serveOfficialPricing(harness, http.MethodPut, "/api/v1/pricing/official", completeOfficialPricingJSON("seed/model"), harness.admin)
	if seed.Code != http.StatusOK {
		t.Fatalf("seed PUT status = %d: %s", seed.Code, seed.Body.String())
	}
	before := serveOfficialPricing(harness, http.MethodGet, "/api/v1/pricing/official", "", harness.admin)
	if before.Code != http.StatusOK {
		t.Fatalf("baseline GET status = %d: %s", before.Code, before.Body.String())
	}
	baseline := before.Body.String()

	valid := completeOfficialPricingJSON("new/model")
	cases := map[string]string{
		"unknown field": `{"pricing":[` + strings.TrimSuffix(valid, `}`) + `,"unknown":true}]}`,
		"negative":      `{"pricing":[{"canonical_model_id":"new/model","prompt_price_per_1m":-1,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4}]}`,
		"null":          `{"pricing":[{"canonical_model_id":"new/model","prompt_price_per_1m":1,"completion_price_per_1m":2,"cache_read_price_per_1m":null,"cache_write_price_per_1m":4}]}`,
		"missing":       `{"pricing":[{"canonical_model_id":"new/model","prompt_price_per_1m":1,"completion_price_per_1m":2,"cache_read_price_per_1m":3}]}`,
		"NaN":           `{"pricing":[{"canonical_model_id":"new/model","prompt_price_per_1m":NaN,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4}]}`,
		"oversize":      `{"pricing":[` + valid + `],"padding":"` + strings.Repeat("x", 1<<20) + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			response := serveOfficialPricing(harness, http.MethodPut, "/api/v1/pricing/official/batch", body, harness.admin)
			if response.Code < http.StatusBadRequest || response.Code >= 600 {
				t.Fatalf("status = %d, want rejection: %s", response.Code, response.Body.String())
			}
			after := serveOfficialPricing(harness, http.MethodGet, "/api/v1/pricing/official", "", harness.admin)
			if after.Code != http.StatusOK || after.Body.String() != baseline {
				t.Fatalf("invalid batch changed snapshot: status=%d body=%s baseline=%s", after.Code, after.Body.String(), baseline)
			}
		})
	}
}

func TestOfficialPricingRefreshPreviewAndCommitUseSafePreviewSerialization(t *testing.T) {
	body := `{"data":[{"id":"openai/gpt-4o-mini","name":"GPT-4o mini","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`
	metadataClient := pricingmetadata.NewClient(&http.Client{Transport: officialCatalogTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://openrouter.ai/api/v1/models" {
			return nil, &unexpectedOfficialCatalogURL{url: request.URL.String()}
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})})
	db := openAPITestDatabase(t)
	catalog := pricing.NewCatalog(pricing.EmptySnapshot())
	provider := service.NewPricingServiceWithMetadataClient(db, catalog, metadataClient)
	harness := newOfficialAuthHarness(t, provider)
	preview := serveOfficialPricing(harness, http.MethodGet, "/api/v1/pricing/official/refresh/preview?source=openrouter", "", harness.admin)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview status = %d: %s", preview.Code, preview.Body.String())
	}
	if strings.Contains(preview.Body.String(), `"ID"`) || strings.Contains(preview.Body.String(), `"CanonicalModelID"`) {
		t.Fatalf("preview leaked entity fields: %s", preview.Body.String())
	}
	var previewPayload struct {
		PreviewID string `json:"preview_id"`
		Prices    []struct {
			CanonicalModelID     string   `json:"canonical_model_id"`
			CacheReadPricePer1M  *float64 `json:"cache_read_price_per_1m"`
			CacheWritePricePer1M *float64 `json:"cache_write_price_per_1m"`
		} `json:"prices"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &previewPayload); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if previewPayload.PreviewID == "" || len(previewPayload.Prices) != 1 || previewPayload.Prices[0].CanonicalModelID != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected preview payload: %s", preview.Body.String())
	}
	if previewPayload.Prices[0].CacheReadPricePer1M != nil || previewPayload.Prices[0].CacheWritePricePer1M != nil {
		t.Fatalf("missing cache prices were not preserved in preview: %s", preview.Body.String())
	}

	before := serveOfficialPricing(harness, http.MethodGet, "/api/v1/pricing/official", "", harness.admin)
	if before.Code != http.StatusOK || strings.Contains(before.Body.String(), `"canonical_model_id"`) || len(catalog.Snapshot().OfficialPrices()) != 0 {
		t.Fatalf("preview changed persisted or published prices: %d %s", before.Code, before.Body.String())
	}

	commitBody := `{"preview_id":"` + previewPayload.PreviewID + `"}`
	commit := serveOfficialPricing(harness, http.MethodPost, "/api/v1/pricing/official/refresh/commit", commitBody, harness.admin)
	if commit.Code != http.StatusOK {
		t.Fatalf("commit status = %d: %s", commit.Code, commit.Body.String())
	}
	stored := serveOfficialPricing(harness, http.MethodGet, "/api/v1/pricing/official", "", harness.admin)
	if stored.Code != http.StatusOK || !strings.Contains(stored.Body.String(), `"canonical_model_id":"openai/gpt-4o-mini"`) || !strings.Contains(stored.Body.String(), `"cache_read_price_per_1m":null`) {
		t.Fatalf("refresh commit did not persist expected row: %d %s", stored.Code, stored.Body.String())
	}
	published := catalog.Snapshot().OfficialPrices()
	if len(published) != 1 || published[0].Source != entities.OfficialModelPriceSourceOpenRouter || published[0].SourceURL != "https://openrouter.ai/api/v1/models" || published[0].FetchedAt == nil || published[0].ResponseHash == "" || published[0].PromptPricePer1M == nil || *published[0].PromptPricePer1M != 1 || published[0].CompletionPricePer1M == nil || *published[0].CompletionPricePer1M != 2 || published[0].MissingSegments != "cache_read,cache_write" {
		t.Fatalf("commit published incorrect prices or provenance: %+v", published)
	}
}

type officialCatalogTransport func(*http.Request) (*http.Response, error)

func (transport officialCatalogTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type unexpectedOfficialCatalogURL struct{ url string }

func (err *unexpectedOfficialCatalogURL) Error() string { return "unexpected catalog URL: " + err.url }

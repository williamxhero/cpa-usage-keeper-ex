package test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	keeperapi "cpa-usage-keeper/internal/api"
	"cpa-usage-keeper/internal/auth"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

func TestCredentialModelAdminHTTPPersistedSelectionQueriesRestartClear(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	identity := entities.UsageIdentity{Name: "Synthetic model account", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-private-index", LookupKey: "synthetic-private-key", Type: "openai", BindingIdentityStatus: "unique"}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entities.CPAAPIKey{APIKey: "synthetic-downstream", DisplayKey: "Synthetic caller"}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.LoadPricingSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	catalog := pricing.NewCatalog(snapshot)
	prices := service.NewPricingService(db, catalog)
	if _, err := prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "base", PromptPricePer1M: 10, PriceMultiplier: new(.5)}); err != nil {
		t.Fatal(err)
	}
	if _, err := prices.ReplacePricingRules(ctx, servicedto.ReplacePricingRulesInput{Model: "base", Rules: []servicedto.PricingRuleInput{{Key: "service_tier", Value: "priority", Multiplier: new(2.0)}, {Key: "reasoning_effort", Value: "high", Multiplier: new(3.0)}}}); err != nil {
		t.Fatal(err)
	}
	router, cookie := credentialPricingRouter(t, db, catalog)
	credential := bindPricingFixture(t, db, identity.ID, catalog)
	model := "unpriced/request-model"
	path := "/api/v1/pricing/credentials/" + credential.SubjectID + "/model?" + url.Values{"model": {model}}.Encode()
	listPath := "/api/v1/pricing/credentials/" + credential.SubjectID + "/models"
	day := time.Now().Truncate(time.Hour).Add(-6 * time.Hour)
	alias := "base"
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{{EventKey: "http-model", APIGroupKey: "synthetic-downstream", Model: model, ModelAlias: &alias, AuthType: "apikey", AuthIndex: identity.Identity, Timestamp: day.Add(3 * time.Hour), ServiceTier: "priority", ReasoningEffort: "high", InputTokens: 1_000_000, TotalTokens: 1_000_000}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, db, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	used := serveAPIGet(router, "/api/v1/models/used", cookie)
	if used.Code != 200 || !strings.Contains(used.Body.String(), model) || !strings.Contains(used.Body.String(), alias) {
		t.Fatalf("safe absent-baseline models %d %s", used.Code, used.Body.String())
	}
	assertPricingRemoved(t, serveAPIGet(router, path, cookie))
	query := "?" + url.Values{"range": {"custom"}, "unit": {"hour"}, "start": {day.Format(time.RFC3339)}, "end": {day.Add(6 * time.Hour).Format(time.RFC3339)}}.Encode()
	assertQueries := func(cost string, selected bool) {
		t.Helper()
		for _, entry := range []struct{ path, key string }{{"/api/v1/usage/overview", "total_cost"}, {"/api/v1/usage/analysis", "total_cost_usd"}, {"/api/v1/usage/events", "cost_usd"}} {
			res := serveAPIGet(router, entry.path+query, cookie)
			if res.Code != 200 || !strings.Contains(res.Body.String(), `"`+entry.key+`":`+cost) {
				t.Fatalf("query %s %d %s", entry.path, res.Code, res.Body.String())
			}
			if selected && entry.path == "/api/v1/usage/events" {
				for _, fact := range []string{`"scope":"credential_model"`, `"selected_model":"` + model + `"`, `"selected_by":"model"`, `"baseline_model":"base"`, `"baseline_by":"model_alias"`, `"snapshot_id"`} {
					if !strings.Contains(res.Body.String(), fact) {
						t.Fatalf("missing selected evidence %s", res.Body.String())
					}
				}
			}
		}
	}
	assertQueries("30", false)
	for _, value := range []string{"1.2x", "120%", "1.2X", "1.2"} {
		_, err := prices.(service.PricingCredentialModelsProvider).SetCredentialModel(ctx, credential.SubjectID, model, value)
		if err != nil {
			t.Fatal(err)
		}
		assertQueries("12", true)
	}
	assertPricingRemoved(t, serveAPIGet(router, listPath, cookie))
	before := catalog.Snapshot()
	for _, body := range []string{`{}`, `{"multiplier":null}`, `{"multiplier":" "}`, `{"multiplier":"-1"}`, `{"multiplier":"NaN"}`, `{"multiplier":"Infinity"}`, `{"multiplier":"20%x"}`, `{"multiplier":true}`, `{"multiplier":1e999}`, `{"multiplier":1,"key":"synthetic-private-key"}`, `{"multiplier":1} {}`} {
		assertPricingRemoved(t, serveCredentialMutation(router, "PUT", path, body, cookie))
		if catalog.Snapshot() != before {
			t.Fatal("retired write published")
		}
	}
	restarted, err := repository.LoadPricingSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	catalog = pricing.NewCatalog(restarted)
	prices = service.NewPricingService(db, catalog)
	router, cookie = credentialPricingRouter(t, db, catalog)
	assertPricingRemoved(t, serveAPIGet(router, path, cookie))
	assertQueries("12", true)
	applyLegacyPricingFixture(t, prices, "DELETE", path, "")
	assertQueries("30", false)
	assertPricingRemoved(t, serveAPIGet(router, listPath, cookie))
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		assertPricingRemoved(t, serveCredentialMutation(router, method, "/api/v1/pricing/credentials/cred_absent/model?model=M", `{"multiplier":1}`, cookie))
		assertPricingRemoved(t, serveCredentialMutation(router, method, strings.Split(path, "?")[0], `{"multiplier":1}`, cookie))
	}
}

func TestCredentialModelRoutesAdminOnly(t *testing.T) {
	db := openAPITestDatabase(t)
	sessions := auth.NewSessionManager(time.Hour)
	token, _, err := sessions.CreateAPIKeyViewerWithSource(42, auth.SessionSourceStandard)
	if err != nil {
		t.Fatal(err)
	}
	cfg := keeperapi.AuthConfig{Enabled: true, LoginPassword: "synthetic-password", SessionTTL: time.Hour}
	router := keeperapi.NewRouter(nil, nil, nil, service.NewPricingService(db, emptyPricingCatalogForTest()), cfg, keeperapi.NewAuthHandler(cfg, sessions), "")
	viewer := &http.Cookie{Name: standardSessionCookieName, Value: token}
	for _, route := range []struct{ method, path string }{{http.MethodGet, "/api/v1/pricing/credentials/cred_synthetic/models"}, {http.MethodGet, "/api/v1/pricing/credentials/cred_synthetic/model?model=M"}, {http.MethodPut, "/api/v1/pricing/credentials/cred_synthetic/model?model=M"}, {http.MethodDelete, "/api/v1/pricing/credentials/cred_synthetic/model?model=M"}} {
		if res := serveCredentialMutation(router, route.method, route.path, `{"multiplier":1}`); res.Code != 401 {
			t.Fatalf("anonymous %d", res.Code)
		}
		if res := serveCredentialMutation(router, route.method, route.path, `{"multiplier":1}`, viewer); res.Code != 403 {
			t.Fatalf("viewer %d", res.Code)
		}
	}
}

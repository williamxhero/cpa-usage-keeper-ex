package test

import (
	"context"
	"encoding/json"
	"fmt"
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
	bound := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, identity.ID), cookie)
	if bound.Code != 201 {
		t.Fatal(bound.Body.String())
	}
	var credential safeCredential
	if err := json.Unmarshal(bound.Body.Bytes(), &credential); err != nil {
		t.Fatal(err)
	}
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
	inherited := serveAPIGet(router, path, cookie)
	if inherited.Code != 200 || !strings.Contains(inherited.Body.String(), `"multiplier":null`) {
		t.Fatalf("inherit %d %s", inherited.Code, inherited.Body.String())
	}
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
	for _, body := range []string{`{"multiplier":"1.2x"}`, `{"multiplier":"120%"}`, `{"multiplier":"1.2X"}`, `{"multiplier":1.2}`} {
		res := serveCredentialMutation(router, http.MethodPut, path, body, cookie)
		if res.Code != 200 {
			t.Fatalf("save %d %s", res.Code, res.Body.String())
		}
		var dto service.CredentialModelMultiplier
		if err := json.Unmarshal(res.Body.Bytes(), &dto); err != nil {
			t.Fatal(err)
		}
		if dto.Model != model || dto.SubjectID != credential.SubjectID || dto.Multiplier == nil || *dto.Multiplier != 1.2 {
			t.Fatalf("canonical %+v", dto)
		}
		for _, secret := range []string{identity.Identity, identity.LookupKey, "lookup_key"} {
			if strings.Contains(res.Body.String(), secret) {
				t.Fatalf("unsafe response %s", res.Body.String())
			}
		}
		assertQueries("12", true)
	}
	list := serveAPIGet(router, listPath, cookie)
	var configs service.CredentialModelMultipliers
	if list.Code != 200 || json.Unmarshal(list.Body.Bytes(), &configs) != nil || len(configs.Models) != 1 {
		t.Fatalf("unique list %d %s", list.Code, list.Body.String())
	}
	before := catalog.Snapshot()
	for _, body := range []string{`{}`, `{"multiplier":null}`, `{"multiplier":" "}`, `{"multiplier":"-1"}`, `{"multiplier":"NaN"}`, `{"multiplier":"Infinity"}`, `{"multiplier":"20%x"}`, `{"multiplier":true}`, `{"multiplier":1e999}`, `{"multiplier":1,"key":"synthetic-private-key"}`, `{"multiplier":1} {}`} {
		res := serveCredentialMutation(router, http.MethodPut, path, body, cookie)
		if res.Code != 400 || !strings.Contains(res.Body.String(), `"field":"multiplier"`) || strings.Contains(res.Body.String(), identity.LookupKey) || catalog.Snapshot() != before {
			t.Fatalf("atomic invalid %d %s", res.Code, res.Body.String())
		}
	}
	restarted, err := repository.LoadPricingSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	router, cookie = credentialPricingRouter(t, db, pricing.NewCatalog(restarted))
	readback := serveAPIGet(router, path, cookie)
	if readback.Code != 200 || !strings.Contains(readback.Body.String(), `"multiplier":1.2`) {
		t.Fatalf("restart %d %s", readback.Code, readback.Body.String())
	}
	assertQueries("12", true)
	cleared := serveCredentialMutation(router, http.MethodDelete, path, "", cookie)
	if cleared.Code != 200 || !strings.Contains(cleared.Body.String(), `"multiplier":null`) {
		t.Fatalf("clear %d %s", cleared.Code, cleared.Body.String())
	}
	assertQueries("30", false)
	if res := serveAPIGet(router, listPath, cookie); res.Code != 200 || !strings.Contains(res.Body.String(), `"models":[]`) {
		t.Fatal(res.Body.String())
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		if res := serveCredentialMutation(router, method, "/api/v1/pricing/credentials/cred_absent/model?model=M", `{"multiplier":1}`, cookie); res.Code != 404 {
			t.Fatalf("missing subject %d", res.Code)
		}
		if res := serveCredentialMutation(router, method, strings.Split(path, "?")[0], `{"multiplier":1}`, cookie); res.Code != 400 || !strings.Contains(res.Body.String(), `"field":"model"`) {
			t.Fatalf("missing model %d %s", res.Code, res.Body.String())
		}
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

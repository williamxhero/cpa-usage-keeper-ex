package test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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

func TestCredentialOnlyAmbiguityServiceAndHTTPExplanation(t *testing.T) {
	for _, mode := range []string{"default", "model"} {
		t.Run(mode, func(t *testing.T) {
			db := openAPITestDatabase(t)
			ctx := context.Background()
			identity := entities.UsageIdentity{Name: "Synthetic safe account", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-private-identity", LookupKey: "synthetic-private-source-key", Type: "openai", BindingIdentityStatus: "unique"}
			if err := db.Create(&identity).Error; err != nil {
				t.Fatal(err)
			}
			snapshot, err := repository.LoadPricingSnapshot(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			catalog := pricing.NewCatalog(snapshot)
			prices := service.NewPricingService(db, catalog)
			if _, err := prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "base", PromptPricePer1M: 10, PriceMultiplier: new(3.0)}); err != nil {
				t.Fatal(err)
			}
			router, cookie := credentialPricingRouter(t, db, catalog)
			bound := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, identity.ID), cookie)
			var credential safeCredential
			if bound.Code != http.StatusCreated || json.Unmarshal(bound.Body.Bytes(), &credential) != nil {
				t.Fatal(bound.Body.String())
			}
			path := "/api/v1/pricing/credentials/" + credential.SubjectID + "/default"
			if mode == "model" {
				path = "/api/v1/pricing/credentials/" + credential.SubjectID + "/model?model=base"
			}
			saved := serveCredentialMutation(router, http.MethodPut, path, `{"multiplier":0.2}`, cookie)
			if saved.Code != http.StatusOK {
				t.Fatal(saved.Body.String())
			}
			start := time.Now().Truncate(time.Hour).Add(-6 * time.Hour)
			end := start.Add(6 * time.Hour)
			event := entities.UsageEvent{EventKey: "credential-only-explanation", Model: "base", AuthType: "apikey", AuthIndex: identity.Identity, Timestamp: start.Add(time.Minute), InputTokens: 1_000_000, TotalTokens: 1_000_000}
			if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{event}); err != nil {
				t.Fatal(err)
			}
			query := "?" + url.Values{"range": {"custom"}, "unit": {"hour"}, "start": {start.Format(time.RFC3339)}, "end": {end.Format(time.RFC3339)}}.Encode()
			assertExplanation := func(cost float64, warning string) {
				t.Helper()
				if catalog.NewResolver().HasChannels() {
					t.Fatal("fixture must have no channels")
				}
				page, err := service.NewUsageService(db, catalog).ListUsageEvents(ctx, servicedto.UsageFilter{StartTime: &start, EndTime: &end, EndExclusive: true})
				if err != nil || len(page.Events) != 1 {
					t.Fatalf("service page %+v %v", page, err)
				}
				response := serveAPIGet(router, "/api/v1/usage/events"+query, cookie)
				var dto struct {
					SnapshotID string `json:"pricing_snapshot_id"`
					Events     []struct {
						Cost       float64                `json:"cost_usd"`
						Available  bool                   `json:"cost_available"`
						SnapshotID string                 `json:"pricing_snapshot_id"`
						Selection  *pricing.CostSelection `json:"pricing_selection"`
						DualCosts  pricing.DualCosts      `json:"dual_costs"`
					} `json:"events"`
				}
				if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &dto) != nil || len(dto.Events) != 1 {
					t.Fatalf("HTTP page %d %s", response.Code, response.Body.String())
				}
				actual := dto.Events[0]
				selection := actual.Selection
				if math.Abs(actual.Cost-cost) > 1e-9 || !actual.Available || selection == nil || selection.AttributionWarning != warning || selection.ChannelID != "" || selection.ChannelName != "" || dto.SnapshotID != catalog.Snapshot().ID() || actual.SnapshotID != dto.SnapshotID || selection.SnapshotID != dto.SnapshotID || page.PricingSnapshotID != dto.SnapshotID || page.Events[0].PricingSnapshotID != dto.SnapshotID {
					t.Fatalf("unpinned/incorrect HTTP explanation %+v", dto)
				}
				wantScope := "credential_" + mode
				if warning != "" {
					wantScope = "legacy"
					if selection.SubjectID != "" {
						t.Fatal("guessed ambiguous subject")
					}
				}
				if selection.Scope != wantScope || actual.DualCosts.Configured.TotalCostUSD == nil || *actual.DualCosts.Configured.TotalCostUSD != cost || actual.DualCosts.Reference.TotalCostUSD == nil || *actual.DualCosts.Reference.TotalCostUSD != 10 || !actual.DualCosts.Configured.Complete || !actual.DualCosts.Reference.Complete {
					t.Fatalf("changed arithmetic/state %+v", actual)
				}
				body, err := json.Marshal(selection)
				serviceBody, serviceErr := json.Marshal(page.Events[0].PricingSelection)
				if err != nil || serviceErr != nil || string(body) != string(serviceBody) || strings.Contains(string(body), identity.Identity) || strings.Contains(string(body), identity.LookupKey) {
					t.Fatalf("unsafe/mismatched explanation %s %s", body, serviceBody)
				}
			}
			assertExplanation(2, "")
			old := catalog.NewResolver()
			// Persisted ambiguous metadata disables exact attribution, not saved pricing.
			if err := db.Model(&identity).Update("binding_identity_status", "ambiguous").Error; err != nil {
				t.Fatal(err)
			}
			snapshot, err = repository.LoadPricingSnapshot(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			catalog = pricing.NewCatalog(snapshot)
			router, cookie = credentialPricingRouter(t, db, catalog)
			assertExplanation(30, "unresolved_identity")
			prior := old.Calculate(repository.UsageEventCostSubject(event))
			if prior.Cost.TotalCostUSD != 2 || prior.AttributionWarning != "" || old.SnapshotID() == catalog.Snapshot().ID() {
				t.Fatal("old snapshot mutated")
			}
			readback := serveAPIGet(router, path, cookie)
			if readback.Code != 200 || !strings.Contains(readback.Body.String(), `"multiplier":0.2`) {
				t.Fatalf("saved override lost %s", readback.Body.String())
			}
		})
	}
}

func TestCredentialDefaultAdminHTTPPersistenceCostQueriesAndSafeValidation(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	identity := entities.UsageIdentity{Name: "Synthetic safe account", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-private-identity", LookupKey: "synthetic-private-source-key", Type: "openai", BindingIdentityStatus: "unique"}
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
	if bound.Code != http.StatusCreated {
		t.Fatalf("bind %d %s", bound.Code, bound.Body.String())
	}
	var credential safeCredential
	if err := json.Unmarshal(bound.Body.Bytes(), &credential); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/pricing/credentials/" + credential.SubjectID + "/default"
	inherited := serveAPIGet(router, path, cookie)
	if inherited.Code != 200 || !strings.Contains(inherited.Body.String(), `"multiplier":null`) {
		t.Fatalf("inherited %d %s", inherited.Code, inherited.Body.String())
	}
	day := time.Now().Truncate(time.Hour).Add(-6 * time.Hour)
	timestamp := day.Add(3 * time.Hour)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{{EventKey: "http-default-query", APIGroupKey: "synthetic-downstream", Model: "base", AuthType: "apikey", AuthIndex: identity.Identity, Timestamp: timestamp, ServiceTier: "priority", ReasoningEffort: "high", InputTokens: 1_000_000, TotalTokens: 1_000_000}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, db, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	query := "?" + url.Values{"range": {"custom"}, "unit": {"hour"}, "start": {day.Format(time.RFC3339)}, "end": {day.Add(6 * time.Hour).Format(time.RFC3339)}}.Encode()
	assertHTTPPrice := func(expected float64, expectDiagnostics bool) {
		t.Helper()
		for _, endpoint := range []struct{ path, key string }{{"/api/v1/usage/overview", "total_cost"}, {"/api/v1/usage/analysis", "total_cost_usd"}, {"/api/v1/usage/events", "cost_usd"}} {
			response := serveAPIGet(router, endpoint.path+query, cookie)
			if response.Code != 200 {
				t.Fatalf("query %s %d %s", endpoint.path, response.Code, response.Body.String())
			}
			var data any
			if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			found := false
			var visit func(any)
			visit = func(value any) {
				switch v := value.(type) {
				case map[string]any:
					for key, item := range v {
						if key == endpoint.key {
							number, ok := item.(float64)
							if ok && math.Abs(number-expected) < 1e-9 {
								found = true
							}
						}
						visit(item)
					}
				case []any:
					for _, item := range v {
						visit(item)
					}
				}
			}
			visit(data)
			if !found {
				t.Fatalf("price %g missing from %s: %s", expected, endpoint.path, response.Body.String())
			}
			if expectDiagnostics && endpoint.path != "/api/v1/usage/events" && !strings.Contains(response.Body.String(), `"pricing_snapshot_id"`) {
				t.Fatalf("snapshot marker missing: %s", response.Body.String())
			}
		}
	}
	assertHTTPPrice(30, false)
	for _, body := range []string{`{"multiplier":".3"}`, `{"multiplier":"0.3x"}`, `{"multiplier":"30%"}`, `{"multiplier":0.3}`, `{"multiplier":3e-1}`} {
		saved := serveCredentialMutation(router, http.MethodPut, path, body, cookie)
		if saved.Code != 200 {
			t.Fatalf("save %s: %d %s", body, saved.Code, saved.Body.String())
		}
		var dto service.CredentialDefault
		if err := json.Unmarshal(saved.Body.Bytes(), &dto); err != nil {
			t.Fatal(err)
		}
		if dto.Multiplier == nil || *dto.Multiplier != .3 || dto.SubjectID != credential.SubjectID || dto.SnapshotID == "" {
			t.Fatalf("direct canonical DTO %+v", dto)
		}
		for _, forbidden := range []string{identity.Identity, identity.LookupKey, "identity", "lookup_key", "data"} {
			if strings.Contains(saved.Body.String(), forbidden) {
				t.Fatalf("unsafe/non-direct save response %s", saved.Body.String())
			}
		}
		assertHTTPPrice(3, true)
	}
	before := catalog.Snapshot()
	for _, body := range []string{`{}`, `{"multiplier":null}`, `{"multiplier":""}`, `{"multiplier":" "}`, `{"multiplier":"-1"}`, `{"multiplier":"+1"}`, `{"multiplier":-0.1}`, `{"multiplier":1e999}`, `{"multiplier":"NaN"}`, `{"multiplier":"Inf"}`, `{"multiplier":"1e2"}`, `{"multiplier":"1 x"}`, `{"multiplier":"1x%"}`, `{"multiplier":true}`, `{"multiplier":{}}`, `{"multiplier":"synthetic-private-source-key"}`, `{"multiplier":1,"lookup_key":"synthetic-private-source-key"}`, `{"multiplier":1} {"credential":"synthetic-private-source-key"}`} {
		res := serveCredentialMutation(router, http.MethodPut, path, body, cookie)
		if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), `"field":"multiplier"`) || strings.Contains(res.Body.String(), identity.LookupKey) {
			t.Fatalf("unsafe invalid response %s: %d %s", body, res.Code, res.Body.String())
		}
		if catalog.Snapshot() != before {
			t.Fatal("invalid HTTP write published")
		}
	}
	for _, test := range []struct {
		body string
		cost float64
	}{{`{"multiplier":0}`, 0}, {`{"multiplier":-0}`, 0}, {`{"multiplier":"1."}`, 10}, {`{"multiplier":"120%"}`, 12}, {`{"multiplier":"1.2X"}`, 12}} {
		res := serveCredentialMutation(router, http.MethodPut, path, test.body, cookie)
		if res.Code != 200 {
			t.Fatalf("explicit save %d %s", res.Code, res.Body.String())
		}
		assertHTTPPrice(test.cost, true)
	}
	cleared := serveCredentialMutation(router, http.MethodDelete, path, "", cookie)
	if cleared.Code != 200 || !strings.Contains(cleared.Body.String(), `"multiplier":null`) {
		t.Fatalf("clear %d %s", cleared.Code, cleared.Body.String())
	}
	assertHTTPPrice(30, false)
	// Persisted canonical default survives reconstructing the actual router/service.
	saved := serveCredentialMutation(router, http.MethodPut, path, `{"multiplier":"20%"}`, cookie)
	if saved.Code != 200 {
		t.Fatal(saved.Body.String())
	}
	restartedSnapshot, err := repository.LoadPricingSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	router, cookie = credentialPricingRouter(t, db, pricing.NewCatalog(restartedSnapshot))
	readback := serveAPIGet(router, path, cookie)
	if readback.Code != 200 || !strings.Contains(readback.Body.String(), `"multiplier":0.2`) {
		t.Fatalf("restart %d %s", readback.Code, readback.Body.String())
	}
	assertHTTPPrice(2, true)
	// Rollups outlive retained requests: the API must explain that historical
	// evidence cannot be recovered rather than claim a guessed complete fee.
	if err := db.Where("event_key = ?", "http-default-query").Delete(&entities.UsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"/api/v1/usage/overview", "/api/v1/usage/analysis"} {
		response := serveAPIGet(router, endpoint+query, cookie)
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"unavailable_reason":"retained_pricing_evidence_incomplete"`) || !strings.Contains(response.Body.String(), `"cost_available":false`) || !strings.Contains(response.Body.String(), `"pricing_snapshot_id"`) {
			t.Fatalf("missing public incompleteness: %d %s", response.Code, response.Body.String())
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		res := serveCredentialMutation(router, method, "/api/v1/pricing/credentials/cred_absent/default", `{"multiplier":1}`, cookie)
		if res.Code != 404 {
			t.Fatalf("missing subject %s %d %s", method, res.Code, res.Body.String())
		}
	}
}

func TestCredentialDefaultRoutesKeepAdminReadOnlyAndUnauthenticatedBoundaries(t *testing.T) {
	db := openAPITestDatabase(t)
	sessions := auth.NewSessionManager(time.Hour)
	token, _, err := sessions.CreateAPIKeyViewerWithSource(42, auth.SessionSourceStandard)
	if err != nil {
		t.Fatal(err)
	}
	cfg := keeperapi.AuthConfig{Enabled: true, LoginPassword: "synthetic-password", SessionTTL: time.Hour}
	router := keeperapi.NewRouter(nil, nil, nil, service.NewPricingService(db, emptyPricingCatalogForTest()), cfg, keeperapi.NewAuthHandler(cfg, sessions), "")
	viewer := &http.Cookie{Name: standardSessionCookieName, Value: token}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		path := "/api/v1/pricing/credentials/cred_synthetic/default"
		if res := serveCredentialMutation(router, method, path, `{"multiplier":1}`); res.Code != 401 {
			t.Fatalf("anonymous %s %d", method, res.Code)
		}
		if res := serveCredentialMutation(router, method, path, `{"multiplier":1}`, viewer); res.Code != 403 {
			t.Fatalf("viewer %s %d", method, res.Code)
		}
	}
}

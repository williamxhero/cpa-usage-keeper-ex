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

func TestChannelModelsAdminHTTPFiveLayerMatrixReadbackSwitchClearRestart(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	identity := entities.UsageIdentity{Name: "Synthetic channel model account", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-channel-model-index", LookupKey: "synthetic-private-source", Type: "openai", BindingIdentityStatus: "unique"}
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
	var credential safeCredential
	if bound.Code != 201 || json.Unmarshal(bound.Body.Bytes(), &credential) != nil {
		t.Fatal(bound.Body.String())
	}
	created := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/channels", fmt.Sprintf(`{"name":"Channel model HTTP","member_subject_ids":[%q]}`, credential.SubjectID), cookie)
	var channel service.PricingChannel
	if created.Code != 200 || json.Unmarshal(created.Body.Bytes(), &channel) != nil {
		t.Fatal(created.Body.String())
	}
	channelPath := "/api/v1/pricing/channels/" + channel.ID
	modelPath := channelPath + "/model?model=observed-model"
	credentialPath := "/api/v1/pricing/credentials/" + credential.SubjectID
	day := time.Now().Truncate(time.Hour).Add(-6 * time.Hour)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{{EventKey: "channel-model-http", APIGroupKey: "synthetic-downstream", Model: "observed-model", ModelAlias: new("base"), ServiceTier: "priority", ReasoningEffort: "high", AuthType: "apikey", AuthIndex: identity.Identity, Timestamp: day.Add(time.Hour), InputTokens: 1_000_000, TotalTokens: 1_000_000}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, db, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	mutate := func(method, path, body string) {
		t.Helper()
		res := serveCredentialMutation(router, method, path, body, cookie)
		if res.Code != 200 {
			t.Fatalf("mutation %s %s %d %s", method, path, res.Code, res.Body.String())
		}
		if strings.Contains(res.Body.String(), identity.LookupKey) || strings.Contains(res.Body.String(), identity.Identity) {
			t.Fatal("private evidence escaped")
		}
	}
	assertCosts := func(want float64, scope string) {
		t.Helper()
		for _, unit := range []string{"hour", "day"} {
			startText, endText := day.Format(time.RFC3339), day.Add(6*time.Hour).Format(time.RFC3339)
			if unit == "day" {
				startText, endText = day.Format("2006-01-02"), day.Add(6*time.Hour).Format("2006-01-02")
			}
			query := "?" + url.Values{"range": {"custom"}, "unit": {unit}, "start": {startText}, "end": {endText}}.Encode()
			for _, item := range []struct{ path, key string }{{"/api/v1/usage/overview", "total_cost"}, {"/api/v1/usage/analysis", "total_cost_usd"}} {
				res := serveAPIGet(router, item.path+query, cookie)
				if res.Code != 200 || !strings.Contains(res.Body.String(), fmt.Sprintf(`"%s":%g`, item.key, want)) || !strings.Contains(res.Body.String(), `"cost_available":true`) {
					t.Fatalf("query %s %d %s", item.path, res.Code, res.Body.String())
				}
			}
			res := serveAPIGet(router, "/api/v1/usage/overview/comparisons"+query, cookie)
			var comparisons struct {
				Channels []struct {
					Key  string
					Cost *float64
				}
				Models      []struct{ Cost *float64 }
				AIProviders []struct{ Cost *float64 } `json:"ai_providers"`
			}
			if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &comparisons) != nil || len(comparisons.Channels) != 1 || comparisons.Channels[0].Key != channel.ID || comparisons.Channels[0].Cost == nil || math.Abs(*comparisons.Channels[0].Cost-want) > 1e-9 {
				t.Fatalf("channels %s", res.Body.String())
			}
			for _, rows := range [][]struct{ Cost *float64 }{comparisons.Models, comparisons.AIProviders} {
				sum := 0.0
				for _, row := range rows {
					if row.Cost == nil {
						t.Fatal("missing configured cost")
					}
					sum += *row.Cost
				}
				if math.Abs(sum-want) > 1e-9 {
					t.Fatalf("comparison sum %g want %g", sum, want)
				}
			}
			res = serveAPIGet(router, "/api/v1/usage/events"+query, cookie)
			var details struct {
				Events []struct {
					Cost      float64                `json:"cost_usd"`
					Available bool                   `json:"cost_available"`
					ChannelID string                 `json:"channel_id"`
					Selection *pricing.CostSelection `json:"pricing_selection"`
				}
			}
			if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &details) != nil || len(details.Events) != 1 || !details.Events[0].Available || details.Events[0].ChannelID != channel.ID || math.Abs(details.Events[0].Cost-want) > 1e-9 {
				t.Fatalf("details %s", res.Body.String())
			}
			exported := serveAPIGet(router, "/api/v1/usage/events/export"+query+"&format=json", cookie)
			var exportDetails struct {
				Events []struct {
					Cost float64 `json:"cost_usd"`
				}
			}
			if exported.Code != 200 || json.Unmarshal(exported.Body.Bytes(), &exportDetails) != nil || len(exportDetails.Events) != 1 || math.Abs(exportDetails.Events[0].Cost-want) > 1e-9 {
				t.Fatalf("export %s", exported.Body.String())
			}
			selection := details.Events[0].Selection
			if scope == "" {
				if selection != nil {
					t.Fatalf("legacy changed %+v", selection)
				}
			} else if selection == nil || selection.Scope != scope || selection.BaselineModel != "base" || selection.BaselineCostUSD == nil || *selection.BaselineCostUSD != 10 {
				t.Fatalf("selection %s", res.Body.String())
			}
		}
	}
	assertCosts(30, "")
	mutate("PUT", channelPath+"/default", `{"multiplier":"20%"}`)
	assertCosts(2, "channel_default")
	mutate("PUT", modelPath, `{"mode":"multiplier","multiplier":"1.1x"}`)
	assertCosts(11, "channel_model")
	mutate("PUT", credentialPath+"/default", `{"multiplier":".3"}`)
	assertCosts(3, "credential_default")
	mutate("PUT", credentialPath+"/model?model=observed-model", `{"multiplier":"1.2"}`)
	assertCosts(12, "credential_model")
	restarted, err := repository.LoadPricingSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	catalog = pricing.NewCatalog(restarted)
	router, cookie = credentialPricingRouter(t, db, catalog)
	assertCosts(12, "credential_model")
	mutate("DELETE", credentialPath+"/model?model=observed-model", "")
	assertCosts(3, "credential_default")
	mutate("DELETE", credentialPath+"/default", "")
	assertCosts(11, "channel_model")
	fixed := `{"mode":"fixed","fixed":{"prompt_price_per_1m":4,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4,"pricing_style":"openai"}}`
	mutate("PUT", modelPath, fixed)
	assertCosts(4, "channel_model")
	for _, path := range []string{modelPath, channelPath + "/models"} {
		res := serveAPIGet(router, path, cookie)
		if res.Code != 200 || !strings.Contains(res.Body.String(), `"mode":"fixed"`) || !strings.Contains(res.Body.String(), `"multiplier":null`) {
			t.Fatalf("readback %s", res.Body.String())
		}
	}
	mutate("PUT", modelPath, `{"mode":"multiplier","multiplier":"0","fixed":"synthetic-private-source"}`)
	assertCosts(0, "channel_model")
	mutate("PUT", modelPath, fixed)
	assertCosts(4, "channel_model")
	mutate("DELETE", modelPath, "")
	assertCosts(2, "channel_default")
	mutate("DELETE", channelPath+"/default", "")
	assertCosts(30, "")
}

func TestChannelFixedAdminHTTPAtomicInvalidInputsNoBaselineAndInactiveFields(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	identity := entities.UsageIdentity{Name: "Synthetic channel fixed", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-channel-fixed", Type: "openai", BindingIdentityStatus: "unique"}
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
	router, cookie := credentialPricingRouter(t, db, catalog)
	bound := serveCredentialMutation(router, "POST", "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, identity.ID), cookie)
	var credential safeCredential
	if bound.Code != 201 || json.Unmarshal(bound.Body.Bytes(), &credential) != nil {
		t.Fatal(bound.Body.String())
	}
	created := serveCredentialMutation(router, "POST", "/api/v1/pricing/channels", fmt.Sprintf(`{"name":"Fixed channel HTTP","member_subject_ids":[%q]}`, credential.SubjectID), cookie)
	var channel service.PricingChannel
	if created.Code != 200 || json.Unmarshal(created.Body.Bytes(), &channel) != nil {
		t.Fatal(created.Body.String())
	}
	path := "/api/v1/pricing/channels/" + channel.ID + "/model?model=unpriced"
	fixed := `{"mode":"fixed","fixed":{"prompt_price_per_1m":1,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4,"pricing_style":"openai"}}`
	if res := serveCredentialMutation(router, "PUT", path, fixed, cookie); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	day := time.Now().Truncate(time.Hour).Add(-6 * time.Hour)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{{EventKey: "channel-fixed-http", APIGroupKey: "synthetic-downstream", Model: "unpriced", AuthType: "apikey", AuthIndex: identity.Identity, Timestamp: day.Add(time.Hour), InputTokens: 3_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000, CacheCreationTokens: 1_000_000, TotalTokens: 4_000_000}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, db, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	query := "?" + url.Values{"range": {"custom"}, "unit": {"hour"}, "start": {day.Format(time.RFC3339)}, "end": {day.Add(6 * time.Hour).Format(time.RFC3339)}}.Encode()
	assertQueries := func(cost float64, available bool, scope string) {
		t.Helper()
		for _, item := range []struct{ path, key string }{{"/api/v1/usage/overview", "total_cost"}, {"/api/v1/usage/analysis", "total_cost_usd"}, {"/api/v1/usage/events", "cost_usd"}} {
			res := serveAPIGet(router, item.path+query, cookie)
			if res.Code != 200 || !strings.Contains(res.Body.String(), fmt.Sprintf(`"%s":%g`, item.key, cost)) || !strings.Contains(res.Body.String(), fmt.Sprintf(`"cost_available":%t`, available)) {
				t.Fatalf("query %s %s", item.path, res.Body.String())
			}
			if item.path == "/api/v1/usage/events" && scope != "" {
				for _, fact := range []string{`"scope":"` + scope + `"`, `"baseline_cost_usd":null`, `"baseline_available":false`, `"baseline_unavailable_reason":"missing_baseline"`} {
					if !strings.Contains(res.Body.String(), fact) {
						t.Fatalf("missing independent reference %s", res.Body.String())
					}
				}
			}
		}
	}
	assertQueries(10, true, "channel_model")
	previous := catalog.Snapshot()
	base := map[string]any{"prompt_price_per_1m": 1, "completion_price_per_1m": 2, "cache_read_price_per_1m": 3, "cache_write_price_per_1m": 4, "pricing_style": "openai"}
	for _, field := range []string{"prompt_price_per_1m", "completion_price_per_1m", "cache_read_price_per_1m", "cache_write_price_per_1m"} {
		for _, invalid := range []any{nil, " ", -1, "NaN", "Infinity", "2x", "20%", "synthetic-private-token", true, "1e2", 6e307} {
			target := map[string]any{}
			for key, value := range base {
				target[key] = value
			}
			target[field] = invalid
			body, _ := json.Marshal(map[string]any{"mode": "fixed", "fixed": target})
			res := serveCredentialMutation(router, "PUT", path, string(body), cookie)
			if res.Code != 400 || catalog.Snapshot() != previous || strings.Contains(res.Body.String(), "synthetic-private-token") {
				t.Fatalf("invalid %s=%v %d %s", field, invalid, res.Code, res.Body.String())
			}
		}
		target := map[string]any{}
		for key, value := range base {
			if key != field {
				target[key] = value
			}
		}
		body, _ := json.Marshal(map[string]any{"mode": "fixed", "fixed": target})
		if res := serveCredentialMutation(router, "PUT", path, string(body), cookie); res.Code != 400 || catalog.Snapshot() != previous {
			t.Fatalf("omitted %s %s", field, res.Body.String())
		}
	}
	for _, body := range []string{`{"mode":"fixed"}`, `{"mode":"fixed","fixed":null}`, `{"mode":"invalid","multiplier":1}`, strings.Replace(fixed, `,"pricing_style":"openai"`, "", 1), strings.Replace(fixed, `"openai"`, `"guessed"`, 1), `{"mode":"fixed","fixed":{"prompt_price_per_1m":6e294,"completion_price_per_1m":6e294,"cache_read_price_per_1m":6e294,"cache_write_price_per_1m":6e294,"pricing_style":"openai"}}`, fixed + ` {}`, strings.Replace(fixed, `"mode":"fixed"`, `"unknown":"synthetic-private-token","mode":"fixed"`, 1)} {
		res := serveCredentialMutation(router, "PUT", path, body, cookie)
		if res.Code != 400 || catalog.Snapshot() != previous || strings.Contains(res.Body.String(), "synthetic-private-token") {
			t.Fatalf("invalid atomic %s", res.Body.String())
		}
	}
	assertQueries(10, true, "channel_model")
	credentialPath := "/api/v1/pricing/credentials/" + credential.SubjectID
	for _, higher := range []string{credentialPath + "/default", credentialPath + "/model?model=unpriced"} {
		if res := serveCredentialMutation(router, "PUT", higher, `{"multiplier":"0"}`, cookie); res.Code != 200 {
			t.Fatal(res.Body.String())
		}
		scope := "credential_default"
		if strings.Contains(higher, "/model?") {
			scope = "credential_model"
		}
		assertQueries(0, false, scope)
		if res := serveCredentialMutation(router, "DELETE", higher, "", cookie); res.Code != 200 {
			t.Fatal(res.Body.String())
		}
		assertQueries(10, true, "channel_model")
	}
	zero := `{"mode":"fixed","multiplier":"synthetic-private-token","fixed":{"prompt_price_per_1m":0,"completion_price_per_1m":"0","cache_read_price_per_1m":0,"cache_write_price_per_1m":0,"pricing_style":"claude"}}`
	if res := serveCredentialMutation(router, "PUT", path, zero, cookie); res.Code != 200 || strings.Contains(res.Body.String(), "synthetic-private-token") {
		t.Fatal(res.Body.String())
	}
	assertQueries(0, true, "channel_model")
	if res := serveCredentialMutation(router, "PUT", path, `{"mode":"multiplier","multiplier":"0x","fixed":"synthetic-private-token"}`, cookie); res.Code != 200 || strings.Contains(res.Body.String(), `"fixed"`) {
		t.Fatal(res.Body.String())
	}
	assertQueries(0, false, "channel_model")
	if res := serveCredentialMutation(router, "PUT", path, fixed, cookie); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	restarted, err := repository.LoadPricingSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	router, cookie = credentialPricingRouter(t, db, pricing.NewCatalog(restarted))
	assertQueries(10, true, "channel_model")
	if res := serveCredentialMutation(router, "DELETE", path, "", cookie); res.Code != 200 || !strings.Contains(res.Body.String(), `"mode":"inherit"`) {
		t.Fatal(res.Body.String())
	}
	assertQueries(0, false, "")
}

func TestChannelModelRoutesAdminAuthorizationAndSafeMissingTargets(t *testing.T) {
	db := openAPITestDatabase(t)
	sessions := auth.NewSessionManager(time.Hour)
	token, _, err := sessions.CreateAPIKeyViewerWithSource(42, auth.SessionSourceStandard)
	if err != nil {
		t.Fatal(err)
	}
	cfg := keeperapi.AuthConfig{Enabled: true, LoginPassword: "synthetic-password", SessionTTL: time.Hour}
	router := keeperapi.NewRouter(nil, nil, nil, service.NewPricingService(db, emptyPricingCatalogForTest()), cfg, keeperapi.NewAuthHandler(cfg, sessions), "")
	viewer := &http.Cookie{Name: standardSessionCookieName, Value: token}
	for _, endpoint := range []struct{ method, path string }{{"GET", "/models"}, {"GET", "/model?model=base"}, {"PUT", "/model?model=base"}, {"DELETE", "/model?model=base"}} {
		path := "/api/v1/pricing/channels/chan_missing" + endpoint.path
		if res := serveCredentialMutation(router, endpoint.method, path, `{"multiplier":1}`); res.Code != 401 {
			t.Fatalf("anonymous %d", res.Code)
		}
		if res := serveCredentialMutation(router, endpoint.method, path, `{"multiplier":1}`, viewer); res.Code != 403 {
			t.Fatalf("viewer %d", res.Code)
		}
	}
	adminRouter, cookie := credentialPricingRouter(t, db, emptyPricingCatalogForTest())
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		if res := serveCredentialMutation(adminRouter, method, "/api/v1/pricing/channels/missing/model?model=base", `{"multiplier":1}`, cookie); res.Code != 404 {
			t.Fatalf("missing %s %d %s", method, res.Code, res.Body.String())
		}
		if res := serveCredentialMutation(adminRouter, method, "/api/v1/pricing/channels/missing/model?model=%20", `{"multiplier":1}`, cookie); res.Code != 400 {
			t.Fatalf("blank %s %d", method, res.Code)
		}
	}
}

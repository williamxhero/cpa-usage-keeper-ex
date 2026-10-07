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

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
)

func TestCredentialFixedAdminHTTPNoBaselinePersisted(t *testing.T) {
	db := openAPITestDatabase(t)
	identity := entities.UsageIdentity{Name: "Synthetic fixed account", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-fixed-index", Type: "openai", BindingIdentityStatus: "unique"}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entities.CPAAPIKey{APIKey: "synthetic-downstream", DisplayKey: "Synthetic caller"}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.LoadPricingSnapshot(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	catalog := pricing.NewCatalog(snapshot)
	router, cookie := credentialPricingRouter(t, db, catalog)
	bound := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, identity.ID), cookie)
	var credential safeCredential
	if bound.Code != 201 || json.Unmarshal(bound.Body.Bytes(), &credential) != nil {
		t.Fatal(bound.Body.String())
	}
	path := "/api/v1/pricing/credentials/" + credential.SubjectID + "/model?model=unpriced"
	fixed := `{"mode":"fixed","fixed":{"prompt_price_per_1m":1,"completion_price_per_1m":2,"cache_read_price_per_1m":3,"cache_write_price_per_1m":4,"pricing_style":"openai"}}`
	saved := serveCredentialMutation(router, http.MethodPut, path, fixed, cookie)
	if saved.Code != 200 {
		t.Fatalf("fixed save %d %s", saved.Code, saved.Body.String())
	}
	day := time.Now().Truncate(time.Hour).Add(-6 * time.Hour)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{{EventKey: "http-fixed", APIGroupKey: "synthetic-downstream", Model: "unpriced", AuthType: "apikey", AuthIndex: identity.Identity, Timestamp: day.Add(time.Hour), InputTokens: 3_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000, CacheCreationTokens: 1_000_000, TotalTokens: 4_000_000}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(context.Background(), db, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	query := "?" + url.Values{"range": {"custom"}, "unit": {"hour"}, "start": {day.Format(time.RFC3339)}, "end": {day.Add(6 * time.Hour).Format(time.RFC3339)}}.Encode()
	assertQueries := func(cost string, available bool, mode string) {
		t.Helper()
		for _, item := range []struct{ path, key string }{{"/api/v1/usage/overview", "total_cost"}, {"/api/v1/usage/analysis", "total_cost_usd"}, {"/api/v1/usage/events", "cost_usd"}} {
			res := serveAPIGet(router, item.path+query, cookie)
			if res.Code != 200 || !strings.Contains(res.Body.String(), `"`+item.key+`":`+cost) || !strings.Contains(res.Body.String(), fmt.Sprintf(`"cost_available":%t`, available)) {
				t.Fatalf("cost query %s %d %s", item.path, res.Code, res.Body.String())
			}
			if item.path == "/api/v1/usage/events" && mode != "" {
				for _, fact := range []string{`"mode":"` + mode + `"`, `"baseline_cost_usd":null`, `"baseline_available":false`, `"baseline_unavailable_reason":"missing_baseline"`} {
					if !strings.Contains(res.Body.String(), fact) {
						t.Fatalf("missing independent reference %s", res.Body.String())
					}
				}
			}
		}
	}
	assertQueries("10", true, "fixed")
	previous := catalog.Snapshot()
	base := map[string]any{"prompt_price_per_1m": 1, "completion_price_per_1m": 2, "cache_read_price_per_1m": 3, "cache_write_price_per_1m": 4, "pricing_style": "openai"}
	for _, field := range []string{"prompt_price_per_1m", "completion_price_per_1m", "cache_read_price_per_1m", "cache_write_price_per_1m"} {
		for _, invalid := range []any{nil, " ", -1, "NaN", "Infinity", "2x", "20%", "broken", true, "1e2", 6e307} {
			target := map[string]any{}
			for key, value := range base {
				target[key] = value
			}
			target[field] = invalid
			body, _ := json.Marshal(map[string]any{"mode": "fixed", "fixed": target})
			res := serveCredentialMutation(router, http.MethodPut, path, string(body), cookie)
			if res.Code != 400 || catalog.Snapshot() != previous {
				t.Fatalf("accepted %s=%v %d %s", field, invalid, res.Code, res.Body.String())
			}
		}
		target := map[string]any{}
		for key, value := range base {
			if key != field {
				target[key] = value
			}
		}
		body, _ := json.Marshal(map[string]any{"mode": "fixed", "fixed": target})
		if res := serveCredentialMutation(router, http.MethodPut, path, string(body), cookie); res.Code != 400 || catalog.Snapshot() != previous {
			t.Fatalf("omitted %s %d %s", field, res.Code, res.Body.String())
		}
	}
	for _, body := range []string{`{"mode":"fixed"}`, `{"mode":"fixed","fixed":null}`, `{"mode":"invalid","multiplier":1}`, strings.Replace(fixed, `,"pricing_style":"openai"`, "", 1), strings.Replace(fixed, `"openai"`, `"guessed"`, 1), `{"mode":"fixed","fixed":{"prompt_price_per_1m":6e294,"completion_price_per_1m":6e294,"cache_read_price_per_1m":6e294,"cache_write_price_per_1m":6e294,"pricing_style":"openai"}}`} {
		if res := serveCredentialMutation(router, http.MethodPut, path, body, cookie); res.Code != 400 || catalog.Snapshot() != previous {
			t.Fatalf("invalid mode/style/overflow %d %s", res.Code, res.Body.String())
		}
	}
	assertQueries("10", true, "fixed")
	restarted, err := repository.LoadPricingSnapshot(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	router, cookie = credentialPricingRouter(t, db, pricing.NewCatalog(restarted))
	read := serveAPIGet(router, path, cookie)
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"mode":"fixed"`) || strings.Contains(read.Body.String(), identity.Identity) {
		t.Fatalf("restart unsafe %d %s", read.Code, read.Body.String())
	}
	assertQueries("10", true, "fixed")
	// Hidden inactive fields are not parsed or charged, and target is complete.
	zero := `{"mode":"fixed","multiplier":"synthetic-private-token","fixed":{"prompt_price_per_1m":0,"completion_price_per_1m":"0","cache_read_price_per_1m":0,"cache_write_price_per_1m":0,"pricing_style":"claude"}}`
	if res := serveCredentialMutation(router, http.MethodPut, path, zero, cookie); res.Code != 200 || strings.Contains(res.Body.String(), "synthetic-private-token") {
		t.Fatalf("zero %d %s", res.Code, res.Body.String())
	}
	assertQueries("0", true, "fixed")
	if res := serveCredentialMutation(router, http.MethodPut, path, `{"mode":"multiplier","multiplier":"0x","fixed":"synthetic-private-token"}`, cookie); res.Code != 200 || strings.Contains(res.Body.String(), `"fixed"`) {
		t.Fatalf("switch multiplier %d %s", res.Code, res.Body.String())
	}
	assertQueries("0", false, "multiplier")
	if res := serveCredentialMutation(router, http.MethodPut, path, fixed, cookie); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	assertQueries("10", true, "fixed")
	if res := serveCredentialMutation(router, http.MethodDelete, path, "", cookie); res.Code != 200 || !strings.Contains(res.Body.String(), `"mode":"inherit"`) {
		t.Fatalf("clear %d %s", res.Code, res.Body.String())
	}
	assertQueries("0", false, "")
}

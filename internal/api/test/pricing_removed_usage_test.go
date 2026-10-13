package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
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

func TestRemovedPricingLayersDoNotChangeStrictUsageCostsOrExports(t *testing.T) {
	db := openAPITestDatabase(t)
	identity := seedRetainedPricingRows(t, db)
	if err := db.Create(&entities.CPAAPIKey{APIKey: "synthetic-downstream", DisplayKey: "Synthetic caller"}).Error; err != nil {
		t.Fatal(err)
	}
	catalog := emptyPricingCatalogForTest()
	router, cookie := credentialPricingRouter(t, db, catalog)
	if res := serveCredentialMutation(router, "PUT", "/api/v1/pricing/official", completeOfficialPricingJSON("base"), cookie); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	day := time.Now().Truncate(time.Hour).Add(-6 * time.Hour)
	event := entities.UsageEvent{EventKey: "retired-strict-cost", APIGroupKey: "synthetic-downstream", Model: "base", AuthType: "apikey", AuthIndex: identity.Identity, ServiceTier: "priority", ReasoningEffort: "high", Timestamp: day.Add(time.Hour), InputTokens: 3_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000, CacheCreationTokens: 1_000_000, TotalTokens: 4_000_000}
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{event}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(context.Background(), db, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	assertCosts := func(want float64, source string) {
		t.Helper()
		for _, unit := range []string{"hour", "day"} {
			start, end := day.Format(time.RFC3339), day.Add(6*time.Hour).Format(time.RFC3339)
			if unit == "day" {
				start, end = day.Format("2006-01-02"), day.Add(6*time.Hour).Format("2006-01-02")
			}
			query := "?" + url.Values{"range": {"custom"}, "unit": {unit}, "start": {start}, "end": {end}}.Encode()
			for _, endpoint := range []struct{ path, key string }{{"/usage/overview", "total_cost"}, {"/usage/analysis", "total_cost_usd"}} {
				res := serveAPIGet(router, "/api/v1"+endpoint.path+query, cookie)
				if res.Code != 200 || !strings.Contains(res.Body.String(), fmt.Sprintf(`"%s":%g`, endpoint.key, want)) || !strings.Contains(res.Body.String(), `"cost_available":true`) || !strings.Contains(res.Body.String(), `"pricing_snapshot_id":"`+catalog.Snapshot().ID()+`"`) {
					t.Fatalf("strict cost %s: %d %s", endpoint.path, res.Code, res.Body.String())
				}
			}
			res := serveAPIGet(router, "/api/v1/usage/overview/comparisons"+query, cookie)
			var comparisons struct {
				Channels    []json.RawMessage
				Models      []struct{ Cost *float64 }
				AIProviders []struct{ Cost *float64 } `json:"ai_providers"`
			}
			if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &comparisons) != nil || len(comparisons.Channels) != 0 {
				t.Fatalf("retired channel entered strict breakdown: %d %s", res.Code, res.Body.String())
			}
			for _, rows := range [][]struct{ Cost *float64 }{comparisons.Models, comparisons.AIProviders} {
				if len(rows) == 0 {
					t.Fatal("missing strict usage breakdown")
				}
				sum := 0.0
				for _, row := range rows {
					if row.Cost == nil {
						t.Fatal("complete strict breakdown lost cost")
					}
					sum += *row.Cost
				}
				if sum != want {
					t.Fatalf("strict breakdown=%g want=%g", sum, want)
				}
			}
			for _, endpoint := range []string{"/api/v1/usage/events" + query, "/api/v1/usage/events/export" + query + "&format=json"} {
				res := serveAPIGet(router, endpoint, cookie)
				var page struct {
					SnapshotID string `json:"pricing_snapshot_id"`
					Events     []struct {
						Cost        float64                `json:"cost_usd"`
						Available   bool                   `json:"cost_available"`
						ChannelID   string                 `json:"channel_id"`
						ChannelName string                 `json:"channel_name"`
						SnapshotID  string                 `json:"pricing_snapshot_id"`
						Selection   *pricing.CostSelection `json:"pricing_selection"`
						DualCosts   pricing.DualCosts      `json:"dual_costs"`
					} `json:"events"`
				}
				if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &page) != nil || len(page.Events) != 1 {
					t.Fatalf("strict event/export: %d %s", res.Code, res.Body.String())
				}
				actual := page.Events[0]
				if actual.Cost != want || !actual.Available || actual.ChannelID != "" || actual.ChannelName != "" {
					t.Fatalf("strict event cost/retired attribution: %+v", actual)
				}
				selection := actual.Selection
				snapshotID := catalog.Snapshot().ID()
				if strings.HasPrefix(endpoint, "/api/v1/usage/events?") && page.SnapshotID != snapshotID {
					t.Fatal("event page omitted current root snapshot")
				}
				if actual.SnapshotID != snapshotID || selection == nil || selection.SnapshotID != snapshotID || selection.Scope != source || selection.Legacy || len(selection.MatchedRules) != 0 || selection.LegacyModelMultiplier != 1 || selection.LegacyRuleMultiplier != 1 || selection.FinalMultiplier != 1 {
					t.Fatalf("strict explanation read legacy layers: %+v", page)
				}
				configured, reference := actual.DualCosts.Configured, actual.DualCosts.Reference
				if configured.TotalCostUSD == nil || *configured.TotalCostUSD != want || !configured.Complete || !configured.HasKnown || configured.Status != "complete" || reference.TotalCostUSD == nil || *reference.TotalCostUSD != 10 || !reference.Complete || !reference.HasKnown || reference.Status != "complete" || !reflect.DeepEqual(selection.DualCosts, actual.DualCosts) {
					t.Fatalf("strict dual cost compatibility: %+v", actual)
				}
				encoded, err := json.Marshal(selection)
				if err != nil {
					t.Fatal(err)
				}
				for _, secret := range []string{identity.Identity, identity.LookupKey, "synthetic-downstream"} {
					if strings.Contains(string(encoded), secret) {
						t.Fatal("strict explanation leaked upstream/downstream identity")
					}
				}
			}
		}
	}
	// 1M uncached prompt + 1M completion + 1M each cache segment: 1+2+3+4=10.
	assertCosts(10, "official")
	matrix := `{"cells":[{"subject_id":"cred_retained","model":"base","mode":"fixed","fixed":{"pricing_style":"openai","prompt_price_per_1m":2,"completion_price_per_1m":3,"cache_read_price_per_1m":4,"cache_write_price_per_1m":5}}]}`
	if res := serveCredentialMutation(router, "PUT", "/api/v1/pricing/credentials/matrix", matrix, cookie); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	assertCosts(14, "credential_matrix")
	before := allAPIDatabaseRows(t, db)
	snapshot := catalog.Snapshot()
	for _, route := range removedPricingRoutes() {
		assertPricingRemoved(t, serveCredentialMutation(router, route.method, "/api/v1"+route.path, route.body, cookie))
		if catalog.Snapshot() != snapshot || !reflect.DeepEqual(before, allAPIDatabaseRows(t, db)) {
			t.Fatal("retired mutation changed strict cost fixture")
		}
	}
	assertCosts(14, "credential_matrix")
	zero := strings.NewReplacer(`"prompt_price_per_1m":2`, `"prompt_price_per_1m":0`, `"completion_price_per_1m":3`, `"completion_price_per_1m":0`, `"cache_read_price_per_1m":4`, `"cache_read_price_per_1m":0`, `"cache_write_price_per_1m":5`, `"cache_write_price_per_1m":0`).Replace(matrix)
	if res := serveCredentialMutation(router, "PUT", "/api/v1/pricing/credential-model-matrix/batch", zero, cookie); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	assertCosts(0, "credential_matrix")
	clear := `{"cells":[{"subject_id":"cred_retained","model":"base","clear":true}]}`
	if res := serveCredentialMutation(router, "PUT", "/api/v1/pricing/credential-model-matrix", clear, cookie); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	assertCosts(10, "official")
	restarted, err := repository.LoadPricingSnapshot(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	catalog = pricing.NewCatalog(restarted)
	router, cookie = credentialPricingRouter(t, db, catalog)
	assertCosts(10, "official")
	// A retained old fixed row alone is not a strict baseline or matrix cell.
	missing := event
	missing.EventKey, missing.Model = "retired-strict-unpriced", "legacy-fixed"
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{missing}); err != nil {
		t.Fatal(err)
	}
	query := "?" + url.Values{"range": {"custom"}, "unit": {"hour"}, "start": {day.Format(time.RFC3339)}, "end": {day.Add(6 * time.Hour).Format(time.RFC3339)}}.Encode()
	res := serveAPIGet(router, "/api/v1/usage/events"+query, cookie)
	var page struct {
		Events []struct {
			Model     string
			Cost      float64           `json:"cost_usd"`
			Available bool              `json:"cost_available"`
			DualCosts pricing.DualCosts `json:"dual_costs"`
		}
	}
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &page) != nil || len(page.Events) != 2 {
		t.Fatalf("missing strict price: %d %s", res.Code, res.Body.String())
	}
	found := false
	for _, item := range page.Events {
		if item.Model == missing.Model {
			found = true
			if item.Cost != 0 || item.Available || item.DualCosts.Configured.TotalCostUSD != nil || item.DualCosts.Configured.Complete || item.DualCosts.Reference.TotalCostUSD != nil || item.DualCosts.Reference.Complete {
				t.Fatalf("retained fixed legacy price incorrectly used: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("missing unpriced legacy-only request")
	}
}

type pricingSyncPreservationStub struct{ refreshes int }

func (s *pricingSyncPreservationStub) RequestPricingCredentialRefresh() { s.refreshes++ }
func (s *pricingSyncPreservationStub) PricingCredentialSyncStatus() service.PricingCredentialSyncStatus {
	return service.PricingCredentialSyncStatus{State: "idle"}
}

func TestPricingCredentialSyncRemainsOperationalAndAuthenticated(t *testing.T) {
	db := openAPITestDatabase(t)
	sessions := auth.NewSessionManager(time.Hour)
	admin, _, err := sessions.Create()
	if err != nil {
		t.Fatal(err)
	}
	viewer, _, err := sessions.CreateAPIKeyViewerWithSource(42, auth.SessionSourceStandard)
	if err != nil {
		t.Fatal(err)
	}
	cfg := keeperapi.AuthConfig{Enabled: true, LoginPassword: "synthetic-password", SessionTTL: time.Hour}
	sync := &pricingSyncPreservationStub{}
	router := keeperapi.NewRouter(nil, nil, nil, service.NewPricingService(db, emptyPricingCatalogForTest()), cfg, keeperapi.NewAuthHandler(cfg, sessions), "", keeperapi.OptionalProviders{CredentialSync: sync})
	for _, route := range []struct {
		method, path string
		status       int
	}{{"GET", "/api/v1/pricing/credentials/sync-status", http.StatusOK}, {"POST", "/api/v1/pricing/credentials/refresh", http.StatusAccepted}} {
		for _, test := range []struct {
			token  string
			status int
		}{{"", http.StatusUnauthorized}, {viewer, http.StatusForbidden}, {admin, route.status}} {
			var cookies []*http.Cookie
			if test.token != "" {
				cookies = append(cookies, &http.Cookie{Name: standardSessionCookieName, Value: test.token})
			}
			res := serveCredentialMutation(router, route.method, route.path, "", cookies...)
			if res.Code != test.status {
				t.Fatalf("preserved sync %s %s: %d %s", route.method, route.path, res.Code, res.Body.String())
			}
		}
	}
	if sync.refreshes != 1 {
		t.Fatalf("authorized refresh calls=%d want=1", sync.refreshes)
	}
}

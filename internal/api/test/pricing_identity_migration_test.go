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

func TestIdentityMigrationAdminHTTPRealSaveQueriesCorrectionPrivacyAndCompleteness(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	identities := []entities.UsageIdentity{
		{Name: "Historical account", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-private-old", LookupKey: "synthetic-private-key-old", Type: "openai", BindingIdentityStatus: "unique"},
		{Name: "Rotated account", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: " synthetic-private-new ", LookupKey: "synthetic-private-key-new", Type: "openai", BindingIdentityStatus: "unique"},
	}
	if err := db.Create(&identities).Error; err != nil {
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
	router, cookie := credentialPricingRouter(t, db, catalog)
	credential := bindPricingFixture(t, db, identities[0].ID, catalog)
	subject := credential.SubjectID
	applyLegacyPricingFixture(t, prices, "PUT", "/api/v1/pricing/credentials/"+subject+"/default", `{"multiplier":"20%"}`)
	savedChannel, err := prices.(service.PricingChannelsProvider).CreatePricingChannel(ctx, service.PricingChannelInput{Name: "Saved contract", MemberSubjectIDs: []string{subject}})
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().Truncate(time.Hour).Add(-6 * time.Hour)
	events := []entities.UsageEvent{}
	for i, identity := range identities {
		events = append(events, entities.UsageEvent{EventKey: fmt.Sprintf("migration-http-%d", i), APIGroupKey: "synthetic-downstream", Model: "base", AuthType: "apikey", AuthIndex: identity.Identity, Timestamp: day.Add(3*time.Hour + time.Duration(i)*time.Minute), InputTokens: 1_000_000, TotalTokens: 1_000_000})
	}
	if _, _, err := repository.InsertUsageEvents(db, events); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, db, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	query := "?" + url.Values{"range": {"custom"}, "unit": {"hour"}, "start": {day.Format(time.RFC3339)}, "end": {day.Add(6 * time.Hour).Format(time.RFC3339)}}.Encode()
	costs := func(expected float64) {
		t.Helper()
		for _, endpoint := range []struct{ path, key string }{{"/api/v1/usage/overview", "total_cost"}, {"/api/v1/usage/analysis", "total_cost_usd"}, {"/api/v1/usage/events", "cost_usd"}} {
			res := serveAPIGet(router, endpoint.path+query, cookie)
			if res.Code != 200 {
				t.Fatal(res.Body.String())
			}
			var data any
			if err := json.Unmarshal(res.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			found := false
			sum := 0.0
			var visit func(any)
			visit = func(v any) {
				switch x := v.(type) {
				case map[string]any:
					for k, item := range x {
						if k == endpoint.key {
							if n, ok := item.(float64); ok {
								sum += n
								if math.Abs(n-expected) < 1e-9 {
									found = true
								}
							}
						}
						visit(item)
					}
				case []any:
					for _, item := range x {
						visit(item)
					}
				}
			}
			visit(data)
			if endpoint.path == "/api/v1/usage/events" {
				found = math.Abs(sum-expected) < 1e-9
			}
			if !found {
				t.Fatalf("missing actual cost %g %s", expected, res.Body.String())
			}
		}
	}
	state := func() service.PricingIdentityState {
		t.Helper()
		assertPricingRemoved(t, serveAPIGet(router, "/api/v1/pricing/identity-bindings", cookie))
		s, err := prices.(service.PricingIdentityMigrationProvider).GetPricingIdentityState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{identities[0].Identity, identities[1].Identity, identities[0].LookupKey, identities[1].LookupKey, "lookup_key", "auth_index", "file_path", "raw_identity"} {
			if strings.Contains(string(encoded), secret) {
				t.Fatal("unsafe internal read-only identity state")
			}
		}
		return s
	}
	costs(7)
	before := catalog.Snapshot()
	for _, body := range []string{`{}`, `{"confirmed":false}`, `{"raw_identity":"synthetic-private-key-new"}`, `{} {}`, `null`} {
		assertPricingRemoved(t, serveCredentialMutation(router, "POST", "/api/v1/pricing/identity-bindings/migrate", body, cookie))
		if catalog.Snapshot() != before {
			t.Fatal("retired migration published")
		}
	}
	s := state()
	ref := ""
	for _, item := range s.Directory {
		if item.Credential.DirectoryID == identities[1].ID {
			ref = item.Ref
		}
	}
	migrated, err := prices.(service.PricingIdentityMigrationProvider).MigratePricingIdentity(ctx, service.PricingIdentityMigrationInput{SubjectID: subject, DirectoryRef: ref, SnapshotID: s.SnapshotID, Confirmed: true})
	if err != nil || migrated.SubjectID != subject || migrated.BindingRef == "" {
		t.Fatalf("migration fixture: %+v %v", migrated, err)
	}
	costs(4)
	assertPricingRemoved(t, serveAPIGet(router, "/api/v1/pricing/channels/"+savedChannel.ID, cookie))
	s = state()
	_, err = prices.(service.PricingIdentityMigrationProvider).CorrectPricingIdentity(ctx, service.PricingIdentityCorrectionInput{BindingRef: "binding_" + subject, ExpectedSubjectID: subject, Action: "unbind", SnapshotID: s.SnapshotID, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	costs(7)
	s = state()
	_, err = prices.(service.PricingIdentityMigrationProvider).CorrectPricingIdentity(ctx, service.PricingIdentityCorrectionInput{BindingRef: "binding_" + subject, ExpectedSubjectID: subject, TargetSubjectID: subject, Action: "rebind", SnapshotID: s.SnapshotID, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	costs(4)
	// No historical reconstruction promise after genuinely missing facts.
	if err := db.Where("event_key = ?", "migration-http-0").Delete(&entities.UsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/usage/overview", "/api/v1/usage/analysis"} {
		res := serveAPIGet(router, path+query, cookie)
		if res.Code != 200 || !strings.Contains(res.Body.String(), `"unavailable_reason":"retained_pricing_evidence_incomplete"`) {
			t.Fatalf("hidden history gap %s", res.Body.String())
		}
	}
}

func TestIdentityMigrationHTTPAdminBoundaries(t *testing.T) {
	db := openAPITestDatabase(t)
	sessions := auth.NewSessionManager(time.Hour)
	token, _, err := sessions.CreateAPIKeyViewerWithSource(42, auth.SessionSourceStandard)
	if err != nil {
		t.Fatal(err)
	}
	cfg := keeperapi.AuthConfig{Enabled: true, LoginPassword: "synthetic-password", SessionTTL: time.Hour}
	router := keeperapi.NewRouter(nil, nil, nil, service.NewPricingService(db, emptyPricingCatalogForTest()), cfg, keeperapi.NewAuthHandler(cfg, sessions), "")
	viewer := &http.Cookie{Name: standardSessionCookieName, Value: token}
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/pricing/identity-bindings"}, {"POST", "/api/v1/pricing/identity-bindings/migrate"}, {"PUT", "/api/v1/pricing/identity-bindings/binding_synthetic/correction"}} {
		if res := serveCredentialMutation(router, route.method, route.path, `{}`); res.Code != 401 {
			t.Fatalf("anonymous %d", res.Code)
		}
		if res := serveCredentialMutation(router, route.method, route.path, `{}`, viewer); res.Code != 403 {
			t.Fatalf("viewer %d", res.Code)
		}
	}
}

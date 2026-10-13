package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

func TestPricingChannelsAdminHTTPRealPersistenceQueriesReadbackAndDeletion(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	identities := []entities.UsageIdentity{
		{Name: "Synthetic OpenAI A", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-a", LookupKey: "synthetic-private-source-a", Type: "openai", BindingIdentityStatus: "unique"},
		{Name: "Synthetic OpenAI B", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-b", LookupKey: "synthetic-private-source-b", Type: "openai", BindingIdentityStatus: "unique"},
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
	channels := []service.PricingChannel{}
	subjects := []string{}
	for i, identity := range identities {
		credential := bindPricingFixture(t, db, identity.ID, catalog)
		subjects = append(subjects, credential.SubjectID)
		channel, err := prices.(service.PricingChannelsProvider).CreatePricingChannel(ctx, service.PricingChannelInput{Name: fmt.Sprintf("Channel %c", 'A'+i), MemberSubjectIDs: []string{credential.SubjectID}})
		if err != nil {
			t.Fatal(err)
		}
		channels = append(channels, channel)
		_, err = prices.(service.PricingChannelsProvider).SetChannelDefault(ctx, channel.ID, []string{"20%", ".5x"}[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	day := time.Now().Truncate(time.Hour).Add(-6 * time.Hour)
	events := []entities.UsageEvent{}
	for i, identity := range identities {
		events = append(events, entities.UsageEvent{EventKey: fmt.Sprintf("channel-http-%d", i), APIGroupKey: "synthetic-downstream", Model: "base", AuthType: "apikey", AuthIndex: identity.Identity, Timestamp: day.Add(3*time.Hour + time.Duration(i)*time.Minute), InputTokens: 1_000_000, TotalTokens: 1_000_000})
	}
	if _, _, err := repository.InsertUsageEvents(db, events); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, db, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	query := "?" + url.Values{"range": {"custom"}, "unit": {"hour"}, "start": {day.Format(time.RFC3339)}, "end": {day.Add(6 * time.Hour).Format(time.RFC3339)}}.Encode()
	assertCosts := func(a, b float64) {
		t.Helper()
		overview := serveAPIGet(router, "/api/v1/usage/overview"+query, cookie)
		if overview.Code != 200 {
			t.Fatal(overview.Body.String())
		}
		var summary struct {
			Summary struct {
				TotalCost     float64 `json:"total_cost"`
				CostAvailable bool    `json:"cost_available"`
			}
			Series struct {
				Cost []float64 `json:"cost"`
			}
		}
		if err := json.Unmarshal(overview.Body.Bytes(), &summary); err != nil {
			t.Fatal(err)
		}
		if summary.Summary.TotalCost != a+b || !summary.Summary.CostAvailable {
			t.Fatalf("overview %+v", summary)
		}
		sum := 0.0
		for _, value := range summary.Series.Cost {
			sum += value
		}
		if sum != a+b {
			t.Fatalf("time series %g", sum)
		}
		response := serveAPIGet(router, "/api/v1/usage/overview/comparisons"+query, cookie)
		if response.Code != 200 {
			t.Fatal(response.Body.String())
		}
		var comparison struct {
			Channels []struct {
				Key, Label string
				Cost       *float64
			}
			Models      []struct{ Cost *float64 }
			AIProviders []struct{ Cost *float64 } `json:"ai_providers"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &comparison); err != nil {
			t.Fatal(err)
		}
		if len(comparison.Channels) != 2 {
			t.Fatalf("channel breakdown %s", response.Body.String())
		}
		for _, row := range comparison.Channels {
			want := b
			if row.Key == channels[0].ID {
				want = a
			}
			if row.Cost == nil || *row.Cost != want {
				t.Fatalf("channel %+v", row)
			}
		}
		sum = 0
		for _, row := range comparison.AIProviders {
			if row.Cost == nil {
				t.Fatal("credential incomplete")
			}
			sum += *row.Cost
		}
		if sum != a+b {
			t.Fatalf("credential sum %g", sum)
		}
		response = serveAPIGet(router, "/api/v1/usage/events"+query, cookie)
		if response.Code != 200 {
			t.Fatal(response.Body.String())
		}
		var details struct {
			Events []struct {
				ChannelID   string  `json:"channel_id"`
				ChannelName string  `json:"channel_name"`
				Cost        float64 `json:"cost_usd"`
				Available   bool    `json:"cost_available"`
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &details); err != nil {
			t.Fatal(err)
		}
		if len(details.Events) != 2 {
			t.Fatal(len(details.Events))
		}
		sum = 0
		for _, event := range details.Events {
			sum += event.Cost
			if event.ChannelID == "" || event.ChannelName == "" || !event.Available {
				t.Fatalf("detail attribution %+v", event)
			}
		}
		if sum != a+b {
			t.Fatalf("detail sum %g", sum)
		}
	}
	assertCosts(2, 5)
	defaultPath := "/api/v1/pricing/credentials/" + subjects[0] + "/default"
	applyLegacyPricingFixture(t, prices, "PUT", defaultPath, `{"multiplier":0.3}`)
	assertCosts(3, 5)
	applyLegacyPricingFixture(t, prices, "DELETE", defaultPath, "")
	assertCosts(2, 5)
	path := "/api/v1/pricing/channels/" + channels[0].ID
	_, err = prices.(service.PricingChannelsProvider).UpdatePricingChannel(ctx, channels[0].ID, service.PricingChannelInput{Name: "Renamed channel", MemberSubjectIDs: []string{subjects[0]}})
	if err != nil {
		t.Fatal(err)
	}
	restartedSnapshot, err := repository.LoadPricingSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	catalog = pricing.NewCatalog(restartedSnapshot)
	prices = service.NewPricingService(db, catalog)
	router, cookie = credentialPricingRouter(t, db, catalog)
	assertCosts(2, 5)
	assertPricingRemoved(t, serveAPIGet(router, path, cookie))
	for _, body := range []string{`{}`, `{"name":"Bad","member_subject_ids":null}`, fmt.Sprintf(`{"name":"Duplicate","member_subject_ids":[%q,%q]}`, subjects[0], subjects[0]), fmt.Sprintf(`{"name":"Conflict","member_subject_ids":[%q]}`, subjects[1]), `{"name":"Unknown","member_subject_ids":["missing"]}`, `{"name":"sk-synthetic-secret","member_subject_ids":[]}`, `{"name":"synthetic-private-source-a","member_subject_ids":[]}`, `{"name":"synthetic-a","member_subject_ids":[]}`, `{"name":"Safe","member_subject_ids":[],"api_key":"synthetic-private-source-a"}`} {
		assertPricingRemoved(t, serveCredentialMutation(router, "PUT", path, body, cookie))
	}
	assertCosts(2, 5)
	assertPricingRemoved(t, serveCredentialMutation(router, "DELETE", path, "", cookie))
	assertPricingRemoved(t, serveCredentialMutation(router, "DELETE", path+"?confirm_dependencies=true", "", cookie))
	assertPricingRemoved(t, serveAPIGet(router, path, cookie))
	assertPricingRemoved(t, serveAPIGet(router, "/api/v1/pricing/credentials/"+subjects[0]+"/default", cookie))
	assertCosts(2, 5)
}

func TestPricingChannelRoutesKeepAdminReadOnlyAndAnonymousBoundaries(t *testing.T) {
	db := openAPITestDatabase(t)
	sessions := auth.NewSessionManager(time.Hour)
	token, _, err := sessions.CreateAPIKeyViewerWithSource(42, auth.SessionSourceStandard)
	if err != nil {
		t.Fatal(err)
	}
	cfg := keeperapi.AuthConfig{Enabled: true, LoginPassword: "synthetic-password", SessionTTL: time.Hour}
	router := keeperapi.NewRouter(nil, nil, nil, service.NewPricingService(db, emptyPricingCatalogForTest()), cfg, keeperapi.NewAuthHandler(cfg, sessions), "")
	viewer := &http.Cookie{Name: standardSessionCookieName, Value: token}
	for _, endpoint := range []struct{ method, path string }{{"GET", "/pricing/channels"}, {"POST", "/pricing/channels"}, {"GET", "/pricing/channels/chan_test"}, {"PUT", "/pricing/channels/chan_test"}, {"DELETE", "/pricing/channels/chan_test"}, {"GET", "/pricing/channels/chan_test/default"}, {"PUT", "/pricing/channels/chan_test/default"}, {"DELETE", "/pricing/channels/chan_test/default"}} {
		if response := serveCredentialMutation(router, endpoint.method, "/api/v1"+endpoint.path, `{"multiplier":1}`); response.Code != 401 {
			t.Fatalf("anonymous %s %d", endpoint.path, response.Code)
		}
		if response := serveCredentialMutation(router, endpoint.method, "/api/v1"+endpoint.path, `{"multiplier":1}`, viewer); response.Code != 403 {
			t.Fatalf("viewer %s %d", endpoint.path, response.Code)
		}
	}
}

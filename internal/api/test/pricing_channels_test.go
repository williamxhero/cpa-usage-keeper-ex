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
		bound := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, identity.ID), cookie)
		if bound.Code != 201 {
			t.Fatal(bound.Body.String())
		}
		var credential safeCredential
		if err := json.Unmarshal(bound.Body.Bytes(), &credential); err != nil {
			t.Fatal(err)
		}
		subjects = append(subjects, credential.SubjectID)
		body := fmt.Sprintf(`{"name":"Channel %c","member_subject_ids":[%q]}`, 'A'+i, credential.SubjectID)
		created := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/channels", body, cookie)
		if created.Code != 200 {
			t.Fatalf("create %d %s", created.Code, created.Body.String())
		}
		var channel service.PricingChannel
		if err := json.Unmarshal(created.Body.Bytes(), &channel); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(channel.ID, "chan_") || channel.Multiplier != nil || len(channel.MemberSubjectIDs) != 1 {
			t.Fatalf("created %+v", channel)
		}
		channels = append(channels, channel)
		multiplier := []string{"20%", ".5x"}[i]
		path := "/api/v1/pricing/channels/" + channel.ID + "/default"
		saved := serveCredentialMutation(router, http.MethodPut, path, fmt.Sprintf(`{"multiplier":%q}`, multiplier), cookie)
		if saved.Code != 200 {
			t.Fatal(saved.Body.String())
		}
		if strings.Contains(saved.Body.String(), identity.LookupKey) || strings.Contains(saved.Body.String(), identity.Identity) {
			t.Fatal("secret or raw identity in new DTO")
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
	if response := serveCredentialMutation(router, http.MethodPut, defaultPath, `{"multiplier":0.3}`, cookie); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	assertCosts(3, 5)
	if response := serveCredentialMutation(router, http.MethodDelete, defaultPath, "", cookie); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	assertCosts(2, 5)
	path := "/api/v1/pricing/channels/" + channels[0].ID
	renamed := serveCredentialMutation(router, http.MethodPut, path, fmt.Sprintf(`{"name":"Renamed channel","member_subject_ids":[%q]}`, subjects[0]), cookie)
	if renamed.Code != 200 {
		t.Fatal(renamed.Body.String())
	}
	restartedSnapshot, err := repository.LoadPricingSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	router, cookie = credentialPricingRouter(t, db, pricing.NewCatalog(restartedSnapshot))
	assertCosts(2, 5)
	readback := serveAPIGet(router, path, cookie)
	if readback.Code != 200 || !strings.Contains(readback.Body.String(), `"name":"Renamed channel"`) || !strings.Contains(readback.Body.String(), `"multiplier":0.2`) {
		t.Fatal(readback.Body.String())
	}
	for _, body := range []string{`{}`, `{"name":"Bad","member_subject_ids":null}`, fmt.Sprintf(`{"name":"Duplicate","member_subject_ids":[%q,%q]}`, subjects[0], subjects[0]), fmt.Sprintf(`{"name":"Conflict","member_subject_ids":[%q]}`, subjects[1]), `{"name":"Unknown","member_subject_ids":["missing"]}`, `{"name":"sk-synthetic-secret","member_subject_ids":[]}`, `{"name":"Safe","member_subject_ids":[],"api_key":"synthetic-private-source-a"}`} {
		response := serveCredentialMutation(router, http.MethodPut, path, body, cookie)
		if response.Code != 400 && response.Code != 409 {
			t.Fatalf("invalid accepted %d %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "synthetic-private-source-a") || strings.Contains(response.Body.String(), "sk-synthetic-secret") {
			t.Fatal("unsafe error")
		}
	}
	assertCosts(2, 5)
	response := serveCredentialMutation(router, http.MethodDelete, path, "", cookie)
	if response.Code != 409 {
		t.Fatalf("unsafe deletion %d %s", response.Code, response.Body.String())
	}
	if response := serveCredentialMutation(router, http.MethodDelete, path+"?confirm_dependencies=true", "", cookie); response.Code != 204 {
		t.Fatalf("confirmed delete %d %s", response.Code, response.Body.String())
	}
	if response := serveAPIGet(router, path, cookie); response.Code != 404 {
		t.Fatal(response.Body.String())
	}
	if response := serveAPIGet(router, "/api/v1/pricing/credentials/"+subjects[0]+"/default", cookie); response.Code != 200 {
		t.Fatal("credential history deleted")
	}
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

package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	keeperapi "cpa-usage-keeper/internal/api"
	"cpa-usage-keeper/internal/auth"
	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"gorm.io/gorm"
)

type safeCredential struct {
	DirectoryID   int64  `json:"directory_id"`
	SubjectID     string `json:"subject_id"`
	Name          string `json:"name"`
	Alias         string `json:"alias"`
	ProviderType  string `json:"provider_type"`
	AuthType      string `json:"auth_type"`
	Endpoint      string `json:"endpoint"`
	Status        string `json:"status"`
	BindingStatus string `json:"binding_status"`
}

func credentialPricingRouter(t *testing.T, db *gorm.DB, catalogs ...*pricing.Catalog) (http.Handler, *http.Cookie) {
	t.Helper()
	sessions := auth.NewSessionManager(time.Hour)
	token, _, err := sessions.CreateWithSource(auth.SessionSourceStandard)
	if err != nil {
		t.Fatal(err)
	}
	cfg := keeperapi.AuthConfig{Enabled: true, LoginPassword: "synthetic-password", SessionTTL: time.Hour}
	catalog := emptyPricingCatalogForTest()
	if len(catalogs) > 0 {
		catalog = catalogs[0]
	}
	return keeperapi.NewRouter(nil, nil, service.NewUsageService(db, catalog), service.NewPricingService(db, catalog), cfg, keeperapi.NewAuthHandler(cfg, sessions), ""), &http.Cookie{Name: standardSessionCookieName, Value: token}
}

func credentialDirectory(t *testing.T, router http.Handler, cookie *http.Cookie) []safeCredential {
	t.Helper()
	res := serveAPIGet(router, "/api/v1/pricing/credentials", cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("directory status = %d, want 200: %s", res.Code, res.Body.String())
	}
	var result struct {
		Credentials []safeCredential `json:"credentials"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Credentials
}

func TestPricingCredentialDirectoryAndErrorsUseSafeAllowlist(t *testing.T) {
	db := openAPITestDatabase(t)
	alias := "synthetic-raw-key"
	authPath := "/synthetic/auth/private-token.json"
	row := entities.UsageIdentity{Name: authPath, Alias: &alias, AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-index", Type: "openai", Provider: "synthetic-refresh-token", LookupKey: "synthetic-raw-key", FilePath: &authPath, BaseURL: "https://endpoint-user:endpoint-password@upstream.example/private-path?api_key=query-secret&access_token=query-token#fragment-secret", BindingIdentityStatus: "unique"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	router, cookie := credentialPricingRouter(t, db)
	res := serveAPIGet(router, "/api/v1/pricing/credentials", cookie)
	if res.Code != http.StatusOK {
		t.Fatal(res.Code)
	}
	for _, forbidden := range []string{alias, authPath, "private-token", "synthetic-refresh-token", "endpoint-user", "endpoint-password", "private-path", "query-secret", "query-token", "fragment-secret", "lookup_key", "file_path", "api_key"} {
		if strings.Contains(res.Body.String(), forbidden) {
			t.Fatalf("directory leaked %q: %s", forbidden, res.Body.String())
		}
	}
	directory := credentialDirectory(t, router, cookie)
	if directory[0].Endpoint != "https://upstream.example" || directory[0].Name == "" {
		t.Fatalf("lost safe metadata: %+v", directory)
	}
	for _, body := range []string{`{"directory_id":"synthetic-raw-key"}`, `{"directory_id":1,"lookup_key":"synthetic-raw-key"}`, `{"directory_id":-1}`, `{"directory_id":1} {"access_token":"query-token"}`} {
		res := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", body, cookie)
		if res.Code != http.StatusBadRequest || strings.Contains(res.Body.String(), "synthetic-raw-key") || strings.Contains(res.Body.String(), "query-token") {
			t.Fatalf("unsafe validation response: %d %s", res.Code, res.Body.String())
		}
	}
}

func TestPricingCredentialProviderTypesUseSupportedSafeAliases(t *testing.T) {
	db := openAPITestDatabase(t)
	for i, providerType := range []string{"gemini-cli", "kimi", "kimi-ai", "kimi.ai", "kimi.com", "synthetic-provider-token"} {
		row := entities.UsageIdentity{Name: "Friendly upstream", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: fmt.Sprintf("alias-index-%d", i), Type: providerType, BindingIdentityStatus: "unique"}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	router, cookie := credentialPricingRouter(t, db)
	for _, item := range credentialDirectory(t, router, cookie) {
		var original entities.UsageIdentity
		if err := db.First(&original, item.DirectoryID).Error; err != nil {
			t.Fatal(err)
		}
		want := original.Type
		if want == "synthetic-provider-token" {
			want = "unknown"
		}
		if item.ProviderType != want {
			t.Fatalf("provider %q displayed as %q, want %q", original.Type, item.ProviderType, want)
		}
	}
}

func TestPricingCredentialNamesNeverExposeAuthFileBasenamesOrTokenShapedText(t *testing.T) {
	db := openAPITestDatabase(t)
	fileName := "private-refresh-token.json"
	filePath := `C:\synthetic\auth\private-refresh-token.json`
	for i, name := range []string{fileName, "private-refresh-token", "account eyJhbGciOiJIUzI1NiJ9.synthetic.signature", "account ghp_syntheticprivate", "account abcdef0123456789abcdef0123456789abcdef"} {
		row := entities.UsageIdentity{Name: name, Alias: &name, AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: fmt.Sprintf("synthetic-index-%d", i), Type: "codex", FileName: &fileName, FilePath: &filePath, BindingIdentityStatus: "unique"}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	router, cookie := credentialPricingRouter(t, db)
	for _, item := range credentialDirectory(t, router, cookie) {
		if item.Name != "Credential" || item.Alias != "" {
			t.Fatalf("unsafe fallback name or alias: %+v", item)
		}
	}
}

func TestPricingCredentialSavedSubjectSurvivesRotationWithoutAutoMigration(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	identity := entities.UsageIdentity{Name: "Same friendly name", AuthTypeName: "apikey", Identity: "original-index", Type: "openai", LookupKey: "synthetic-secret"}
	sync := func(rows []entities.UsageIdentity) {
		t.Helper()
		if err := repository.ReplaceUsageIdentitiesForProviderTypes(ctx, db, rows, []string{"openai"}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	sync([]entities.UsageIdentity{identity})
	router, cookie := credentialPricingRouter(t, db)
	directory := credentialDirectory(t, router, cookie)
	savedResponse := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, directory[0].DirectoryID), cookie)
	if savedResponse.Code != http.StatusCreated {
		t.Fatal(savedResponse.Code)
	}
	var saved safeCredential
	if err := json.Unmarshal(savedResponse.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	identity.Identity = "rotated-index"
	sync([]entities.UsageIdentity{identity})
	router, cookie = credentialPricingRouter(t, db)
	res := serveAPIGet(router, "/api/v1/pricing/credential-subjects", cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("saved subjects status = %d: %s", res.Code, res.Body.String())
	}
	var result struct {
		Credentials []safeCredential `json:"credentials"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Credentials) != 1 || result.Credentials[0].SubjectID != saved.SubjectID || result.Credentials[0].BindingStatus != "stale" {
		t.Fatalf("rotation lost original relation: %+v", result.Credentials)
	}
	for _, credential := range credentialDirectory(t, router, cookie) {
		if credential.Status == "active" && (credential.SubjectID != "" || credential.BindingStatus != "unbound") {
			t.Fatalf("same name auto-migrated binding: %+v", credential)
		}
	}
}

func TestPricingCredentialSavedSubjectRemainsVisibleButNeverResolvesAnAmbiguousIdentity(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	row := entities.UsageIdentity{Name: "Friendly upstream", AuthTypeName: "apikey", Identity: "shared-index", Type: "openai"}
	sync := func(rows []entities.UsageIdentity) {
		t.Helper()
		if err := repository.ReplaceUsageIdentitiesForProviderTypes(ctx, db, rows, []string{"openai"}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	sync([]entities.UsageIdentity{row})
	router, cookie := credentialPricingRouter(t, db)
	directory := credentialDirectory(t, router, cookie)
	res := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, directory[0].DirectoryID), cookie)
	var saved safeCredential
	if res.Code != http.StatusCreated {
		t.Fatal(res.Code)
	}
	if err := json.Unmarshal(res.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	duplicate := row
	duplicate.Name = "Second configuration"
	sync([]entities.UsageIdentity{row, duplicate})
	for _, stale := range []bool{false, true} {
		if stale {
			sync(nil)
		}
		items := credentialDirectory(t, router, cookie)
		if len(items) != 1 || items[0].BindingStatus != "ambiguous" || items[0].SubjectID != "" {
			t.Fatalf("ambiguity must not assert saved attribution: %+v", items)
		}
		res = serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, directory[0].DirectoryID), cookie)
		if res.Code != http.StatusConflict {
			t.Fatalf("ambiguous save = %d", res.Code)
		}
		res = serveAPIGet(router, "/api/v1/pricing/credential-subjects", cookie)
		var result struct {
			Credentials []safeCredential `json:"credentials"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Credentials) != 1 || result.Credentials[0].SubjectID != saved.SubjectID || result.Credentials[0].BindingStatus != "ambiguous" {
			t.Fatalf("saved record changed on rejected save: %+v", result.Credentials)
		}
	}
}

func TestPricingCredentialRoutesKeepAdministratorAndReadOnlyBoundaries(t *testing.T) {
	db := openAPITestDatabase(t)
	sessions := auth.NewSessionManager(time.Hour)
	token, _, err := sessions.CreateAPIKeyViewerWithSource(42, auth.SessionSourceStandard)
	if err != nil {
		t.Fatal(err)
	}
	cfg := keeperapi.AuthConfig{Enabled: true, LoginPassword: "synthetic-password", SessionTTL: time.Hour}
	router := keeperapi.NewRouter(nil, nil, nil, service.NewPricingService(db, emptyPricingCatalogForTest()), cfg, keeperapi.NewAuthHandler(cfg, sessions), "")
	viewer := &http.Cookie{Name: standardSessionCookieName, Value: token}
	for _, path := range []string{"/api/v1/pricing/credentials", "/api/v1/pricing/credential-subjects"} {
		if res := serveAPIGet(router, path); res.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated read %s: %d", path, res.Code)
		}
		if res := serveAPIGet(router, path, viewer); res.Code != http.StatusForbidden {
			t.Fatalf("viewer directory read %s: %d", path, res.Code)
		}
	}
	for _, test := range []struct {
		cookie *http.Cookie
		want   int
	}{{nil, http.StatusUnauthorized}, {viewer, http.StatusForbidden}} {
		cookies := []*http.Cookie{}
		if test.cookie != nil {
			cookies = append(cookies, test.cookie)
		}
		res := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", `{"directory_id":1}`, cookies...)
		if res.Code != test.want {
			t.Fatalf("unauthorized save: %d, want %d", res.Code, test.want)
		}
	}
}

func TestPricingCredentialMissingOrUnverifiedIdentityCannotBeBound(t *testing.T) {
	db := openAPITestDatabase(t)
	rows := []entities.UsageIdentity{
		{Name: "Missing identity", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Type: "openai", BindingIdentityStatus: "unique"},
		{Name: "Legacy unverified directory", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Type: "openai", Identity: "legacy-index"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	router, cookie := credentialPricingRouter(t, db)
	for _, item := range credentialDirectory(t, router, cookie) {
		if item.BindingStatus != "unknown" || item.SubjectID != "" {
			t.Fatalf("missing evidence became attribution: %+v", item)
		}
		res := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, item.DirectoryID), cookie)
		if res.Code != http.StatusConflict {
			t.Fatalf("unverified save: %d %s", res.Code, res.Body.String())
		}
	}
	var count int64
	if err := db.Model(&entities.CredentialPricingSubject{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected save changed subjects: %d, %v", count, err)
	}
}

func TestPricingCredentialConflictingTypeDeclarationCannotBeBound(t *testing.T) {
	db := openAPITestDatabase(t)
	row := entities.UsageIdentity{Name: "Conflicting declaration", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "oauth", Identity: "synthetic-conflict", Type: "openai", BindingIdentityStatus: "unique"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	router, cookie := credentialPricingRouter(t, db)
	res := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, row.ID), cookie)
	if res.Code != http.StatusConflict {
		t.Fatalf("conflicting type declaration accepted: %d %s", res.Code, res.Body.String())
	}
	directory := credentialDirectory(t, router, cookie)
	if len(directory) != 1 || directory[0].BindingStatus != "unknown" || directory[0].SubjectID != "" {
		t.Fatalf("conflict mutated effective binding: %+v", directory)
	}
}

func TestPricingCredentialResolutionRequiresTrustedUpstreamIdentityType(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	for authType, typeName := range map[entities.UsageIdentityAuthType]string{entities.UsageIdentityAuthTypeAIProvider: "apikey", entities.UsageIdentityAuthTypeAuthFile: "oauth"} {
		if err := repository.ReplaceUsageIdentitiesForAuthType(ctx, db, []entities.UsageIdentity{{Name: "Same friendly name", AuthTypeName: typeName, Identity: "shared-index", Type: "openai"}}, authType, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	router, cookie := credentialPricingRouter(t, db)
	subjects := map[string]string{}
	for _, item := range credentialDirectory(t, router, cookie) {
		res := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, item.DirectoryID), cookie)
		if res.Code != http.StatusCreated {
			t.Fatalf("explicit typed selection rejected: %d %s", res.Code, res.Body.String())
		}
		var saved safeCredential
		if err := json.Unmarshal(res.Body.Bytes(), &saved); err != nil {
			t.Fatal(err)
		}
		subjects[item.AuthType] = saved.SubjectID
	}
	resolver, ok := service.NewPricingService(db, emptyPricingCatalogForTest()).(interface {
		ResolvePricingCredential(context.Context, string, string) (servicedto.PricingCredential, error)
	})
	if !ok {
		t.Fatal("credential subjects cannot resolve observed upstream usage identities")
	}
	for _, test := range []struct{ authType, index, status, subject string }{
		{"apikey", "shared-index", "bound", subjects["apikey"]},
		{"oauth", "shared-index", "bound", subjects["oauth"]},
		{"", "shared-index", "ambiguous", ""},
		{"api_group_key", "shared-index", "unknown", ""},
		{"apikey", "downstream-group", "unknown", ""},
		{"apikey", " shared-index ", "unknown", ""},
	} {
		resolved, err := resolver.ResolvePricingCredential(ctx, test.authType, test.index)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.BindingStatus != test.status || resolved.SubjectID != test.subject {
			t.Fatalf("identity (%q,%q) resolved incorrectly: %+v", test.authType, test.index, resolved)
		}
	}
}

func TestPricingCredentialAdministratorCanSelectBindAndReadAfterRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "credential-restart.db")
	open := func() *gorm.DB {
		t.Helper()
		db, err := repository.OpenDatabase(config.Config{SQLitePath: dbPath})
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		return db
	}
	db := open()
	// The binding preserves the exact stored value, including case and whitespace;
	// it must not introduce another normalization on top of the existing directory.
	row := entities.UsageIdentity{Name: "Friendly upstream", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: " Synthetic-Index ", Type: "openai", LookupKey: "synthetic-secret", BindingIdentityStatus: "unique"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	router, cookie := credentialPricingRouter(t, db)
	directory := credentialDirectory(t, router, cookie)
	if len(directory) != 1 || directory[0].Name != "Friendly upstream" || directory[0].BindingStatus != "unbound" {
		t.Fatalf("unexpected directory: %+v", directory)
	}
	res := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, directory[0].DirectoryID), cookie)
	if res.Code != http.StatusCreated {
		t.Fatalf("bind status = %d, want 201: %s", res.Code, res.Body.String())
	}
	var saved safeCredential
	if err := json.Unmarshal(res.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(saved.SubjectID, "cred_") || saved.BindingStatus != "bound" {
		t.Fatalf("invalid saved subject: %+v", saved)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	db = open()
	router, cookie = credentialPricingRouter(t, db)
	refreshed := credentialDirectory(t, router, cookie)
	if len(refreshed) != 1 || refreshed[0].SubjectID != saved.SubjectID || refreshed[0].BindingStatus != "bound" {
		t.Fatalf("database reopen lost binding: %+v", refreshed)
	}
	var subject entities.CredentialPricingSubject
	if err := db.First(&subject, "id = ?", saved.SubjectID).Error; err != nil {
		t.Fatal(err)
	}
	if subject.AuthType != row.AuthType || subject.AuthTypeName != row.AuthTypeName || subject.Identity != row.Identity || subject.UsageIdentityID != row.ID {
		t.Fatalf("restart changed original identity relation: %+v", subject)
	}

	// Separately exercise a real directory refresh through the existing canonical
	// identity chain, which intentionally trims its incoming Identity before storage.
	canonical := entities.UsageIdentity{Name: "Refreshable upstream", AuthTypeName: "apikey", Identity: "canonical-index", Type: "openai"}
	for round := 0; round < 2; round++ {
		if err := repository.ReplaceUsageIdentitiesForProviderTypes(ctx, db, []entities.UsageIdentity{canonical}, []string{"openai"}, time.Now()); err != nil {
			t.Fatal(err)
		}
		items := credentialDirectory(t, router, cookie)
		for _, item := range items {
			if item.Status != "active" {
				continue
			}
			if round == 0 {
				res = serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, item.DirectoryID), cookie)
				if res.Code != http.StatusCreated {
					t.Fatalf("canonical bind: %d %s", res.Code, res.Body.String())
				}
				if err := json.Unmarshal(res.Body.Bytes(), &saved); err != nil {
					t.Fatal(err)
				}
			} else if item.SubjectID != saved.SubjectID || item.BindingStatus != "bound" {
				t.Fatalf("refresh changed binding: %+v", item)
			}
		}
		canonical.Name = "Renamed upstream"
		canonical.LookupKey = "changed-synthetic-secret"
	}
}

func TestPricingCredentialBindingDoesNotChangeLegacyCostsOrRequestAttributes(t *testing.T) {
	db := openAPITestDatabase(t)
	ctx := context.Background()
	catalog := emptyPricingCatalogForTest()
	provider := service.NewPricingService(db, catalog)
	multiplier := 0.5
	if _, err := provider.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "model-a", PromptPricePer1M: 2, CompletionPricePer1M: 4, CacheReadPricePer1M: 0.25, CacheWritePricePer1M: 3, PriceMultiplier: &multiplier}); err != nil {
		t.Fatal(err)
	}
	ruleMultiplier := 2.0
	if _, err := provider.ReplacePricingRules(ctx, servicedto.ReplacePricingRulesInput{Model: "model-a", Rules: []servicedto.PricingRuleInput{{Key: "auth_index", Value: "synthetic-index", Multiplier: &ruleMultiplier}}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.ReplaceUsageIdentitiesForProviderTypes(ctx, db, []entities.UsageIdentity{{Name: "Friendly upstream", AuthTypeName: "apikey", Identity: "synthetic-index", Type: "openai"}}, []string{"openai"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Hour)
	rows := []entities.UsageEvent{
		{EventKey: "priced-request", Model: "model-a", APIGroupKey: "downstream-a", AuthIndex: "synthetic-index", AuthType: "apikey", ServiceTier: "priority", ReasoningEffort: "high", Endpoint: "https://synthetic.example/v1", Timestamp: now.Add(-30 * time.Minute), InputTokens: 1_000_000, OutputTokens: 500_000, CacheReadTokens: 100_000, CacheCreationTokens: 50_000, TotalTokens: 1_550_000},
		{EventKey: "unknown-price-request", Model: "unpriced-model", APIGroupKey: "downstream-b", AuthIndex: "synthetic-index", AuthType: "apikey", Timestamp: now.Add(-20 * time.Minute), InputTokens: 100, TotalTokens: 100},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, db, now); err != nil {
		t.Fatal(err)
	}
	router, cookie := credentialPricingRouter(t, db, catalog)
	query := url.Values{"range": {"custom"}, "unit": {"hour"}, "start": {now.Add(-5 * time.Hour).Format(time.RFC3339)}, "end": {now.Format(time.RFC3339)}}.Encode()
	paths := []string{"/api/v1/usage/overview?" + query, "/api/v1/usage/overview/comparisons?" + query, "/api/v1/usage/events?" + query, "/api/v1/pricing", "/api/v1/pricing/rules?model=model-a"}
	legacyPayload := func(body string) any {
		t.Helper()
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatal(err)
		}
		var stripAdditivePricing func(any)
		stripAdditivePricing = func(value any) {
			switch item := value.(type) {
			case map[string]any:
				delete(item, "pricing_snapshot_id")
				delete(item, "pricing_selection")
				delete(item, "dual_costs")
				for _, child := range item {
					stripAdditivePricing(child)
				}
			case []any:
				for _, child := range item {
					stripAdditivePricing(child)
				}
			}
		}
		stripAdditivePricing(payload)
		return payload
	}
	assertPricingMetadata := func(path, body, snapshotID string) {
		t.Helper()
		if !strings.HasPrefix(path, "/api/v1/usage/") {
			return
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatal(err)
		}
		if snapshotID == "" || payload["pricing_snapshot_id"] != snapshotID {
			t.Fatalf("missing current root pricing snapshot on %s: %s", path, body)
		}
		var assertSnapshotIDs func(any)
		assertSnapshotIDs = func(value any) {
			switch item := value.(type) {
			case map[string]any:
				if id, exists := item["pricing_snapshot_id"]; exists && id != snapshotID {
					t.Fatalf("mixed pricing snapshots on %s: %s", path, body)
				}
				for _, child := range item {
					assertSnapshotIDs(child)
				}
			case []any:
				for _, child := range item {
					assertSnapshotIDs(child)
				}
			}
		}
		assertSnapshotIDs(payload)
		if !strings.HasPrefix(path, "/api/v1/usage/events?") {
			return
		}
		var events struct {
			Events []struct {
				Model      string                 `json:"model"`
				Cost       float64                `json:"cost_usd"`
				Available  bool                   `json:"cost_available"`
				SnapshotID string                 `json:"pricing_snapshot_id"`
				DualCosts  pricing.DualCosts      `json:"dual_costs"`
				Selection  *pricing.CostSelection `json:"pricing_selection"`
			}
		}
		if err := json.Unmarshal([]byte(body), &events); err != nil {
			t.Fatal(err)
		}
		for _, event := range events.Events {
			selection := event.Selection
			if event.SnapshotID != snapshotID || selection == nil || selection.SnapshotID != snapshotID || selection.Scope != "legacy" || selection.Mode != "legacy" || !selection.Legacy || selection.LegacyAdjustmentsReplaced || !reflect.DeepEqual(selection.DualCosts, event.DualCosts) {
				t.Fatalf("missing legacy snapshot/explanation contract: %+v", event)
			}
			configured, reference := event.DualCosts.Configured, event.DualCosts.Reference
			if event.Available {
				if configured.TotalCostUSD == nil || *configured.TotalCostUSD != event.Cost || !configured.Complete || !configured.HasKnown || configured.Status != "complete" || reference.TotalCostUSD == nil || *reference.TotalCostUSD != 3.875 || !reference.Complete || !reference.HasKnown || reference.Status != "complete" || !selection.BaselineAvailable || selection.BaselineCostUSD == nil || *selection.BaselineCostUSD != 3.875 || selection.LegacyModelMultiplier != .5 || selection.LegacyRuleMultiplier != 2 || selection.FinalMultiplier != 1 {
					t.Fatalf("priced legacy evidence changed: %+v", event)
				}
			} else if configured.TotalCostUSD != nil || configured.Complete || configured.HasKnown || configured.Status != "unavailable" || reference.TotalCostUSD != nil || reference.Complete || reference.HasKnown || reference.Status != "unavailable" || selection.BaselineAvailable || selection.BaselineCostUSD != nil {
				t.Fatalf("missing-price legacy evidence fabricated coverage: %+v", event)
			}
			explanation, err := json.Marshal(selection)
			if err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{"synthetic-index", "downstream-a", "downstream-b", "https://synthetic.example/v1"} {
				if strings.Contains(string(explanation), private) {
					t.Fatalf("legacy explanation leaked %s", private)
				}
			}
		}
	}
	beforeSnapshotID := catalog.NewResolver().SnapshotID()
	before := map[string]string{}
	for _, path := range paths {
		res := serveAPIGet(router, path, cookie)
		if res.Code != http.StatusOK {
			t.Fatalf("baseline %s: %d %s", path, res.Code, res.Body.String())
		}
		before[path] = res.Body.String()
		assertPricingMetadata(path, before[path], beforeSnapshotID)
	}
	if !strings.Contains(before[paths[0]], `"cost_available":false`) || !strings.Contains(before[paths[2]], `"cost_available":true`) || !strings.Contains(before[paths[2]], `"cost_available":false`) {
		t.Fatal("baseline must exercise both known and missing-price availability")
	}
	costSubject := pricing.NewCostSubject(pricing.UsageDimensions{Model: "model-a", AuthIndex: "synthetic-index"}, helper.UsageTokenCostInput{InputTokens: rows[0].InputTokens, OutputTokens: rows[0].OutputTokens, CacheReadTokens: rows[0].CacheReadTokens, CacheCreationTokens: rows[0].CacheCreationTokens})
	baselineCost := catalog.NewResolver().Calculate(costSubject)
	if !baselineCost.Available || baselineCost.Cost.TotalCostUSD <= 0 || baselineCost.RuleMultiplier != 2 {
		t.Fatalf("ineffective legacy regression fixture: %+v", baselineCost)
	}
	baselineSnapshot := catalog.Snapshot()
	var baselineRequests []entities.UsageEvent
	if err := db.Order("id").Find(&baselineRequests).Error; err != nil {
		t.Fatal(err)
	}
	item := credentialDirectory(t, router, cookie)[0]
	res := serveCredentialMutation(router, http.MethodPost, "/api/v1/pricing/credentials", fmt.Sprintf(`{"directory_id":%d}`, item.DirectoryID), cookie)
	if res.Code != http.StatusCreated {
		t.Fatalf("save: %d %s", res.Code, res.Body.String())
	}
	var saved safeCredential
	if err := json.Unmarshal(res.Body.Bytes(), &saved); err != nil || saved.SubjectID == "" {
		t.Fatalf("missing saved credential subject: %+v, %v", saved, err)
	}
	afterSnapshotID := catalog.NewResolver().SnapshotID()
	if afterSnapshotID == beforeSnapshotID {
		t.Fatal("binding did not publish a new pricing snapshot")
	}
	for _, path := range paths {
		res = serveAPIGet(router, path, cookie)
		if res.Code != http.StatusOK || !reflect.DeepEqual(legacyPayload(res.Body.String()), legacyPayload(before[path])) {
			t.Fatalf("binding changed legacy response %s: %d %s", path, res.Code, res.Body.String())
		}
		assertPricingMetadata(path, res.Body.String(), afterSnapshotID)
	}
	// Ticket 3 publishes bindings in the same immutable candidate. The snapshot
	// pointer must change, but legacy model prices, fees and match metadata must not.
	currentCost := catalog.NewResolver().Calculate(costSubject)
	if baselineCost.CredentialSubjectID != "" || currentCost.CredentialSubjectID != saved.SubjectID {
		t.Fatalf("binding did not add the exact safe credential subject: before=%+v after=%+v", baselineCost, currentCost)
	}
	// Legacy calculations now carry bound-subject evidence even without overrides.
	// Compare every other field, including all original cost and match metadata.
	legacyCurrentCost := currentCost
	legacyCurrentCost.CredentialSubjectID = baselineCost.CredentialSubjectID
	if !reflect.DeepEqual(catalog.Snapshot().ModelConfigs(), baselineSnapshot.ModelConfigs()) || !reflect.DeepEqual(legacyCurrentCost, baselineCost) {
		t.Fatalf("binding changed published prices, cost or matching metadata: before=%+v after=%+v", baselineCost, currentCost)
	}
	var persisted []entities.UsageEvent
	if err := db.Order("id").Find(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, baselineRequests) {
		t.Fatal("binding rewrote existing request attributes")
	}
	bound := credentialDirectory(t, router, cookie)[0]
	if err := db.Model(&entities.UsageEvent{}).Where("id = ?", rows[0].ID).Update("api_group_key", "different-downstream-group").Error; err != nil {
		t.Fatal(err)
	}
	resolved, err := provider.(service.PricingCredentialProvider).ResolvePricingCredential(ctx, rows[0].AuthType, rows[0].AuthIndex)
	if err != nil || resolved.SubjectID != bound.SubjectID || resolved.BindingStatus != "bound" {
		t.Fatalf("downstream group changed upstream binding: %+v, %v", resolved, err)
	}
}

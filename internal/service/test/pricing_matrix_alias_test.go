package test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/providerconfig"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
)

func TestMatrixGenuineLocalAliasSurvivesMetadataSyncWithoutChangingSubject(t *testing.T) {
	db := openMetadataTestDatabase(t, "matrix-local-alias.db")
	ctx := context.Background()
	if err := repository.ActivateStrictPricing(db); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	fetcher := newMetadataTestFetcher()
	fetcher.standardResults["codex"] = &response.ProviderKeyConfigResult{StatusCode: 200, Payload: []providerconfig.ProviderKeyConfig{{APIKey: "synthetic-key-never-public", AuthIndex: "synthetic-stable-index", Name: "CPA Display Name"}}}
	syncer := newMetadataTestSyncer(db, fetcher, func() time.Time { return now })
	if err := syncer.SyncMetadata(ctx); err != nil {
		t.Fatal(err)
	}
	provider, catalog := newCatalogPricingService(t, db)
	directory := provider.(service.PricingCredentialProvider)
	credentials, err := directory.ListPricingCredentials(ctx)
	if err != nil || len(credentials) != 1 {
		t.Fatalf("metadata directory: %+v %v", credentials, err)
	}
	if _, err := directory.BindPricingCredential(ctx, credentials[0].DirectoryID); err != nil {
		t.Fatal(err)
	}
	matrix := provider.(service.CredentialModelMatrixProvider)
	page, err := matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
	if err != nil || len(page.Credentials) != 1 {
		t.Fatalf("discovery: %+v %v", page, err)
	}
	first := page.Credentials[0]
	if first.Alias != "" || first.Name != "CPA Display Name" || first.SubjectID == "" {
		t.Fatalf("display name must not be invented alias: %+v", first)
	}
	if err := repository.UpdateUsageIdentityAlias(ctx, db, first.DirectoryID, "Real Local Alias"); err != nil {
		t.Fatal(err)
	}
	fetcher.standardResults["codex"].Payload[0].Name = "Changed CPA Display Name"
	now = now.Add(time.Hour)
	if err := syncer.SyncMetadata(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.LoadPricingSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	catalog.Replace(snapshot)
	page, err = matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
	if err != nil || len(page.Credentials) != 1 {
		t.Fatalf("synced matrix: %+v %v", page, err)
	}
	actual := page.Credentials[0]
	if actual.Alias != "Real Local Alias" || actual.SubjectID != first.SubjectID || actual.DirectoryID != first.DirectoryID || actual.Name != "Changed CPA Display Name" {
		t.Fatalf("sync lost local alias or stable identity: %+v", actual)
	}
	for _, alias := range []string{"synthetic-key-never-public", "/private/credential.json", "Bearer synthetic-token-value"} {
		if err := db.Model(&entities.UsageIdentity{}).Where("id = ?", first.DirectoryID).Update("alias", alias).Error; err != nil {
			t.Fatal(err)
		}
		page, err = matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), alias) {
			t.Fatalf("unsafe alias leaked: %q", alias)
		}
	}
}

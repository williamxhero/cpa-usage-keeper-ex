package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/providerconfig"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/service"
)

func TestPricingCredentialDirectoryRejectsSharedUpstreamConfiguration(t *testing.T) {
	db := openMetadataTestDatabase(t, "shared-identity.db")
	fetcher := newMetadataTestFetcher()
	config := providerconfig.OpenAICompatibilityConfig{Name: "First configuration", BaseURL: "https://synthetic.example/v1", APIKeyEntries: []providerconfig.OpenAIApiKeyEntry{{APIKey: "synthetic-secret", AuthIndex: "shared-index"}}}
	second := config
	second.Name = "Second configuration"
	fetcher.openAIResult = &response.OpenAICompatibilityResult{StatusCode: 200, Payload: []providerconfig.OpenAICompatibilityConfig{config, second}}
	if err := newMetadataTestSyncer(db, fetcher, time.Now).SyncMetadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider := service.NewPricingService(db, pricing.NewCatalog(pricing.EmptySnapshot()))
	directory, err := provider.(service.PricingCredentialProvider).ListPricingCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(directory) != 1 || directory[0].BindingStatus != "ambiguous" || directory[0].SubjectID != "" {
		t.Fatalf("first-item dedup falsely claims a unique configuration: %+v", directory)
	}
	// The HTTP registration route is retired; retained fixture/migration binding
	// must still reject a directory row that is not uniquely attributable.
	_, err = provider.(service.PricingCredentialProvider).BindPricingCredential(context.Background(), directory[0].DirectoryID)
	if !errors.Is(err, service.ErrCredentialNotSelectable) {
		t.Fatalf("ambiguous directory binding error = %v", err)
	}
}

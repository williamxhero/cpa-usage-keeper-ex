package test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/pricingmetadata"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
)

func TestOfficialPricingRefreshUsesOpenRouterOnlyForUnpersistedModels(t *testing.T) {
	db := openUsageServiceTestDatabase(t)
	prompt := 9.0
	completion := 10.0
	if _, err := repository.UpsertOfficialModelPrice(db, entities.OfficialModelPrice{
		CanonicalModelID: "protected-model", Source: entities.OfficialModelPriceSourceOfficial,
		PromptPricePer1M: &prompt, CompletionPricePer1M: &completion,
	}); err != nil {
		t.Fatalf("seed protected official price: %v", err)
	}
	metadataClient := pricingmetadata.NewClient(&http.Client{Transport: officialRefreshTransport{
		primary:  `{"official":{"id":"official","name":"Official","models":{"official-model":{"id":"official-model","name":"Official model","last_updated":"2026-10-01","cost":{"input":1,"output":2}}}}}`,
		fallback: `{"data":[{"id":"fallback-model","name":"Fallback model","pricing":{"prompt":"0.000003","completion":"0.000004"}},{"id":"protected-model","name":"Should not replace","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`,
	}})
	provider := service.NewPricingServiceWithMetadataClient(db, pricing.NewCatalog(pricing.EmptySnapshot()), metadataClient)
	officialProvider := provider.(service.OfficialPricingProvider)
	preview, err := officialProvider.PreviewOfficialPricingRefresh(context.Background(), "models-dev")
	if err != nil {
		t.Fatalf("preview official pricing: %v", err)
	}
	if len(preview.Prices) != 2 {
		t.Fatalf("expected primary plus one fallback model, got %+v", preview.Prices)
	}
	var fallback *entities.OfficialModelPrice
	for index := range preview.Prices {
		if preview.Prices[index].CanonicalModelID == "fallback-model" {
			fallback = &preview.Prices[index]
		}
		if preview.Prices[index].CanonicalModelID == "protected-model" {
			t.Fatalf("persisted official model was reintroduced by fallback: %+v", preview.Prices)
		}
		if preview.Prices[index].CanonicalModelID == "official-model" && preview.Prices[index].SourceVersion != "2026-10-01" {
			t.Fatalf("official source version was not retained: %+v", preview.Prices[index])
		}
	}
	if fallback == nil || fallback.Source != entities.OfficialModelPriceSourceOpenRouter || fallback.PromptPricePer1M == nil || *fallback.PromptPricePer1M != 3 || fallback.CompletionPricePer1M == nil || *fallback.CompletionPricePer1M != 4 || fallback.MissingSegments != "cache_read,cache_write" {
		t.Fatalf("unexpected OpenRouter fallback conversion or missing segments: %+v", fallback)
	}
	if _, err := officialProvider.CommitOfficialPricingRefresh(context.Background(), preview.PreviewID); err != nil {
		t.Fatalf("commit official pricing: %v", err)
	}
}

func TestOfficialPricingRefreshFailurePreservesPricesAndMarksStale(t *testing.T) {
	db := openUsageServiceTestDatabase(t)
	prompt := 1.0
	completion := 2.0
	if _, err := repository.UpsertOfficialModelPrice(db, entities.OfficialModelPrice{
		CanonicalModelID: "stable-model", Source: entities.OfficialModelPriceSourceOfficial,
		PromptPricePer1M: &prompt, CompletionPricePer1M: &completion,
	}); err != nil {
		t.Fatalf("seed official price: %v", err)
	}
	metadataClient := pricingmetadata.NewClient(&http.Client{Transport: officialRefreshTransport{status: http.StatusBadGateway}})
	provider := service.NewPricingServiceWithMetadataClient(db, pricing.NewCatalog(pricing.EmptySnapshot()), metadataClient)
	officialProvider := provider.(service.OfficialPricingProvider)
	if _, err := officialProvider.PreviewOfficialPricingRefresh(context.Background(), "openrouter"); err == nil {
		t.Fatal("expected upstream refresh failure")
	}
	prices, err := repository.ListOfficialModelPrices(db)
	if err != nil {
		t.Fatalf("list preserved prices: %v", err)
	}
	if len(prices) != 1 || !prices[0].Stale || prices[0].PromptPricePer1M == nil || *prices[0].PromptPricePer1M != 1 || prices[0].CompletionPricePer1M == nil || *prices[0].CompletionPricePer1M != 2 {
		t.Fatalf("failed refresh did not preserve and stale-mark price: %+v", prices)
	}
}

type officialRefreshTransport struct {
	primary  string
	fallback string
	status   int
}

func (transport officialRefreshTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.status != 0 {
		return &http.Response{StatusCode: transport.status, Body: io.NopCloser(strings.NewReader("upstream failure")), Header: make(http.Header)}, nil
	}
	body := transport.fallback
	if request.URL.String() == "https://models.dev/api.json" {
		body = transport.primary
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}

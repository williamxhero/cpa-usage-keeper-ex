package test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/pricingmetadata"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
	"gorm.io/gorm"
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

func TestOfficialPricingCommitRechecksOpenRouterProtection(t *testing.T) {
	db := openUsageServiceTestDatabase(t)
	catalog := pricing.NewCatalog(pricing.EmptySnapshot())
	client := pricingmetadata.NewClient(&http.Client{Transport: officialRefreshTransport{
		fallback: `{"data":[{"id":"new-model","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`,
	}})
	provider := service.NewPricingServiceWithMetadataClient(db, catalog, client).(service.OfficialPricingProvider)
	preview, err := provider.PreviewOfficialPricingRefresh(context.Background(), "openrouter")
	if err != nil || len(preview.Prices) != 1 {
		t.Fatalf("preview fallback: %+v, err=%v", preview, err)
	}
	prompt := 9.0
	if _, err := provider.UpdateOfficialPricing(context.Background(), service.OfficialPricingInput{CanonicalModelID: "new-model", PromptPricePer1M: &prompt}); err != nil {
		t.Fatalf("save official price after preview: %v", err)
	}
	if _, err := provider.CommitOfficialPricingRefresh(context.Background(), preview.PreviewID); err != nil {
		t.Fatalf("commit fallback preview: %v", err)
	}
	rows, err := provider.ListOfficialPricing(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Source != entities.OfficialModelPriceSourceOfficial || rows[0].PromptPricePer1M == nil || *rows[0].PromptPricePer1M != 9 {
		t.Fatalf("fallback overwrote official price created after preview: %+v, err=%v", rows, err)
	}
	published := catalog.Snapshot().OfficialPrices()
	if len(published) != 1 || published[0].Source != entities.OfficialModelPriceSourceOfficial || *published[0].PromptPricePer1M != 9 {
		t.Fatalf("published snapshot lost protected official price: %+v", published)
	}
}

func TestOfficialPricingRefreshDoesNotFillAmbiguousPrimaryFromFallback(t *testing.T) {
	db := openUsageServiceTestDatabase(t)
	client := pricingmetadata.NewClient(&http.Client{Transport: officialRefreshTransport{
		primary:  `{"primary":{"models":{"one":{"id":"model","cost":{"input":1,"output":2}},"two":{"id":"MODEL","cost":{"input":3,"output":4}}}}}`,
		fallback: `{"data":[{"id":"model","pricing":{"prompt":"0.000005","completion":"0.000006"}}]}`,
	}})
	provider := service.NewPricingServiceWithMetadataClient(db, pricing.NewCatalog(pricing.EmptySnapshot()), client).(service.OfficialPricingProvider)
	preview, err := provider.PreviewOfficialPricingRefresh(context.Background(), "models-dev")
	if err != nil {
		t.Fatalf("preview ambiguous primary: %v", err)
	}
	if len(preview.Prices) != 0 || len(preview.AmbiguousModels) != 1 || preview.AmbiguousModels[0] != "model" {
		t.Fatalf("fallback reintroduced ambiguous primary identity: %+v", preview)
	}
	if _, err := provider.CommitOfficialPricingRefresh(context.Background(), preview.PreviewID); err != nil {
		t.Fatalf("commit empty ambiguous preview: %v", err)
	}
	rows, err := provider.ListOfficialPricing(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatalf("ambiguous model became a persisted price: %+v, err=%v", rows, err)
	}
}

func TestOfficialPricingPartialPrimaryRefreshPreservesKnownSnapshotAndMarksStale(t *testing.T) {
	db := openUsageServiceTestDatabase(t)
	prompt, completion, cacheRead, cacheWrite := 3.0, 15.0, 0.3, 3.75
	if _, err := repository.UpsertOfficialModelPrice(db, entities.OfficialModelPrice{
		CanonicalModelID: "stable-model", Source: entities.OfficialModelPriceSourceOfficial,
		PromptPricePer1M: &prompt, CompletionPricePer1M: &completion, CacheReadPricePer1M: &cacheRead, CacheWritePricePer1M: &cacheWrite,
	}); err != nil {
		t.Fatalf("seed official price: %v", err)
	}
	client := pricingmetadata.NewClient(&http.Client{Transport: officialRefreshTransport{
		primary:  `{"primary":{"models":{"stable":{"id":"stable-model","cost":{"input":1,"output":2}}}}}`,
		fallback: `{"data":[]}`,
	}})
	provider := service.NewPricingServiceWithMetadataClient(db, pricing.NewCatalog(pricing.EmptySnapshot()), client).(service.OfficialPricingProvider)
	if _, err := provider.PreviewOfficialPricingRefresh(context.Background(), "models-dev"); err == nil {
		t.Fatal("expected partial primary refresh to be rejected")
	}
	rows, err := provider.ListOfficialPricing(context.Background())
	if err != nil || len(rows) != 1 || !rows[0].Stale || rows[0].PromptPricePer1M == nil || *rows[0].PromptPricePer1M != 3 || rows[0].CacheReadPricePer1M == nil || *rows[0].CacheReadPricePer1M != 0.3 || rows[0].CacheWritePricePer1M == nil || *rows[0].CacheWritePricePer1M != 3.75 {
		t.Fatalf("partial refresh erased known snapshot values: %+v, err=%v", rows, err)
	}
}

func TestOfficialPricingRefreshDeadlineStillMarksSnapshotStale(t *testing.T) {
	db := openUsageServiceTestDatabase(t)
	catalog := pricing.NewCatalog(pricing.EmptySnapshot())
	client := pricingmetadata.NewClient(&http.Client{Transport: officialDeadlineTransport{}})
	provider := service.NewPricingServiceWithMetadataClient(db, catalog, client).(service.OfficialPricingProvider)
	prompt := 3.0
	if _, err := provider.UpdateOfficialPricing(context.Background(), service.OfficialPricingInput{CanonicalModelID: "stable-model", PromptPricePer1M: &prompt}); err != nil {
		t.Fatalf("seed official price: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := provider.PreviewOfficialPricingRefresh(ctx, "openrouter"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected fetch deadline error, got %v", err)
	}
	rows, err := provider.ListOfficialPricing(context.Background())
	if err != nil || len(rows) != 1 || !rows[0].Stale || rows[0].PromptPricePer1M == nil || *rows[0].PromptPricePer1M != 3 {
		t.Fatalf("deadline prevented persisted stale marking: %+v, err=%v", rows, err)
	}
	published := catalog.Snapshot().OfficialPrices()
	if len(published) != 1 || !published[0].Stale || published[0].PromptPricePer1M == nil || *published[0].PromptPricePer1M != 3 {
		t.Fatalf("deadline prevented snapshot stale marking: %+v", published)
	}
}

func TestOfficialPricingRefreshRejectsOversizedPreview(t *testing.T) {
	db := openUsageServiceTestDatabase(t)
	models := make([]string, 5001)
	for i := range models {
		models[i] = fmt.Sprintf(`{"id":"model-%d","pricing":{"prompt":"0.000001","completion":"0.000002"}}`, i)
	}
	client := pricingmetadata.NewClient(&http.Client{Transport: officialRefreshTransport{fallback: `{"data":[` + strings.Join(models, ",") + `]}`}})
	catalog := pricing.NewCatalog(pricing.EmptySnapshot())
	provider := service.NewPricingServiceWithMetadataClient(db, catalog, client).(service.OfficialPricingProvider)
	preview, err := provider.PreviewOfficialPricingRefresh(context.Background(), "openrouter")
	if !errors.Is(err, service.ErrOfficialPricingBatchTooLarge) || preview.PreviewID != "" {
		t.Fatalf("oversized refresh issued an uncommittable preview: id=%q, err=%v", preview.PreviewID, err)
	}
	client = pricingmetadata.NewClient(&http.Client{Transport: officialRefreshTransport{fallback: `{"data":[` + strings.Join(models[:5000], ",") + `]}`}})
	provider = service.NewPricingServiceWithMetadataClient(db, catalog, client).(service.OfficialPricingProvider)
	preview, err = provider.PreviewOfficialPricingRefresh(context.Background(), "openrouter")
	if err != nil || preview.PreviewID == "" || len(preview.Prices) != 5000 {
		t.Fatalf("maximum-sized preview was rejected: count=%d, err=%v", len(preview.Prices), err)
	}
	rows, err := provider.CommitOfficialPricingRefresh(context.Background(), preview.PreviewID)
	if err != nil || len(rows) != 5000 {
		t.Fatalf("maximum-sized preview could not commit atomically: rows=%d, err=%v", len(rows), err)
	}
	persisted, err := provider.ListOfficialPricing(context.Background())
	published := catalog.Snapshot().OfficialPrices()
	if err != nil || len(persisted) != 5000 || len(published) != 5000 {
		t.Fatalf("maximum-sized commit did not persist and publish all rows: persisted=%d published=%d err=%v", len(persisted), len(published), err)
	}
}

func TestOfficialPricingCommitSnapshotFailureRollsBackAndCanRetry(t *testing.T) {
	db := openUsageServiceTestDatabase(t)
	catalog := pricing.NewCatalog(pricing.EmptySnapshot())
	client := pricingmetadata.NewClient(&http.Client{Transport: officialRefreshTransport{
		fallback: `{"data":[{"id":"new-model","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`,
	}})
	provider := service.NewPricingServiceWithMetadataClient(db, catalog, client).(service.OfficialPricingProvider)
	prompt := 3.0
	if _, err := provider.UpdateOfficialPricing(context.Background(), service.OfficialPricingInput{CanonicalModelID: "stable-model", PromptPricePer1M: &prompt}); err != nil {
		t.Fatalf("seed official price: %v", err)
	}
	preview, err := provider.PreviewOfficialPricingRefresh(context.Background(), "openrouter")
	if err != nil {
		t.Fatalf("preview refresh: %v", err)
	}
	previous := catalog.Snapshot()
	const callback = "test:official-refresh-snapshot-failure"
	failure := errors.New("synthetic snapshot load failure")
	if err := db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "model_price_rules" {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatalf("register snapshot failure: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })
	if _, err := provider.CommitOfficialPricingRefresh(context.Background(), preview.PreviewID); !errors.Is(err, failure) {
		t.Fatalf("expected snapshot load failure, got %v", err)
	}
	rows, err := provider.ListOfficialPricing(context.Background())
	if err != nil || len(rows) != 1 || rows[0].CanonicalModelID != "stable-model" || *rows[0].PromptPricePer1M != 3 || catalog.Snapshot() != previous {
		t.Fatalf("failed commit changed database or published snapshot: %+v, err=%v", rows, err)
	}
	if err := db.Callback().Query().Remove(callback); err != nil {
		t.Fatalf("remove snapshot failure: %v", err)
	}
	if _, err := provider.CommitOfficialPricingRefresh(context.Background(), preview.PreviewID); err != nil {
		t.Fatalf("retry the same preview after failure: %v", err)
	}
	if len(catalog.Snapshot().OfficialPrices()) != 2 {
		t.Fatalf("retry did not publish complete catalog: %+v", catalog.Snapshot().OfficialPrices())
	}
}

type officialDeadlineTransport struct{}

func (officialDeadlineTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
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

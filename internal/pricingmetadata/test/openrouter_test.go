package test

import (
	"context"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"cpa-usage-keeper/internal/pricingmetadata"
)

func TestOpenRouterPricingMetadataConvertsTokenPricesAndPreservesMissingCacheWrite(t *testing.T) {
	body := `{"data":[{"id":"anthropic/claude-sonnet","name":"Claude Sonnet","pricing":{"prompt":"0.000003","completion":"0.000015","input_cache_read":"0.0000003"}},{"id":"free/model","pricing":{"prompt":"0","completion":"0"}}]}`
	client := pricingmetadata.NewClient(&http.Client{Transport: catalogTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://openrouter.ai/api/v1/models" {
			t.Fatalf("unexpected URL: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	catalog, err := client.Fetch(context.Background(), pricingmetadata.SourceOpenRouter)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Source.ID != pricingmetadata.SourceOpenRouter || catalog.FetchedAt.IsZero() || len(catalog.ResponseHash) != 64 {
		t.Fatalf("missing safe source metadata: %#v", catalog)
	}
	if _, err := time.Parse(time.RFC3339Nano, catalog.FetchedAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("invalid timestamp: %v", err)
	}
	if len(catalog.Entries) != 2 {
		t.Fatalf("unexpected entries: %#v", catalog.Entries)
	}
	model := catalog.Entries[0].Model
	for name, gotWant := range map[string][2]float64{
		"prompt": {*model.Cost.Input, 3}, "completion": {*model.Cost.Output, 15}, "cache read": {*model.Cost.CacheRead, 0.3},
	} {
		if math.Abs(gotWant[0]-gotWant[1]) > 1e-10 {
			t.Errorf("%s = %v, want %v", name, gotWant[0], gotWant[1])
		}
	}
	if model.Cost.CacheWrite != nil {
		t.Fatalf("missing cache-write price became present: %#v", model.Cost.CacheWrite)
	}
	free := catalog.Entries[1].Model
	if free.Cost.Input == nil || *free.Cost.Input != 0 || free.Cost.Output == nil || *free.Cost.Output != 0 {
		t.Fatalf("explicit zero prices were lost: %#v", free.Cost)
	}
}

func TestPricingCatalogRejectsTrailingJSON(t *testing.T) {
	body := `{"data":[]} {"unexpected":true}`
	client := pricingmetadata.NewClient(&http.Client{Transport: catalogTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if _, err := client.Fetch(context.Background(), pricingmetadata.SourceOpenRouter); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing JSON error = %v", err)
	}
}

func TestPricingCatalogRejectsOversizedResponses(t *testing.T) {
	client := pricingmetadata.NewClient(&http.Client{Transport: catalogTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", 64<<20+1)))}, nil
	})})
	if _, err := client.Fetch(context.Background(), pricingmetadata.SourceOpenRouter); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error = %v", err)
	}
}

package test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
)

func windowDualResolver(t *testing.T, multiplier, prompt float64) pricing.Resolver {
	t.Helper()
	snapshot, err := pricing.CompileSnapshot([]pricing.ModelConfig{{Pricing: entities.ModelPriceSetting{Model: "dual-model", PromptPricePer1M: prompt, PriceMultiplier: &multiplier}}})
	if err != nil {
		t.Fatal(err)
	}
	return pricing.NewCatalog(snapshot).NewResolver()
}

func assertWindowEstimate(t *testing.T, estimate pricing.PriceEstimate, status string, total *float64) {
	t.Helper()
	if estimate.Status != status || estimate.Complete != (status == "complete") || estimate.HasKnown != (total != nil) {
		t.Fatalf("unexpected estimate state: %+v", estimate)
	}
	if total == nil {
		if estimate.TotalCostUSD != nil {
			t.Fatalf("unknown estimate must have null total: %+v", estimate)
		}
	} else if estimate.TotalCostUSD == nil || math.Abs(*estimate.TotalCostUSD-*total) > 1e-9 {
		t.Fatalf("expected total %g, got %+v", *total, estimate)
	}
}

func TestUsageWindowDualCostsPreserveRawAndLongLegacyNormalization(t *testing.T) {
	for _, test := range []struct {
		name  string
		long  bool
		model string
	}{
		{name: "raw", model: "dual-model"},
		{name: "long merged boundaries", long: true, model: "dual-model"},
		{name: "long trims grouped model", long: true, model: " dual-model "},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openTestDatabase(t)
			start := time.Date(2026, 9, 1, 10, 30, 0, 0, time.UTC)
			end := start.Add(time.Hour)
			if test.long {
				end = start.Add(8 * time.Hour)
			}
			events := []entities.UsageEvent{
				{EventKey: "dual-positive", AuthIndex: "dual-auth", Model: test.model, Timestamp: start.Add(time.Minute), InputTokens: 9_000_000, TotalTokens: 9_000_000},
				{EventKey: "dual-negative", AuthIndex: "dual-auth", Model: test.model, Timestamp: end.Add(-time.Minute), InputTokens: -300_000, TotalTokens: 300_000},
			}
			if err := db.Create(&events).Error; err != nil {
				t.Fatal(err)
			}
			resolver := windowDualResolver(t, 1, 10)
			calculator, err := repository.NewUsageWindowStatsCalculator(context.Background(), db, resolver)
			if err != nil {
				t.Fatal(err)
			}
			stats, err := calculator.SumByAuthIndex(context.Background(), "dual-auth", start, &end)
			if err != nil {
				t.Fatal(err)
			}
			if stats.Cost != 87 || !stats.CostAvailable || stats.Tokens != 9_300_000 || stats.PricingSnapshotID != resolver.SnapshotID() {
				t.Fatalf("legacy grouping changed: %+v", stats)
			}
			configured, reference := 87.0, 90.0
			assertWindowEstimate(t, stats.DualCosts.Configured, "complete", &configured)
			assertWindowEstimate(t, stats.DualCosts.Reference, "complete", &reference)
			grouped, err := calculator.SumGroupsByAuthIndex(context.Background(), "dual-auth", start, &end, func(model string) (string, bool) { return "group", model == "dual-model" })
			if err != nil {
				t.Fatal(err)
			}
			if !grouped.Complete || len(grouped.Groups) != 1 || grouped.Groups["group"].Cost != 87 {
				t.Fatalf("reference-only rows changed legacy quota grouping: %+v", grouped)
			}
			assertWindowEstimate(t, grouped.Groups["group"].DualCosts.Reference, "complete", &reference)
		})
	}
}

func TestUsageWindowDualCostsKnownZeroUnknownAndPartial(t *testing.T) {
	for _, test := range []struct {
		name   string
		models []string
		status string
		total  *float64
	}{
		{name: "empty known zero", status: "complete", total: float64ForWindowDual(0)},
		{name: "priced zero", models: []string{"dual-model"}, status: "complete", total: float64ForWindowDual(0)},
		{name: "fully unknown", models: []string{"missing"}, status: "unavailable"},
		{name: "partial zero subtotal", models: []string{"dual-model", "missing"}, status: "partial", total: float64ForWindowDual(0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openTestDatabase(t)
			start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
			end := start.Add(time.Hour)
			for index, model := range test.models {
				if err := db.Create(&entities.UsageEvent{EventKey: model, Model: model, AuthIndex: "dual-auth", Timestamp: start.Add(time.Duration(index+1) * time.Minute), InputTokens: 1_000_000, TotalTokens: 1_000_000}).Error; err != nil {
					t.Fatal(err)
				}
			}
			stats, err := repository.SumUsageWindowStatsByAuthIndex(context.Background(), db, "dual-auth", start, &end, windowDualResolver(t, 1, 0))
			if err != nil {
				t.Fatal(err)
			}
			assertWindowEstimate(t, stats.DualCosts.Configured, test.status, test.total)
			assertWindowEstimate(t, stats.DualCosts.Reference, test.status, test.total)
		})
	}
}

func TestUsageWindowDualCostsHourlyReferenceUsesCommittedArchiveEvidence(t *testing.T) {
	for _, mode := range []string{"deduplicated", "partial archive", "pruned with uncommitted impostor"} {
		t.Run(mode, func(t *testing.T) {
			db := openTestDatabase(t)
			start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
			end := start.Add(8 * time.Hour)
			hour := start.Add(2 * time.Hour)
			events := []entities.UsageEvent{
				{EventKey: "hourly-positive", Model: "dual-model", AuthIndex: "dual-auth", Timestamp: hour.Add(time.Minute), InputTokens: 1_000_000, TotalTokens: 1_000_000},
				{EventKey: "hourly-negative", Model: "dual-model", AuthIndex: "dual-auth", Timestamp: hour.Add(2 * time.Minute), InputTokens: -1_000_000, TotalTokens: 1_000_000},
			}
			if err := db.Create(&events).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&entities.UsageOverviewHourlyStat{BucketStart: hour, Model: "dual-model", AuthIndex: "dual-auth", RequestCount: 2, TotalTokens: 2_000_000, CreatedAt: end, UpdatedAt: end}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&entities.UsageAggregationCheckpoint{Name: entities.UsageAggregationCheckpointOverview, LastAggregatedUsageEventID: events[1].ID, CreatedAt: end, UpdatedAt: end}).Error; err != nil {
				t.Fatal(err)
			}
			if mode == "deduplicated" || mode == "partial archive" {
				archive := entities.UsageEventArchive{ID: events[0].ID, EventKey: events[0].EventKey, Model: events[0].Model, AuthIndex: events[0].AuthIndex, Timestamp: events[0].Timestamp, InputTokens: events[0].InputTokens, TotalTokens: events[0].TotalTokens}
				if err := db.Create(&archive).Error; err != nil {
					t.Fatal(err)
				}
			}
			if mode != "deduplicated" {
				if err := db.Delete(&events).Error; err != nil {
					t.Fatal(err)
				}
			}
			if mode == "pruned with uncommitted impostor" {
				for index := range events {
					events[index].ID = 0
					events[index].EventKey += "-new"
				}
				if err := db.Create(&events).Error; err != nil {
					t.Fatal(err)
				}
			}
			resolver := windowDualResolver(t, 2, 10)
			stats, err := repository.SumUsageWindowStatsByAuthIndex(context.Background(), db, "dual-auth", start, &end, resolver)
			if err != nil {
				t.Fatal(err)
			}
			if stats.Cost != 0 || !stats.CostAvailable || stats.Tokens != 2_000_000 {
				t.Fatalf("legacy hourly cost changed: %+v", stats)
			}
			zero, ten := 0.0, 10.0
			assertWindowEstimate(t, stats.DualCosts.Configured, "complete", &zero)
			switch mode {
			case "deduplicated":
				assertWindowEstimate(t, stats.DualCosts.Reference, "complete", &ten)
			case "partial archive":
				assertWindowEstimate(t, stats.DualCosts.Reference, "partial", &ten)
			default:
				assertWindowEstimate(t, stats.DualCosts.Reference, "unavailable", nil)
			}
		})
	}
}

func TestUsageWindowDualCostsReferenceOverflowPreservesLegacyZero(t *testing.T) {
	db := openTestDatabase(t)
	start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	if err := db.Create(&entities.UsageEvent{EventKey: "overflow", Model: "dual-model", AuthIndex: "dual-auth", Timestamp: start, InputTokens: 2_000_000, TotalTokens: 2_000_000}).Error; err != nil {
		t.Fatal(err)
	}
	stats, err := repository.SumUsageWindowStatsByAuthIndex(context.Background(), db, "dual-auth", start, &end, windowDualResolver(t, 0, math.MaxFloat64))
	if err != nil {
		t.Fatal(err)
	}
	if !stats.CostAvailable || stats.Cost != 0 {
		t.Fatalf("legacy zero compatibility changed: %+v", stats)
	}
	zero := 0.0
	assertWindowEstimate(t, stats.DualCosts.Configured, "complete", &zero)
	assertWindowEstimate(t, stats.DualCosts.Reference, "unavailable", nil)
	if _, err := json.Marshal(stats.DualCosts); err != nil {
		t.Fatalf("reference overflow leaked non-finite JSON: %v", err)
	}
}

func TestUsageWindowDualCostsFixedWithoutBaselinePreservesOtherLegacyGroups(t *testing.T) {
	db := openTestDatabase(t)
	start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	zero, five, two := 0.0, 5.0, 2.0
	snapshot, err := pricing.CompileSnapshotWithCredentials(
		[]pricing.ModelConfig{{Pricing: entities.ModelPriceSetting{Model: "dual-model", PromptPricePer1M: 10, PriceMultiplier: &two}}},
		[]pricing.CredentialBinding{{SubjectID: "fixed-subject", AuthType: "oauth", AuthIndex: "dual-auth", Status: "unique"}}, nil,
		pricing.OverrideConfig{CredentialModels: []pricing.CredentialModelConfig{{SubjectID: "fixed-subject", Model: "missing", Mode: pricing.ModeFixed, Fixed: &pricing.FixedTariff{PricingStyle: entities.ModelPricingStyleOpenAI, PromptPricePer1M: &five, CompletionPricePer1M: &zero, CacheReadPricePer1M: &zero, CacheWritePricePer1M: &zero}}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	events := []entities.UsageEvent{
		{EventKey: "baseline-positive", AuthType: "oauth", AuthIndex: "dual-auth", Model: "dual-model", Timestamp: start.Add(time.Minute), InputTokens: 1_000_000, TotalTokens: 1_000_000},
		{EventKey: "baseline-negative", AuthType: "oauth", AuthIndex: "dual-auth", Model: "dual-model", Timestamp: start.Add(2 * time.Minute), InputTokens: -1_000_000, TotalTokens: 1_000_000},
		{EventKey: "fixed-missing", AuthType: "oauth", AuthIndex: "dual-auth", Model: "missing", Timestamp: start.Add(3 * time.Minute), InputTokens: 1_000_000, TotalTokens: 1_000_000},
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatal(err)
	}
	stats, err := repository.SumUsageWindowStatsByAuthIndex(context.Background(), db, "dual-auth", start, &end, pricing.NewCatalog(snapshot).NewResolver())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Cost != 5 || !stats.CostAvailable {
		t.Fatalf("fixed selection or legacy grouped normalization changed: %+v", stats)
	}
	ten := 10.0
	assertWindowEstimate(t, stats.DualCosts.Configured, "complete", &five)
	assertWindowEstimate(t, stats.DualCosts.Reference, "partial", &ten)
}

func float64ForWindowDual(value float64) *float64 { return &value }

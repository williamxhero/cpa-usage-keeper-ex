package quota

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	repositorydto "cpa-usage-keeper/internal/repository/dto"
)

type dualWindowStatsProvider struct {
	stats   repository.UsageWindowStats
	grouped repository.UsageWindowGroupedStats
	err     error
	calls   int
}

func (p *dualWindowStatsProvider) SumByAuthIndex(context.Context, string, time.Time, *time.Time) (repository.UsageWindowStats, error) {
	p.calls++
	return p.stats, p.err
}
func (p *dualWindowStatsProvider) SumGroupsByAuthIndex(context.Context, string, time.Time, *time.Time, repository.UsageWindowStatsGrouper) (repository.UsageWindowGroupedStats, error) {
	p.calls++
	return p.grouped, p.err
}

func quotaTestDualCosts() pricing.DualCosts {
	configured, reference := 6.0, 10.0
	return pricing.DualCosts{
		Configured: pricing.PriceEstimate{TotalCostUSD: &configured, UncachedInputCostUSD: 6, Complete: true, HasKnown: true, Status: "complete"},
		Reference:  pricing.PriceEstimate{TotalCostUSD: &reference, UncachedInputCostUSD: 10, HasKnown: true, Status: "partial", UnavailableReason: "retained_evidence_incomplete"},
	}
}

func TestQuotaWindowDualCostsMapOnlyFreshLocalCosts(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	seconds := int64(5 * 60 * 60)
	providerTokens, providerCost := int64(123), 99.0
	provider := &dualWindowStatsProvider{stats: repository.UsageWindowStats{Tokens: 100, Cost: 6, CostAvailable: true, PricingSnapshotID: "pinned-id", DualCosts: quotaTestDualCosts()}}
	response := CheckResponse{Quota: []QuotaRow{
		{Key: "local-one", Scope: "window", Window: &QuotaWindow{Seconds: &seconds}, ResetAt: now.Add(time.Hour).Format(time.RFC3339)},
		{Key: "local-two", Scope: "window", Window: &QuotaWindow{Seconds: &seconds}, ResetAt: now.Add(time.Hour).Format(time.RFC3339)},
		{Key: "provider", Scope: "window", Window: &QuotaWindow{Seconds: &seconds}, WindowUsageTokens: &providerTokens, WindowUsageCost: &providerCost},
	}}
	actual := (&Service{}).attachWindowUsageStatsWithProvider(context.Background(), "synthetic-auth", response, now, provider)
	if provider.calls != 1 {
		t.Fatalf("expected one pinned local window query, got %d", provider.calls)
	}
	for _, row := range actual.Quota[:2] {
		if row.DualCosts == nil || row.PricingSnapshotID != "pinned-id" || row.WindowUsageCost == nil || *row.WindowUsageCost != 6 || row.DualCosts.Reference.Status != "partial" {
			t.Fatalf("local metadata missing: %+v", row)
		}
	}
	if actual.Quota[2].DualCosts != nil || actual.Quota[2].PricingSnapshotID != "" || *actual.Quota[2].WindowUsageCost != 99 {
		t.Fatalf("provider fee was relabeled with local pricing: %+v", actual.Quota[2])
	}
	*actual.Quota[0].DualCosts.Reference.TotalCostUSD = 999
	if *actual.Quota[1].DualCosts.Reference.TotalCostUSD != 10 || *provider.stats.DualCosts.Reference.TotalCostUSD != 10 {
		t.Fatal("local quota rows share mutable total pointers")
	}
}

func TestQuotaWindowDualCostsClearFailedFallbackAndNormalizeEmptyGroup(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	seconds := int64(5 * 60 * 60)
	oldDual := quotaTestDualCosts()
	tokens := int64(100)
	failed := &dualWindowStatsProvider{err: errors.New("synthetic failure")}
	response := CheckResponse{Quota: []QuotaRow{{Key: "ordinary", Scope: "window", Window: &QuotaWindow{Seconds: &seconds}, ResetAt: now.Add(time.Hour).Format(time.RFC3339), WindowUsageTokens: &tokens, DualCosts: &oldDual, PricingSnapshotID: "old-id"}}}
	actual := (&Service{}).attachWindowUsageStatsWithProvider(context.Background(), "synthetic-auth", response, now, failed)
	if actual.Quota[0].DualCosts != nil || actual.Quota[0].PricingSnapshotID != "" || actual.Quota[0].WindowUsageTokens != nil {
		t.Fatalf("failed fallback retained stale metadata: %+v", actual.Quota[0])
	}
	provider := &dualWindowStatsProvider{grouped: repository.UsageWindowGroupedStats{Complete: true, PricingSnapshotID: "empty-pinned-id", Groups: map[string]repository.UsageWindowStats{}}}
	grouped := CheckResponse{Quota: []QuotaRow{{Key: "bucket." + antigravityGeminiGroupKey + ".primary", GroupKey: antigravityGeminiGroupKey, Scope: "quota_group", Window: &QuotaWindow{Seconds: &seconds}, ResetAt: now.Add(time.Hour).Format(time.RFC3339)}}}
	actual = (&Service{}).attachWindowUsageStatsWithProvider(context.Background(), "synthetic-auth", grouped, now, provider)
	row := actual.Quota[0]
	if row.DualCosts == nil || row.PricingSnapshotID != "empty-pinned-id" || !row.DualCosts.Configured.Complete || !row.DualCosts.Reference.Complete || row.DualCosts.Reference.TotalCostUSD == nil || *row.DualCosts.Reference.TotalCostUSD != 0 {
		t.Fatalf("empty known group is not a pinned known zero: %+v", row)
	}
}

func TestQuotaHistoryDualCostsJSONAndCloneContract(t *testing.T) {
	dual := quotaTestDualCosts()
	history := repositorydto.CodexQuotaEfficiencyHistory{PricingSnapshotID: "pinned-id", Cycles: []repositorydto.CodexQuotaEfficiencyCycle{{
		Usage:       repositorydto.CodexQuotaEfficiencyUsage{TotalCostUSD: 6, CostAvailable: true, PricingSnapshotID: "pinned-id", DualCosts: dual},
		Transitions: []repositorydto.CodexQuotaEfficiencyTransition{{PercentagePoints: 2, CostPerPoint: 3, CostPerPointAvailable: true, Usage: repositorydto.CodexQuotaEfficiencyUsage{DualCosts: dual}, DualCostsPerPoint: dual.Scale(.5)}},
	}}}
	actual := codexQuotaHistoryResponseFromRepository(history)
	body, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		PricingSnapshotID string `json:"pricing_snapshot_id"`
		Cycles            []struct {
			Usage struct {
				DualCosts map[string]map[string]any `json:"dual_costs"`
			} `json:"usage"`
			Transitions []struct {
				DualCostsPerPoint map[string]map[string]any `json:"dual_costs_per_point"`
			} `json:"transitions"`
		} `json:"cycles"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.PricingSnapshotID != "pinned-id" || wire.Cycles[0].Usage.DualCosts["reference"]["status"] != "partial" || wire.Cycles[0].Transitions[0].DualCostsPerPoint["reference"]["total_cost_usd"] != 5.0 {
		t.Fatalf("dual wire contract missing: %s", body)
	}
	for _, name := range []string{"total_cost_usd", "uncached_input_cost_usd", "output_cost_usd", "cache_read_cost_usd", "cache_write_cost_usd", "complete", "has_known", "status"} {
		if _, exists := wire.Cycles[0].Usage.DualCosts["configured"][name]; !exists {
			t.Fatalf("missing frozen dual-cost field %s: %s", name, body)
		}
	}
	*actual.Cycles[0].Usage.DualCosts.Reference.TotalCostUSD = 999
	if *history.Cycles[0].Usage.DualCosts.Reference.TotalCostUSD != 10 {
		t.Fatal("history mapper did not clone totals")
	}
}

func TestQuotaHeaderDualCostsRefreshClearsOldAndClonesNew(t *testing.T) {
	old := quotaTestDualCosts()
	existing := QuotaRow{Key: "window", Scope: "window", DualCosts: &old, PricingSnapshotID: "old-id"}
	cleared := mergeUsageHeaderQuotaRow(existing, QuotaRow{Key: "window", Scope: "window"})
	if cleared.DualCosts != nil || cleared.PricingSnapshotID != "" {
		t.Fatalf("new header window retained old pricing: %+v", cleared)
	}
	newDual := quotaTestDualCosts()
	header := QuotaRow{Key: "window", Scope: "window", DualCosts: &newDual, PricingSnapshotID: "new-id"}
	merged := mergeUsageHeaderQuotaRow(existing, header)
	if merged.PricingSnapshotID != "new-id" || merged.DualCosts == nil {
		t.Fatalf("new projection not merged: %+v", merged)
	}
	*merged.DualCosts.Reference.TotalCostUSD = 999
	if *header.DualCosts.Reference.TotalCostUSD != 10 || *existing.DualCosts.Reference.TotalCostUSD != 10 {
		t.Fatal("header projection shares totals with source")
	}
	preserved := mergeUsageHeaderQuotaRow(QuotaRow{Scope: "additional", DualCosts: &old, PricingSnapshotID: "old-id"}, QuotaRow{Scope: "additional"})
	*preserved.DualCosts.Reference.TotalCostUSD = 777
	if *old.Reference.TotalCostUSD != 10 || preserved.PricingSnapshotID != "old-id" {
		t.Fatal("preserved fee identity or deep clone changed")
	}
}

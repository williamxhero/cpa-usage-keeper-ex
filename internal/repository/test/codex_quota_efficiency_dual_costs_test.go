package test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
)

func TestCodexQuotaEfficiencyDualCostsFallbackRestartsBothProjections(t *testing.T) {
	db := openTestDatabase(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	seedCodexQuotaEfficiencyCycle(t, db, "dual-auth", now.Add(-10*time.Hour), now.Add(-5*time.Hour), []codexQuotaEfficiencySegmentSeed{
		{remaining: 80, first: now.Add(-9 * time.Hour), last: now.Add(-9 * time.Hour)},
		{remaining: 78, first: now.Add(-8 * time.Hour), last: now.Add(-8 * time.Hour)},
	})
	seedCodexQuotaEfficiencyCycle(t, db, "dual-auth", now.Add(-5*time.Hour), now.Add(time.Hour), []codexQuotaEfficiencySegmentSeed{
		{remaining: 90, first: now.Add(-3 * time.Hour), last: now.Add(-3 * time.Hour)},
		{remaining: 88, first: now.Add(-time.Hour), last: now.Add(-time.Hour)},
	})
	completed := usageEventForQuotaEfficiency("dual-completed", "oauth", "dual-auth", now.Add(-8*time.Hour-30*time.Minute), 400_000)
	positive := usageEventForQuotaEfficiency("dual-positive", "oauth", "dual-auth", now.Add(-2*time.Hour), 1_000_000)
	negative := usageEventForQuotaEfficiency("dual-negative", "oauth", "dual-auth", now.Add(-90*time.Minute), -1_000_000)
	negative.TotalTokens = 1_000_000
	negative.ReasoningEffort = "high" // Full-dimension restart must preserve the old separate pricing group.
	unknown := usageEventForQuotaEfficiency("dual-unknown", "oauth", "dual-auth", now.Add(-75*time.Minute), 1_000_000)
	for _, event := range []*entities.UsageEvent{&completed, &positive, &negative} {
		event.Model = "dual-model"
	}
	unknown.Model = "missing"
	seedCodexQuotaEfficiencyUsage(t, db, completed, positive, negative, unknown)
	resolver := windowDualResolver(t, 2, 1)
	result := buildCodexQuotaEfficiencyForTest(t, db, "dual-auth", now, resolver)
	if len(result.Cycles) != 2 || result.PricingSnapshotID != resolver.SnapshotID() {
		t.Fatalf("unexpected cycles/snapshot: %+v", result)
	}
	for _, usage := range []struct {
		index                        int
		total, configured, reference float64
		status                       string
	}{
		{index: 0, total: 3_000_000, configured: 2, reference: 1, status: "partial"},
		{index: 1, total: 400_000, configured: .8, reference: .4, status: "complete"},
	} {
		cycle := result.Cycles[usage.index]
		for _, actual := range []struct {
			configured, reference *float64
			status                string
			id                    string
		}{
			{cycle.Usage.DualCosts.Configured.TotalCostUSD, cycle.Usage.DualCosts.Reference.TotalCostUSD, cycle.Usage.DualCosts.Reference.Status, cycle.Usage.PricingSnapshotID},
			{cycle.Transitions[0].Usage.DualCosts.Configured.TotalCostUSD, cycle.Transitions[0].Usage.DualCosts.Reference.TotalCostUSD, cycle.Transitions[0].Usage.DualCosts.Reference.Status, cycle.Transitions[0].Usage.PricingSnapshotID},
		} {
			if actual.configured == nil || actual.reference == nil || math.Abs(*actual.configured-usage.configured) > 1e-9 || math.Abs(*actual.reference-usage.reference) > 1e-9 || actual.status != usage.status || actual.id != resolver.SnapshotID() {
				t.Fatalf("restart duplicated or lost dual costs: cycle=%+v", cycle)
			}
		}
		if cycle.Usage.TotalTokens != int64(usage.total) || cycle.Usage.TotalCostUSD != usage.configured || cycle.Usage.CostAvailable != (usage.status == "complete") {
			t.Fatalf("legacy costs changed: %+v", cycle.Usage)
		}
		configured, reference := usage.configured/2, usage.reference/2
		assertWindowEstimate(t, cycle.Transitions[0].DualCostsPerPoint.Configured, usage.status, &configured)
		assertWindowEstimate(t, cycle.Transitions[0].DualCostsPerPoint.Reference, usage.status, &reference)
	}
}

func TestCodexQuotaEfficiencyDualCostsReferenceOverflowKeepsLegacyZero(t *testing.T) {
	db := openTestDatabase(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	seedCodexQuotaEfficiencyCycle(t, db, "dual-auth", now.Add(-5*time.Hour), now.Add(time.Hour), []codexQuotaEfficiencySegmentSeed{
		{remaining: 90, first: now.Add(-3 * time.Hour), last: now.Add(-3 * time.Hour)},
		{remaining: 89, first: now.Add(-time.Hour), last: now.Add(-time.Hour)},
	})
	event := usageEventForQuotaEfficiency("dual-overflow", "oauth", "dual-auth", now.Add(-2*time.Hour), 2_000_000)
	event.Model = "dual-model"
	seedCodexQuotaEfficiencyUsage(t, db, event)
	result := buildCodexQuotaEfficiencyForTest(t, db, "dual-auth", now, windowDualResolver(t, 0, math.MaxFloat64))
	cycle := result.Cycles[0]
	zero := 0.0
	if cycle.Usage.TotalCostUSD != 0 || !cycle.Usage.CostAvailable || !cycle.Transitions[0].CostPerPointAvailable {
		t.Fatalf("legacy zero availability changed: %+v", cycle)
	}
	assertWindowEstimate(t, cycle.Usage.DualCosts.Configured, "complete", &zero)
	assertWindowEstimate(t, cycle.Usage.DualCosts.Reference, "unavailable", nil)
	assertWindowEstimate(t, cycle.Transitions[0].DualCostsPerPoint.Reference, "unavailable", nil)
	if _, err := json.Marshal(cycle.Usage.DualCosts); err != nil {
		t.Fatalf("nonfinite reference JSON: %v", err)
	}
}

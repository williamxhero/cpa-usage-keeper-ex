package test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

func assertDualEstimate(t *testing.T, p pricing.PriceEstimate, value float64, status string) {
	t.Helper()
	if p.Status != status || p.Complete != (status == "complete") || p.HasKnown != (status != "unavailable") {
		t.Fatalf("estimate status %+v want %s", p, status)
	}
	if status == "unavailable" {
		if p.TotalCostUSD != nil {
			t.Fatalf("unknown total %+v", p)
		}
		return
	}
	if p.TotalCostUSD == nil {
		t.Fatalf("missing known subtotal %+v", p)
	}
	closeCost(t, *p.TotalCostUSD, value)
	closeCost(t, p.UncachedInputCostUSD+p.OutputCostUSD+p.CacheReadCostUSD+p.CacheWriteCostUSD, value)
}

// All queries use actual persisted events/admin-service mutations and the same
// immutable catalog. No test precomputes the authoritative reference for a DTO.
func TestDualCostsPersistedFiveLayersQueriesRestartAndIndependentBaseline(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	stage := func(first float64, scope string) {
		t.Helper()
		usage := service.NewUsageService(f.db, f.catalog)
		snapshot := f.catalog.Snapshot().ID()
		for _, unit := range []string{"hour", "day"} {
			end := f.end
			if unit == "day" {
				end = f.start.AddDate(0, 0, 1)
			}
			filter := servicedto.UsageFilter{Range: "custom", CustomUnit: unit, StartTime: &f.start, EndTime: &end, EndExclusive: true}
			overview, err := usage.GetUsageOverview(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			if overview.PricingSnapshotID != snapshot || overview.Summary.PricingSnapshotID != snapshot {
				t.Fatal("overview snapshot not pinned")
			}
			closeCost(t, overview.Summary.TotalCost, first+30)
			assertDualEstimate(t, overview.Summary.DualCosts.Configured, first+30, "complete")
			assertDualEstimate(t, overview.Summary.DualCosts.Reference, 20, "complete")
			var series pricing.DualCosts
			for _, dual := range overview.Series.DualCosts {
				series.Merge(dual)
			}
			assertDualEstimate(t, series.Configured, first+30, "complete")
			assertDualEstimate(t, series.Reference, 20, "complete")
			comparisons, err := usage.(service.UsageComparisonProvider).GetUsageOverviewComparisons(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			if comparisons.PricingSnapshotID != snapshot || comparisons.Comparisons.PricingSnapshotID != snapshot {
				t.Fatal("comparison snapshot not pinned")
			}
			for id, configured := range map[string]float64{f.ids[0]: first, f.ids[1]: 30} {
				item := comparisons.Comparisons.Channels[id]
				if item == nil {
					t.Fatal("missing channel")
				}
				assertDualEstimate(t, item.DualCosts.Configured, configured, "complete")
				assertDualEstimate(t, item.DualCosts.Reference, 10, "complete")
			}
			analysis, err := usage.GetAnalysis(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			if analysis.PricingSnapshotID != snapshot || analysis.CostBreakdown.PricingSnapshotID != snapshot {
				t.Fatal("analysis snapshot not pinned")
			}
			assertDualEstimate(t, analysis.CostBreakdown.DualCosts.Configured, first+30, "complete")
			assertDualEstimate(t, analysis.CostBreakdown.DualCosts.Reference, 20, "complete")
			for _, cell := range analysis.Heatmap {
				assertDualEstimate(t, cell.DualCosts.Reference, 20, "complete")
			}
			for _, item := range analysis.ModelEfficiency {
				assertDualEstimate(t, item.DualCosts.Reference, 20, "complete")
			}
		}
		filter := servicedto.UsageFilter{StartTime: &f.start, EndTime: &f.end, EndExclusive: true}
		page, err := usage.ListUsageEvents(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		if page.PricingSnapshotID != snapshot {
			t.Fatal("details snapshot not pinned")
		}
		for _, event := range page.Events {
			configured := 30.0
			wantScope := "legacy"
			if event.AuthIndex == "synthetic-a" {
				configured = first
				wantScope = scope
			}
			assertDualEstimate(t, event.DualCosts.Configured, configured, "complete")
			assertDualEstimate(t, event.DualCosts.Reference, 10, "complete")
			selection := event.PricingSelection
			if selection == nil || selection.Scope != wantScope || selection.SnapshotID != snapshot || selection.BaselineModel != "base" || selection.BaselineBy != "model_alias" || selection.LegacyModelMultiplier != .5 || selection.LegacyRuleMultiplier != 6 || len(selection.MatchedRules) != 2 || selection.LegacyAdjustmentsReplaced != (wantScope != "legacy") {
				t.Fatalf("explanation %+v", selection)
			}
			if event.AuthIndex == "synthetic-a" && scope == "credential_model" && (selection.SelectedModel != "observed-model" || selection.SelectedBy != "model") {
				t.Fatal("override key affected independent baseline")
			}
			body, err := json.Marshal(selection)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "synthetic-a") || strings.Contains(string(body), "priority") {
				t.Fatalf("condition evidence leaked %s", body)
			}
		}
		var export pricing.DualCosts
		if err := usage.StreamUsageEvents(ctx, filter, func(event servicedto.UsageEventRecord) error { export.Merge(event.DualCosts); return nil }); err != nil {
			t.Fatal(err)
		}
		assertDualEstimate(t, export.Reference, 20, "complete")
		realtime, err := usage.GetUsageOverviewRealtime(ctx, servicedto.UsageFilter{RealtimeWindow: "15m", RealtimeEndTime: &f.now})
		if err != nil {
			t.Fatal(err)
		}
		if realtime.PricingSnapshotID != snapshot {
			t.Fatal("realtime snapshot not pinned")
		}
		assertDualEstimate(t, realtime.Insights.Summary.DualCosts.Configured, first+30, "complete")
		assertDualEstimate(t, realtime.Insights.Summary.DualCosts.Reference, 20, "complete")
		for _, item := range realtime.CurrentUsage.Models {
			assertDualEstimate(t, item.DualCosts.Reference, 20, "complete")
		}
	}
	stage(30, "legacy")
	f.set(t, f.ids[0], ".2")
	stage(2, "channel_default")
	if _, err := f.defaults.SetCredentialDefault(ctx, f.subjects[0], ".3"); err != nil {
		t.Fatal(err)
	}
	stage(3, "credential_default")
	models := f.prices.(service.PricingCredentialModelsProvider)
	if _, err := models.SetCredentialModel(ctx, f.subjects[0], "observed-model", "1.2"); err != nil {
		t.Fatal(err)
	}
	stage(12, "credential_model")
	// Persisted channel/model tariff remains independently stored under a higher override.
	if _, err := f.prices.(service.PricingChannelModelsProvider).SetChannelModel(ctx, f.ids[0], "base", ".7"); err != nil {
		t.Fatal(err)
	}
	f.prices, f.catalog = newCatalogPricingService(t, f.db)
	f.defaults = f.prices.(service.PricingCredentialDefaultsProvider)
	stage(12, "credential_model")
}

func TestDualCostsConcurrentPinnedQueryPublicationAndIdentityMigration(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	f.set(t, f.ids[0], ".2")
	usage := service.NewUsageService(f.db, f.catalog)
	filter := servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &f.start, EndTime: &f.end, EndExclusive: true}
	var wg sync.WaitGroup
	for reader := 0; reader < 3; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 12; i++ {
				overview, err := usage.GetUsageOverview(ctx, filter)
				if err != nil {
					t.Error(err)
					return
				}
				if overview.PricingSnapshotID == "" || overview.Summary.PricingSnapshotID != overview.PricingSnapshotID || overview.Summary.DualCosts.Reference.TotalCostUSD == nil || *overview.Summary.DualCosts.Reference.TotalCostUSD != 20 || overview.Summary.DualCosts.Configured.TotalCostUSD == nil || (*overview.Summary.DualCosts.Configured.TotalCostUSD != 32 && *overview.Summary.DualCosts.Configured.TotalCostUSD != 33) {
					t.Errorf("mixed overview %+v", overview.Summary)
					return
				}
				page, err := usage.ListUsageEvents(ctx, filter)
				if err != nil {
					t.Error(err)
					return
				}
				for _, event := range page.Events {
					if event.PricingSnapshotID != page.PricingSnapshotID || event.PricingSelection == nil || event.PricingSelection.SnapshotID != page.PricingSnapshotID || event.DualCosts.Reference.TotalCostUSD == nil || *event.DualCosts.Reference.TotalCostUSD != 10 {
						t.Errorf("mixed details %+v", event)
						return
					}
				}
			}
		}()
	}
	for i := 0; i < 12; i++ {
		value := ".2"
		if i%2 == 1 {
			value = ".3"
		}
		if _, err := f.channels.SetChannelDefault(ctx, f.ids[0], value); err != nil {
			t.Error(err)
			break
		}
	}
	wg.Wait()

	// Ticket9's retained stable subject changes attribution, not unadjusted rates.
	m := newCredentialDefaultFixture(t)
	if _, err := m.defaults.SetCredentialDefault(ctx, m.subjectID, ".2"); err != nil {
		t.Fatal(err)
	}
	assertReference := func(configured float64) {
		t.Helper()
		data, err := service.NewUsageService(m.db, m.catalog).GetUsageOverview(ctx, servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &m.start, EndTime: &m.end, EndExclusive: true})
		if err != nil {
			t.Fatal(err)
		}
		assertDualEstimate(t, data.Summary.DualCosts.Configured, configured, "complete")
		assertDualEstimate(t, data.Summary.DualCosts.Reference, 30, "complete")
	}
	assertReference(33.44)
	migrateFixtureIdentity(t, m, fixtureDirectory(t, m, "synthetic-b").ID)
	assertReference(6)
	m.prices, m.catalog = newCatalogPricingService(t, m.db)
	assertReference(6)
	correct := func(action string) {
		t.Helper()
		state := identityState(t, m.prices)
		target := ""
		if action == "rebind" {
			target = m.subjectID
		}
		if _, err := m.prices.(service.PricingIdentityMigrationProvider).CorrectPricingIdentity(ctx, service.PricingIdentityCorrectionInput{BindingRef: "binding_" + m.subjectID, ExpectedSubjectID: m.subjectID, TargetSubjectID: target, Action: action, SnapshotID: state.SnapshotID, Confirmed: true}); err != nil {
			t.Fatal(err)
		}
	}
	correct("unbind")
	assertReference(59.56) // Original A keeps grouped legacy57.6; migrated B remains1.96.
	correct("rebind")
	assertReference(6)
}

func TestDualCostsLegacyGroupedClampingPrunedProofAndCommittedPrefix(t *testing.T) {
	f := newCredentialDefaultFixture(t)
	usage := service.NewUsageService(f.db, f.catalog)
	ctx := context.Background()
	filter := servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &f.start, EndTime: &f.end, EndExclusive: true}
	overview, err := usage.GetUsageOverview(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, overview.Summary.TotalCost, 87)
	assertDualEstimate(t, overview.Summary.DualCosts.Configured, 87, "complete")
	assertDualEstimate(t, overview.Summary.DualCosts.Reference, 30, "complete")
	// Delete one committed fact; keep the remaining exact retained subtotal.
	if err := f.db.Where("event_key = ?", "default-abnormal-a").Delete(&entities.UsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	overview, err = usage.GetUsageOverview(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, overview.Summary.TotalCost, 87)
	assertDualEstimate(t, overview.Summary.DualCosts.Reference, 29.6, "partial")
	if err := f.db.Where("1 = 1").Delete(&entities.UsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	// A later equal-token historical request is not a committed rollup member.
	late := f.events[2]
	late.ID = 0
	late.EventKey = "dual-late-impostor"
	late.Timestamp = f.start.Add(12*time.Hour + 3*time.Minute)
	if _, _, err := repository.InsertUsageEvents(f.db, []entities.UsageEvent{late}); err != nil {
		t.Fatal(err)
	}
	overview, err = usage.GetUsageOverview(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, overview.Summary.TotalCost, 87)
	assertDualEstimate(t, overview.Summary.DualCosts.Reference, 0, "unavailable")
	if overview.Summary.DualCosts.Reference.UnavailableReason != "retained_evidence_incomplete" {
		t.Fatal("missing safe proof reason")
	}
}

func TestDualCostsFourBucketsFilteredHourDayAndDailyAverage(t *testing.T) {
	f, models := channelModelFixedFixture(t, true)
	ctx := context.Background()
	for i, id := range f.ids {
		if _, err := models.SetChannelFixed(ctx, id, "observed-model", syntheticFixed(float64(i+1), "openai")); err != nil {
			t.Fatal(err)
		}
	}
	var key entities.CPAAPIKey
	if err := f.db.Where("api_key = ?", "synthetic-downstream").First(&key).Error; err != nil {
		t.Fatal(err)
	}
	usage := service.NewUsageService(f.db, f.catalog)
	for _, unit := range []string{"hour", "day"} {
		end := f.start.AddDate(0, 0, 2)
		filter := servicedto.UsageFilter{Range: "custom", CustomUnit: unit, StartTime: &f.start, EndTime: &end, EndExclusive: true, APIKeyID: strconv.FormatInt(key.ID, 10)}
		overview, err := usage.GetUsageOverview(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		assertDualEstimate(t, overview.Summary.DualCosts.Configured, 30, "complete")
		assertDualEstimate(t, overview.Summary.DualCosts.Reference, 72, "complete")
		if overview.Summary.DailyAverageDualCosts == nil {
			t.Fatal("missing authoritative average")
		}
		assertDualEstimate(t, overview.Summary.DailyAverageDualCosts.Configured, 15, "complete")
		assertDualEstimate(t, overview.Summary.DailyAverageDualCosts.Reference, 36, "complete")
		analysis, err := usage.GetAnalysis(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		dual := analysis.CostBreakdown.DualCosts
		for _, test := range []struct{ actual, want float64 }{{dual.Configured.UncachedInputCostUSD, 3}, {dual.Configured.OutputCostUSD, 6}, {dual.Configured.CacheReadCostUSD, 9}, {dual.Configured.CacheWriteCostUSD, 12}, {dual.Reference.UncachedInputCostUSD, 20}, {dual.Reference.OutputCostUSD, 40}, {dual.Reference.CacheReadCostUSD, 4}, {dual.Reference.CacheWriteCostUSD, 8}} {
			closeCost(t, test.actual, test.want)
		}
	}
}

func TestDualCostsPersistedFixedWithoutBaselineAndUnknownZeroIndependence(t *testing.T) {
	f, models := channelModelFixedFixture(t, false, "unpriced-alias")
	ctx := context.Background()
	for i, id := range f.ids {
		if _, err := models.SetChannelFixed(ctx, id, "observed-model", syntheticFixed(float64(i+1), "openai")); err != nil {
			t.Fatal(err)
		}
	}
	check := func(configured float64, status string) {
		t.Helper()
		usage := service.NewUsageService(f.db, f.catalog)
		filter := servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &f.start, EndTime: &f.end, EndExclusive: true}
		overview, err := usage.GetUsageOverview(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		assertDualEstimate(t, overview.Summary.DualCosts.Configured, configured, status)
		assertDualEstimate(t, overview.Summary.DualCosts.Reference, 0, "unavailable")
	}
	check(30, "complete")
	credentials := f.prices.(service.PricingCredentialModelsProvider)
	if _, err := credentials.SetCredentialModel(ctx, f.subjects[0], "observed-model", "0"); err != nil {
		t.Fatal(err)
	}
	check(20, "partial")
	if _, err := credentials.SetCredentialModel(ctx, f.subjects[1], "observed-model", "0"); err != nil {
		t.Fatal(err)
	}
	check(0, "unavailable")
	for _, subject := range f.subjects {
		if _, err := f.prices.(service.PricingCredentialFixedProvider).SetCredentialFixed(ctx, subject, "observed-model", syntheticFixed(0, "openai")); err != nil {
			t.Fatal(err)
		}
	}
	check(0, "complete")
}

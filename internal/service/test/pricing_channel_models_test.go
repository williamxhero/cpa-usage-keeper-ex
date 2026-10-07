package test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	repodto "cpa-usage-keeper/internal/repository/dto"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"gorm.io/gorm"
)

func TestChannelModelPersistedFiveLayerAddClearModelAliasAllCostFamilies(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	models := f.prices.(service.PricingChannelModelsProvider)
	credentials := f.prices.(service.PricingCredentialModelsProvider)
	assertStage := func(first float64, scope, selected, by string) {
		t.Helper()
		f.assertCostFamilies(t, first+30, true)
		f.assertChannels(t, map[string]float64{f.ids[0]: first, f.ids[1]: 30})
		page, err := service.NewUsageService(f.db, f.catalog).ListUsageEvents(ctx, servicedto.UsageFilter{StartTime: &f.start, EndTime: &f.end, EndExclusive: true})
		if err != nil || len(page.Events) != 2 {
			t.Fatalf("details %+v %v", page, err)
		}
		for _, event := range page.Events {
			if event.AuthIndex != "synthetic-a" {
				continue
			}
			closeCost(t, event.CostUSD, first)
			if event.ChannelID != f.ids[0] {
				t.Fatalf("lost channel %+v", event)
			}
			selection := event.PricingSelection
			if scope == "" {
				if selection == nil || !selection.Legacy || selection.Scope != "legacy" || selection.LegacyAdjustmentsReplaced || !selection.BaselineAvailable || selection.SnapshotID != page.PricingSnapshotID {
					t.Fatalf("legacy explanation %+v", selection)
				}
				closeCost(t, *selection.BaselineCostUSD, 10)
				continue
			}
			if selection == nil || selection.Scope != scope || selection.SelectedModel != selected || selection.SelectedBy != by || selection.BaselineModel != "base" || selection.BaselineBy != "model_alias" || !selection.BaselineAvailable {
				t.Fatalf("selection %+v", selection)
			}
			closeCost(t, *selection.BaselineCostUSD, 10)
		}
	}
	assertStage(30, "", "", "")
	f.set(t, f.ids[0], ".2")
	assertStage(2, "channel_default", "", "")
	for _, text := range []string{"1.1", "1.1x", "110%"} {
		if _, err := models.SetChannelModel(ctx, f.ids[0], " observed-model ", text); err != nil {
			t.Fatal(err)
		}
		assertStage(11, "channel_model", "observed-model", "model")
	}
	if _, err := models.SetChannelModel(ctx, f.ids[0], "base", ".7"); err != nil {
		t.Fatal(err)
	}
	assertStage(11, "channel_model", "observed-model", "model")
	if _, err := f.defaults.SetCredentialDefault(ctx, f.subjects[0], ".3"); err != nil {
		t.Fatal(err)
	}
	assertStage(3, "credential_default", "", "")
	if _, err := credentials.SetCredentialModel(ctx, f.subjects[0], "observed-model", "1.2"); err != nil {
		t.Fatal(err)
	}
	assertStage(12, "credential_model", "observed-model", "model")
	if _, err := credentials.SetCredentialModel(ctx, f.subjects[0], "base", ".8"); err != nil {
		t.Fatal(err)
	}
	// Restart recovers all five layers through one full loader.
	f.prices, f.catalog = newCatalogPricingService(t, f.db)
	f.channels = f.prices.(service.PricingChannelsProvider)
	f.defaults = f.prices.(service.PricingCredentialDefaultsProvider)
	models = f.prices.(service.PricingChannelModelsProvider)
	credentials = f.prices.(service.PricingCredentialModelsProvider)
	assertStage(12, "credential_model", "observed-model", "model")
	if _, err := credentials.ClearCredentialModel(ctx, f.subjects[0], "observed-model"); err != nil {
		t.Fatal(err)
	}
	// Credential Alias beats exact channel Model; scope priority comes first.
	assertStage(8, "credential_model", "base", "model_alias")
	if _, err := credentials.ClearCredentialModel(ctx, f.subjects[0], "base"); err != nil {
		t.Fatal(err)
	}
	assertStage(3, "credential_default", "", "")
	if _, err := f.defaults.ClearCredentialDefault(ctx, f.subjects[0]); err != nil {
		t.Fatal(err)
	}
	assertStage(11, "channel_model", "observed-model", "model")
	for _, text := range []string{"0", "1"} {
		if _, err := models.SetChannelModel(ctx, f.ids[0], "observed-model", text); err != nil {
			t.Fatal(err)
		}
		first := 0.0
		if text == "1" {
			first = 10
		}
		assertStage(first, "channel_model", "observed-model", "model")
	}
	if _, err := models.ClearChannelModel(ctx, f.ids[0], "observed-model"); err != nil {
		t.Fatal(err)
	}
	assertStage(7, "channel_model", "base", "model_alias")
	if _, err := models.ClearChannelModel(ctx, f.ids[0], "base"); err != nil {
		t.Fatal(err)
	}
	assertStage(2, "channel_default", "", "")
	if _, err := f.channels.ClearChannelDefault(ctx, f.ids[0]); err != nil {
		t.Fatal(err)
	}
	assertStage(30, "", "", "")
}

func channelModelFixedFixture(t *testing.T, baseline bool, aliases ...string) (channelFixture, service.PricingChannelModelsProvider) {
	t.Helper()
	base, second := newCredentialFixedFixture(t, baseline, aliases...)
	f := attachFixedChannels(t, base, second)
	for _, id := range f.ids {
		if _, err := f.channels.ClearChannelDefault(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	return f, f.prices.(service.PricingChannelModelsProvider)
}

func TestChannelModelOnlyFixedFourBucketsClampingWindowsEfficiencyRestart(t *testing.T) {
	f, models := channelModelFixedFixture(t, true)
	ctx := context.Background()
	for i, id := range f.ids {
		if _, err := models.SetChannelFixed(ctx, id, "observed-model", syntheticFixed(float64(i+1), "openai")); err != nil {
			t.Fatal(err)
		}
	}
	if f.catalog.NewResolver().HasCredentialDefaults() {
		t.Fatal("fixture has credential default")
	}
	f.assertCostFamilies(t, 30, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 10, f.ids[1]: 20})
	result := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if result.Scope != "channel_model" || result.Mode != "fixed" || result.PricingStyle != "claude" || !result.ReferenceAvailable {
		t.Fatalf("fixed %+v", result)
	}
	closeCost(t, result.Cost.UncachedInputCostUSD, 1)
	closeCost(t, result.Cost.OutputCostUSD, 2)
	closeCost(t, result.Cost.CacheReadCostUSD, 3)
	closeCost(t, result.Cost.CacheWriteCostUSD, 4)
	closeCost(t, result.ReferenceCost.TotalCostUSD, 36)
	// Abnormal cache subtraction is normalized per event, never after summing.
	event := f.events[0]
	event.ID = 0
	event.EventKey = "channel-fixed-cache-clamp"
	event.Timestamp = f.start.Add(12*time.Hour + 3*time.Minute)
	event.InputTokens = 100_000
	event.OutputTokens = 100_000
	event.CacheReadTokens = 200_000
	event.CacheCreationTokens = 100_000
	event.TotalTokens = 500_000
	if _, _, err := repository.InsertUsageEvents(f.db, []entities.UsageEvent{event}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, f.db, f.start.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 31.2, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 11.2, f.ids[1]: 20})
	analysis, err := service.NewUsageService(f.db, f.catalog).GetAnalysis(ctx, servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &f.start, EndTime: &f.end, EndExclusive: true})
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, analysis.CostBreakdown.UncachedInputCostUSD, 3)
	closeCost(t, analysis.CostBreakdown.OutputCostUSD, 6.2)
	closeCost(t, analysis.CostBreakdown.CacheReadCostUSD, 9.6)
	closeCost(t, analysis.CostBreakdown.CacheWriteCostUSD, 12.4)
	cycle := entities.QuotaCycle{Provider: "codex", AuthIndex: "synthetic-a", QuotaKey: "rate_limit.primary_window", WindowSeconds: 5 * 60 * 60, ResetAtSource: entities.QuotaResetAtSourceAbsolute, WindowStartedAt: f.start.Add(10 * time.Hour), ResetAt: f.start.Add(15 * time.Hour), FirstObservedAt: f.start.Add(10 * time.Hour), LastObservedAt: f.now, CreatedAt: f.start, UpdatedAt: f.start}
	if err := f.db.Create(&cycle).Error; err != nil {
		t.Fatal(err)
	}
	quota, err := repository.BuildCodexQuotaEfficiencyHistory(ctx, f.db, repodto.CodexQuotaEfficiencyQuery{Provider: "codex", AuthIndex: "synthetic-a", Now: f.now, RangeStart: f.start}, f.catalog.NewResolver())
	if err != nil || len(quota.Cycles) != 1 || !quota.Cycles[0].Usage.CostAvailable {
		t.Fatalf("efficiency %+v %v", quota, err)
	}
	closeCost(t, quota.Cycles[0].Usage.TotalCostUSD, 11.2)
	f.prices, f.catalog = newCatalogPricingService(t, f.db)
	models = f.prices.(service.PricingChannelModelsProvider)
	f.channels = f.prices.(service.PricingChannelsProvider)
	read, err := models.GetChannelModel(ctx, f.ids[0], "observed-model")
	if err != nil || read.Mode != "fixed" || read.Multiplier != nil || *read.Fixed.PromptPricePer1M != 1 {
		t.Fatalf("restart %+v %v", read, err)
	}
	f.assertCostFamilies(t, 31.2, true)
	if _, err := models.SetChannelModel(ctx, f.ids[0], "observed-model", ".2"); err != nil {
		t.Fatal(err)
	}
	read, err = models.GetChannelModel(ctx, f.ids[0], "observed-model")
	if err != nil || read.Fixed != nil || read.Mode != "multiplier" {
		t.Fatalf("inactive fields %+v %v", read, err)
	}
	f.assertCostFamilies(t, 27.76, true) // (36 + 2.8) * .2 plus second fixed20.
}

func TestChannelModelNoBaselineHigherMultiplierNeverDowngradesAndZeroFixed(t *testing.T) {
	f, models := channelModelFixedFixture(t, false, "unpriced-alias")
	ctx := context.Background()
	credentials := f.prices.(service.PricingCredentialModelsProvider)
	for i, id := range f.ids {
		if _, err := models.SetChannelFixed(ctx, id, "observed-model", syntheticFixed(float64(i+1), "openai")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := models.SetChannelFixed(ctx, f.ids[0], "unpriced-alias", syntheticFixed(3, "claude")); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 30, true)
	result := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if !result.Available || result.ReferenceAvailable {
		t.Fatalf("invented reference %+v", result)
	}
	assertPartial := func(scope string) {
		t.Helper()
		usage := service.NewUsageService(f.db, f.catalog)
		for _, unit := range []string{"hour", "day"} {
			end := f.end
			if unit == "day" {
				end = f.start.AddDate(0, 0, 1)
			}
			filter := servicedto.UsageFilter{Range: "custom", CustomUnit: unit, StartTime: &f.start, EndTime: &end, EndExclusive: true}
			overview, err := usage.GetUsageOverview(ctx, filter)
			if err != nil || overview.Summary.CostAvailable {
				t.Fatalf("partial overview %+v %v", overview, err)
			}
			closeCost(t, overview.Summary.TotalCost, 20)
			analysis, err := usage.GetAnalysis(ctx, filter)
			if err != nil || analysis.CostBreakdown.CostAvailable {
				t.Fatalf("partial analysis %+v %v", analysis, err)
			}
			closeCost(t, analysis.CostBreakdown.TotalCostUSD, 20)
			comparisons, err := usage.(service.UsageComparisonProvider).GetUsageOverviewComparisons(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			a, b := comparisons.Comparisons.Channels[f.ids[0]], comparisons.Comparisons.Channels[f.ids[1]]
			if a == nil || b == nil || a.CostAvailable || !b.CostAvailable {
				t.Fatalf("partial channels %+v %+v", a, b)
			}
			closeCost(t, b.CostUSD, 20)
		}
		page, err := usage.ListUsageEvents(ctx, servicedto.UsageFilter{StartTime: &f.start, EndTime: &f.end, EndExclusive: true})
		if err != nil || len(page.Events) != 2 {
			t.Fatalf("details %+v %v", page, err)
		}
		for _, event := range page.Events {
			if event.AuthIndex == "synthetic-a" && (event.CostAvailable || event.PricingSelection == nil || event.PricingSelection.Scope != scope || event.PricingSelection.UnavailableReason != "missing_baseline") {
				t.Fatalf("downgraded %+v", event)
			}
		}
	}
	for _, text := range []string{".3", "0"} {
		if _, err := credentials.SetCredentialModel(ctx, f.subjectID, "unpriced-alias", text); err != nil {
			t.Fatal(err)
		}
		assertPartial("credential_model")
	}
	if _, err := credentials.ClearCredentialModel(ctx, f.subjectID, "unpriced-alias"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.defaults.SetCredentialDefault(ctx, f.subjectID, "0"); err != nil {
		t.Fatal(err)
	}
	assertPartial("credential_default")
	if _, err := f.defaults.ClearCredentialDefault(ctx, f.subjectID); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{".3", "0"} {
		if _, err := models.SetChannelModel(ctx, f.ids[0], "observed-model", text); err != nil {
			t.Fatal(err)
		}
		assertPartial("channel_model")
	}
	empty := repository.UsageEventCostSubject(f.events[0])
	empty.Tokens = helper.UsageTokenCostInput{}
	result = f.catalog.NewResolver().Calculate(empty)
	if !result.Available || !result.ReferenceAvailable || result.Cost.TotalCostUSD != 0 {
		t.Fatalf("empty %+v", result)
	}
	if _, err := models.SetChannelFixed(ctx, f.ids[0], "observed-model", syntheticFixed(0, "claude")); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 20, true)
	// Missing retained facts cannot be guessed from a model-only rollup.
	if err := f.db.Where("event_key = ?", f.events[0].EventKey).Delete(&entities.UsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	overview, err := service.NewUsageService(f.db, f.catalog).GetUsageOverview(ctx, servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &f.start, EndTime: &f.end, EndExclusive: true})
	if err != nil || overview.Summary.CostAvailable || overview.Summary.UnavailableReason != "retained_pricing_evidence_incomplete" {
		t.Fatalf("retained evidence %+v %v", overview, err)
	}
}

func TestChannelModelInvalidCandidateCommitPinnedReadersAndDeletion(t *testing.T) {
	f, models := channelModelFixedFixture(t, false)
	ctx := context.Background()
	if _, err := models.SetChannelFixed(ctx, f.ids[0], "observed-model", syntheticFixed(1, "openai")); err != nil {
		t.Fatal(err)
	}
	previous := f.catalog.Snapshot()
	invalid := []pricing.FixedTariff{syntheticFixed(1, ""), syntheticFixed(1, "guessed"), syntheticFixed(-1, "openai"), syntheticFixed(math.NaN(), "openai"), syntheticFixed(math.Inf(1), "openai"), syntheticFixed(math.MaxFloat64, "openai")}
	for i := range 4 {
		value := syntheticFixed(1, "openai")
		switch i {
		case 0:
			value.PromptPricePer1M = nil
		case 1:
			value.CompletionPricePer1M = nil
		case 2:
			value.CacheReadPricePer1M = nil
		case 3:
			value.CacheWritePricePer1M = nil
		}
		invalid = append(invalid, value)
	}
	for _, value := range invalid {
		if _, err := models.SetChannelFixed(ctx, f.ids[0], "observed-model", value); !errors.Is(err, service.ErrInvalidPricingInput) {
			t.Fatalf("accepted %+v %v", value, err)
		}
		if f.catalog.Snapshot() != previous {
			t.Fatal("invalid publication")
		}
	}
	for _, text := range []string{"", "-1", "NaN", "Infinity", "1e2", ".2x%"} {
		if _, err := models.SetChannelModel(ctx, f.ids[0], "observed-model", text); !errors.Is(err, service.ErrInvalidPricingInput) {
			t.Fatalf("invalid %q %v", text, err)
		}
	}
	if _, err := models.SetChannelModel(ctx, "missing", "observed-model", ".2"); !errors.Is(err, service.ErrPricingChannelNotFound) {
		t.Fatalf("missing channel %v", err)
	}
	callback := "synthetic_channel_model_candidate_failure"
	if err := f.db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "channel_model_prices" {
			tx.AddError(errors.New("synthetic candidate failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := models.SetChannelModel(ctx, f.ids[0], "observed-model", ".2"); err == nil {
		t.Fatal("candidate failure accepted")
	}
	if err := f.db.Callback().Query().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if f.catalog.Snapshot() != previous {
		t.Fatal("candidate failure published")
	}
	if err := f.db.Exec(`CREATE TABLE channel_model_commit_probe (channel_id TEXT, FOREIGN KEY(channel_id) REFERENCES pricing_channels(id) DEFERRABLE INITIALLY DEFERRED)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`CREATE TRIGGER fail_channel_model_commit AFTER UPDATE ON channel_model_prices BEGIN INSERT INTO channel_model_commit_probe(channel_id) VALUES ('absent-channel'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := models.SetChannelFixed(ctx, f.ids[0], "observed-model", syntheticFixed(2, "claude")); err == nil {
		t.Fatal("COMMIT failure accepted")
	}
	if f.catalog.Snapshot() != previous {
		t.Fatal("COMMIT failure published")
	}
	restarted, _ := newCatalogPricingService(t, f.db)
	read, err := restarted.(service.PricingChannelModelsProvider).GetChannelModel(ctx, f.ids[0], "observed-model")
	if err != nil || read.Mode != "fixed" || *read.Fixed.PromptPricePer1M != 1 {
		t.Fatalf("rollback %+v %v", read, err)
	}
	if err := f.db.Exec("DROP TRIGGER fail_channel_model_commit").Error; err != nil {
		t.Fatal(err)
	}
	old := f.catalog.NewResolver()
	subject := repository.UsageEventCostSubject(f.events[0])
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range 50 {
				result := old.Calculate(subject)
				latest := f.catalog.NewResolver().Calculate(subject)
				if !latest.Available || latest.ReferenceAvailable || latest.Scope != "channel_model" || latest.Mode != "fixed" || (latest.Cost.TotalCostUSD != 10 && latest.Cost.TotalCostUSD != 20) {
					t.Errorf("mixed current reader %+v", latest)
					return
				}
				if result.Cost.TotalCostUSD != 10 || result.ReferenceAvailable || result.Scope != "channel_model" || result.Mode != "fixed" {
					t.Errorf("mixed pinned %+v", result)
					return
				}
			}
		}()
	}
	close(start)
	if _, err := models.SetChannelFixed(ctx, f.ids[0], "observed-model", syntheticFixed(2, "claude")); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	closeCost(t, f.catalog.NewResolver().Calculate(subject).Cost.TotalCostUSD, 20)
	read, err = models.GetChannelModel(ctx, f.ids[0], "observed-model")
	if err != nil {
		t.Fatal(err)
	}
	*read.Fixed.PromptPricePer1M = 999
	if fresh, _ := models.GetChannelModel(ctx, f.ids[0], "observed-model"); *fresh.Fixed.PromptPricePer1M != 2 {
		t.Fatal("mutable DTO leaked")
	}
	// A model-only empty channel is still a deletion dependency.
	empty, err := f.channels.CreatePricingChannel(ctx, service.PricingChannelInput{Name: "Empty model channel", MemberSubjectIDs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := models.SetChannelModel(ctx, empty.ID, "observed-model", "1"); err != nil {
		t.Fatal(err)
	}
	if err := f.channels.DeletePricingChannel(ctx, empty.ID, false); !errors.Is(err, service.ErrPricingChannelDependencies) {
		t.Fatalf("unconfirmed model deletion %v", err)
	}
	if err := f.channels.DeletePricingChannel(ctx, empty.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.LoadPricingSnapshot(ctx, f.db); err != nil {
		t.Fatalf("orphaned deleted price %v", err)
	}
	credentialModels := f.prices.(service.PricingCredentialModelsProvider)
	if _, err := credentialModels.SetCredentialModel(ctx, f.subjectID, "observed-model", ".9"); err != nil {
		t.Fatal(err)
	}
	if err := f.channels.DeletePricingChannel(ctx, f.ids[0], true); err != nil {
		t.Fatal(err)
	}
	if config, err := credentialModels.GetCredentialModel(ctx, f.subjectID, "observed-model"); err != nil || config.Mode != "multiplier" || config.Multiplier == nil || *config.Multiplier != .9 {
		t.Fatalf("deleted credential price %+v %v", config, err)
	}
	var eventCount, subjectCount int64
	if err := f.db.Model(&entities.UsageEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&entities.CredentialPricingSubject{}).Where("id = ?", f.subjectID).Count(&subjectCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 || subjectCount != 1 {
		t.Fatalf("deleted preserved evidence: events %d subject %d", eventCount, subjectCount)
	}
}

func TestChannelModelExactIdentityAmbiguityStyleLossAndLegacyCompatibility(t *testing.T) {
	f, models := channelModelFixedFixture(t, true)
	ctx := context.Background()
	legacy := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if _, err := models.SetChannelFixed(ctx, f.ids[0], "base", syntheticFixed(1, "")); err != nil {
		t.Fatal(err)
	}
	previous := f.catalog.Snapshot()
	if err := f.prices.DeletePricing(ctx, "base"); !errors.Is(err, repository.ErrInvalidPricingSnapshot) {
		t.Fatalf("lost required style %v", err)
	}
	if f.catalog.Snapshot() != previous {
		t.Fatal("style loss published")
	}
	if _, err := f.prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "observed-model", PricingStyle: "openai", PromptPricePer1M: 5}); err != nil {
		t.Fatal(err)
	}
	result := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if result.SelectedModel != "base" || result.SelectedBy != "model_alias" || result.MatchedModel != "observed-model" || result.PricingStyle != "openai" {
		t.Fatalf("coupled baseline %+v", result)
	}
	closeCost(t, result.Cost.TotalCostUSD, 10)
	// Restore the original baseline scenario for exact fallback comparison.
	if err := f.prices.DeletePricing(ctx, "observed-model"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", "unknown", " oauth "} {
		subject := repository.UsageEventCostSubject(f.events[0])
		subject.AuthType = raw
		result := f.catalog.NewResolver().Calculate(subject)
		if result.Scope != "" || result.ChannelID != "" || result.AttributionWarning != "unknown_identity" {
			t.Fatalf("unsafe typed fallback %+v", result)
		}
		closeCost(t, result.Cost.TotalCostUSD, legacy.Cost.TotalCostUSD)
	}
	if err := f.db.Model(&entities.UsageIdentity{}).Where("identity = ?", "synthetic-a").Update("binding_identity_status", "ambiguous").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := models.SetChannelModel(ctx, f.ids[0], "base", ".2"); !errors.Is(err, service.ErrPricingChannelConflict) {
		t.Fatalf("ambiguous save %v", err)
	}
	// Real pricing mutation reloads committed identity evidence without discarding saved tariffs.
	if _, err := f.prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "unrelated", PromptPricePer1M: 1}); err != nil {
		t.Fatal(err)
	}
	result = f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if result.Scope != "" || result.ChannelID != "" || result.AttributionWarning != "unresolved_identity" {
		t.Fatalf("ambiguous attribution %+v", result)
	}
	closeCost(t, result.Cost.TotalCostUSD, legacy.Cost.TotalCostUSD)
	if _, err := models.ClearChannelModel(ctx, f.ids[0], "base"); err != nil {
		t.Fatal(err)
	}
	// Clearing overrides preserves configured legacy fees; additive explanation
	// and independent baseline reference remain available under ticket 8.
	restored := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[1]))
	selection := f.catalog.NewResolver().Selection(restored)
	if !reflect.DeepEqual(legacy.Cost, restored.Cost) || selection == nil || !selection.Legacy || !selection.BaselineAvailable || selection.Scope != "legacy" {
		t.Fatalf("legacy changed %+v %+v", legacy, restored)
	}
}

func TestChannelModelOnlyHotArchiveDuplicateCoverageAndMissingEvidence(t *testing.T) {
	f, models := channelModelFixedFixture(t, true)
	ctx := context.Background()
	for i, id := range f.ids {
		if _, err := models.SetChannelFixed(ctx, id, "observed-model", syntheticFixed(float64(i+1), "claude")); err != nil {
			t.Fatal(err)
		}
	}
	var event entities.UsageEvent
	if err := f.db.First(&event, "event_key = ?", f.events[0].EventKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("INSERT INTO usage_events_archive ("+entities.UsageEventStorageColumns+") SELECT "+entities.UsageEventStorageColumns+" FROM usage_events WHERE id = ?", event.ID).Error; err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 30, false)
	if err := f.db.Delete(&event).Error; err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 30, false)
	f.assertChannels(t, map[string]float64{f.ids[0]: 10, f.ids[1]: 20})
	if err := f.db.Table("usage_events_archive").Where("id = ?", event.ID).Delete(&entities.UsageEventArchive{}).Error; err != nil {
		t.Fatal(err)
	}
	overview, err := service.NewUsageService(f.db, f.catalog).GetUsageOverview(ctx, servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &f.start, EndTime: &f.end, EndExclusive: true})
	if err != nil || overview.Summary.CostAvailable || overview.Summary.UnavailableReason != "retained_pricing_evidence_incomplete" {
		t.Fatalf("missing archive evidence %+v %v", overview, err)
	}
	closeCost(t, overview.Summary.TotalCost, 20)
}

func TestChannelModelOnlySameIndexTypedEventsAndOriginalPadding(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	models := f.prices.(service.PricingChannelModelsProvider)
	if _, err := models.SetChannelModel(ctx, f.ids[0], "base", ".2"); err != nil {
		t.Fatal(err)
	}
	other := entities.UsageIdentity{Name: "Synthetic model OAuth", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "synthetic-a", Type: "codex", BindingIdentityStatus: "unique"}
	if err := f.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	subject, err := f.prices.(service.PricingCredentialProvider).BindPricingCredential(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	channel, err := f.channels.CreatePricingChannel(ctx, service.PricingChannelInput{Name: "OAuth model channel", MemberSubjectIDs: []string{subject.SubjectID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := models.SetChannelModel(ctx, channel.ID, "base", ".8"); err != nil {
		t.Fatal(err)
	}
	start := f.start.Add(10 * time.Hour)
	end := start.Add(time.Hour)
	events := []entities.UsageEvent{}
	for i, evidence := range []struct{ index, kind string }{{"synthetic-a", "apikey"}, {"synthetic-a", "oauth"}, {"synthetic-a", ""}, {"synthetic-a", " apikey "}, {" synthetic-a ", "apikey"}} {
		events = append(events, entities.UsageEvent{EventKey: "channel-model-typed-" + string(rune('a'+i)), APIGroupKey: "synthetic-downstream", Model: "base", AuthType: evidence.kind, AuthIndex: evidence.index, Timestamp: start.Add(time.Duration(i) * time.Minute), InputTokens: 1_000_000, TotalTokens: 1_000_000})
	}
	if _, _, err := repository.InsertUsageEvents(f.db, events); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, f.db, f.end); err != nil {
		t.Fatal(err)
	}
	usage := service.NewUsageService(f.db, f.catalog)
	filter := servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &start, EndTime: &end, EndExclusive: true}
	page, err := usage.ListUsageEvents(ctx, filter)
	if err != nil || len(page.Events) != 5 {
		t.Fatalf("typed details %+v %v", page, err)
	}
	sum := 0.0
	unknown := 0
	for _, event := range page.Events {
		sum += event.CostUSD
		if event.ChannelID == "" {
			unknown++
			if event.AttributionWarning == "" || event.PricingSelection == nil || event.PricingSelection.Scope != "legacy" {
				t.Fatalf("guessed identity %+v", event)
			}
		} else if event.PricingSelection == nil || event.PricingSelection.Scope != "channel_model" {
			t.Fatalf("lost model scope %+v", event)
		}
	}
	closeCost(t, sum, 25)
	if unknown != 3 {
		t.Fatal(unknown)
	}
	overview, err := usage.GetUsageOverview(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, overview.Summary.TotalCost, 25)
	comparisons, err := usage.(service.UsageComparisonProvider).GetUsageOverviewComparisons(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	for id, cost := range map[string]float64{f.ids[0]: 2, channel.ID: 8, repository.UnknownPricingChannel: 15} {
		row := comparisons.Comparisons.Channels[id]
		if row == nil {
			t.Fatal(id)
		}
		closeCost(t, row.CostUSD, cost)
	}
	closeCost(t, comparisons.Comparisons.AIProviders["synthetic-a"].CostUSD, 2)
	closeCost(t, comparisons.Comparisons.AuthFiles["synthetic-a"].CostUSD, 8)
	analysis, err := usage.GetAnalysis(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, analysis.CostBreakdown.TotalCostUSD, 25)
	window, err := repository.SumUsageWindowStatsByAuthIndex(ctx, f.db, "synthetic-a", start, &end, f.catalog.NewResolver())
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, window.Cost, 20)
}

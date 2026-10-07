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
)

func syntheticFixed(factor float64, style string) pricing.FixedTariff {
	return pricing.FixedTariff{PromptPricePer1M: new(factor), CompletionPricePer1M: new(2 * factor), CacheReadPricePer1M: new(3 * factor), CacheWritePricePer1M: new(4 * factor), PricingStyle: style}
}
func newCredentialFixedFixture(t *testing.T, baseline bool, requestAliases ...string) (credentialDefaultFixture, string) {
	t.Helper()
	ctx := context.Background()
	db := openUsageServiceTestDatabase(t)
	identities := []entities.UsageIdentity{{Name: "Synthetic A", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "synthetic-a", Type: "codex", BindingIdentityStatus: "unique"}, {Name: "Synthetic B", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "synthetic-b", Type: "codex", BindingIdentityStatus: "unique"}}
	if err := db.Create(&identities).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entities.CPAAPIKey{APIKey: "synthetic-downstream", DisplayKey: "Synthetic caller"}).Error; err != nil {
		t.Fatal(err)
	}
	prices, catalog := newCatalogPricingService(t, db)
	var alias *string
	if baseline {
		alias = new("base")
		if _, err := prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "base", PricingStyle: "claude", PromptPricePer1M: 10, CompletionPricePer1M: 20, CacheReadPricePer1M: 2, CacheWritePricePer1M: 4, PriceMultiplier: new(.5)}); err != nil {
			t.Fatal(err)
		}
		if _, err := prices.ReplacePricingRules(ctx, servicedto.ReplacePricingRulesInput{Model: "base", Rules: []servicedto.PricingRuleInput{{Key: "service_tier", Value: "priority", Multiplier: new(2.0)}, {Key: "reasoning_effort", Value: "high", Multiplier: new(3.0)}}}); err != nil {
			t.Fatal(err)
		}
	}
	if len(requestAliases) > 0 {
		alias = new(requestAliases[0])
	}
	subjects := []string{}
	for _, identity := range identities {
		bound, err := prices.(service.PricingCredentialProvider).BindPricingCredential(ctx, identity.ID)
		if err != nil {
			t.Fatal(err)
		}
		subjects = append(subjects, bound.SubjectID)
	}
	day := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	now := day.Add(12*time.Hour + 10*time.Minute)
	events := []entities.UsageEvent{}
	for i, identity := range identities {
		events = append(events, entities.UsageEvent{EventKey: "fixed-" + identity.Identity, APIGroupKey: "synthetic-downstream", AuthIndex: identity.Identity, AuthType: "oauth", Provider: "codex", Model: "observed-model", ModelAlias: alias, ServiceTier: "priority", ReasoningEffort: "high", Timestamp: day.Add(12*time.Hour + time.Duration(i+1)*time.Minute), InputTokens: 3_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000, CacheCreationTokens: 1_000_000, TotalTokens: 4_000_000})
	}
	if _, _, err := repository.InsertUsageEvents(db, events); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, db, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	return credentialDefaultFixture{db: db, prices: prices, defaults: prices.(service.PricingCredentialDefaultsProvider), catalog: catalog, subjectID: subjects[0], events: events, start: day, end: day.Add(20 * time.Hour), now: now}, subjects[1]
}

func attachFixedChannels(t *testing.T, base credentialDefaultFixture, second string) channelFixture {
	t.Helper()
	f := channelFixture{credentialDefaultFixture: base, channels: base.prices.(service.PricingChannelsProvider), subjects: []string{base.subjectID, second}}
	for i, id := range f.subjects {
		name := "Synthetic fixed channel A"
		if i == 1 {
			name = "Synthetic fixed channel B"
		}
		channel, err := f.channels.CreatePricingChannel(context.Background(), service.PricingChannelInput{Name: name, MemberSubjectIDs: []string{id}})
		if err != nil {
			t.Fatal(err)
		}
		f.ids = append(f.ids, channel.ID)
	}
	f.set(t, f.ids[0], ".2")
	f.set(t, f.ids[1], ".5")
	return f
}

func TestCredentialFixedChannelComposedPersistedPrecedenceClearAllCostFamilies(t *testing.T) {
	base, second := newCredentialFixedFixture(t, true)
	f := attachFixedChannels(t, base, second)
	ctx := context.Background()
	fixed := f.prices.(service.PricingCredentialFixedProvider)
	models := f.prices.(service.PricingCredentialModelsProvider)
	if _, err := f.defaults.SetCredentialDefault(ctx, f.subjectID, ".3"); err != nil {
		t.Fatal(err)
	}
	for model, factor := range map[string]float64{"observed-model": 1, "base": 3} {
		if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, model, syntheticFixed(factor, "openai")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fixed.SetCredentialFixed(ctx, second, "observed-model", syntheticFixed(2, "claude")); err != nil {
		t.Fatal(err)
	}
	assertStage := func(scope, mode, selected, by string, first, other float64) {
		t.Helper()
		f.assertCostFamilies(t, first+other, true)
		f.assertChannels(t, map[string]float64{f.ids[0]: first, f.ids[1]: other})
		page, err := service.NewUsageService(f.db, f.catalog).ListUsageEvents(ctx, servicedto.UsageFilter{StartTime: &f.start, EndTime: &f.end, EndExclusive: true})
		if err != nil || len(page.Events) != 2 {
			t.Fatalf("composed details %+v %v", page, err)
		}
		for _, event := range page.Events {
			if event.AuthIndex != "synthetic-a" {
				continue
			}
			if !event.CostAvailable || event.ChannelID != f.ids[0] || event.ChannelName != "Synthetic fixed channel A" || event.AttributionWarning != "" {
				t.Fatalf("lost exact channel evidence %+v", event)
			}
			closeCost(t, event.CostUSD, first)
			if scope != "" {
				selection := event.PricingSelection
				if selection == nil || selection.Scope != scope || selection.Mode != mode || selection.SelectedModel != selected || selection.SelectedBy != by || selection.BaselineModel != "base" || selection.BaselineBy != "model_alias" || !selection.BaselineAvailable || selection.BaselineCostUSD == nil {
					t.Fatalf("mixed scoped/baseline selection %+v", selection)
				}
				closeCost(t, *selection.BaselineCostUSD, 36)
				if mode == "fixed" && (selection.Fixed == nil || selection.Multiplier != nil || selection.PricingStyle != "claude") {
					t.Fatalf("stacked fixed/style %+v", selection)
				}
			}
		}
	}
	assertStage("credential_model", "fixed", "observed-model", "model", 10, 20)
	// Recover fixed rows, defaults, channel membership and tariffs together.
	f.prices, f.catalog = newCatalogPricingService(t, f.db)
	f.channels = f.prices.(service.PricingChannelsProvider)
	f.defaults = f.prices.(service.PricingCredentialDefaultsProvider)
	models = f.prices.(service.PricingCredentialModelsProvider)
	assertStage("credential_model", "fixed", "observed-model", "model", 10, 20)
	if _, err := models.ClearCredentialModel(ctx, f.subjectID, "observed-model"); err != nil {
		t.Fatal(err)
	}
	assertStage("credential_model", "fixed", "base", "model_alias", 30, 20)
	if _, err := models.ClearCredentialModel(ctx, f.subjectID, "base"); err != nil {
		t.Fatal(err)
	}
	assertStage("credential_default", "multiplier", "", "", 10.8, 20)
	if _, err := f.defaults.ClearCredentialDefault(ctx, f.subjectID); err != nil {
		t.Fatal(err)
	}
	assertStage("channel_default", "multiplier", "", "", 7.2, 20)
	if _, err := f.channels.ClearChannelDefault(ctx, f.ids[0]); err != nil {
		t.Fatal(err)
	}
	assertStage("legacy", "legacy", "", "", 108, 20)
	if _, err := models.ClearCredentialModel(ctx, second, "observed-model"); err != nil {
		t.Fatal(err)
	}
	assertStage("legacy", "legacy", "", "", 108, 18)
	if _, err := f.channels.ClearChannelDefault(ctx, f.ids[1]); err != nil {
		t.Fatal(err)
	}
	assertStage("", "", "", "", 108, 108)
}

func TestCredentialFixedChannelNoBaselineHighestMultiplierNeverDowngrades(t *testing.T) {
	base, second := newCredentialFixedFixture(t, false, "unpriced-alias")
	f := attachFixedChannels(t, base, second)
	ctx := context.Background()
	fixed := f.prices.(service.PricingCredentialFixedProvider)
	models := f.prices.(service.PricingCredentialModelsProvider)
	if _, err := f.defaults.SetCredentialDefault(ctx, f.subjectID, ".3"); err != nil {
		t.Fatal(err)
	}
	for model, factor := range map[string]float64{"observed-model": 1, "unpriced-alias": 3} {
		if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, model, syntheticFixed(factor, "openai")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fixed.SetCredentialFixed(ctx, second, "observed-model", syntheticFixed(2, "claude")); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 30, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 10, f.ids[1]: 20})
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
			comp, err := usage.(service.UsageComparisonProvider).GetUsageOverviewComparisons(ctx, filter)
			if err != nil || len(comp.Comparisons.Channels) != 2 {
				t.Fatalf("partial channel comparison %+v %v", comp, err)
			}
			a, b := comp.Comparisons.Channels[f.ids[0]], comp.Comparisons.Channels[f.ids[1]]
			if a == nil || b == nil || a.CostAvailable || !b.CostAvailable {
				t.Fatalf("mixed channel availability %+v %+v", a, b)
			}
			closeCost(t, a.CostUSD, 0)
			closeCost(t, b.CostUSD, 20)
		}
		page, err := usage.ListUsageEvents(ctx, servicedto.UsageFilter{StartTime: &f.start, EndTime: &f.end, EndExclusive: true})
		if err != nil || len(page.Events) != 2 {
			t.Fatalf("partial details %+v %v", page, err)
		}
		for _, event := range page.Events {
			if event.AuthIndex != "synthetic-a" {
				continue
			}
			selection := event.PricingSelection
			if event.CostAvailable || event.CostUSD != 0 || event.ChannelID != f.ids[0] || selection == nil || selection.Scope != scope || selection.Mode != "multiplier" || selection.BaselineAvailable || selection.BaselineCostUSD != nil || selection.UnavailableReason != "missing_baseline" {
				t.Fatalf("downgraded missing baseline %+v %+v", event, selection)
			}
		}
		window, err := repository.SumUsageWindowStatsByAuthIndex(ctx, f.db, "synthetic-a", f.start, &f.end, f.catalog.NewResolver())
		if err != nil || window.CostAvailable || window.Cost != 0 {
			t.Fatalf("downgraded window %+v %v", window, err)
		}
	}
	for _, text := range []string{".4", "0"} {
		if _, err := models.SetCredentialModel(ctx, f.subjectID, "observed-model", text); err != nil {
			t.Fatal(err)
		}
		assertPartial("credential_model")
	}
	if _, err := models.ClearCredentialModel(ctx, f.subjectID, "observed-model"); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 50, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 30, f.ids[1]: 20})
	if _, err := models.ClearCredentialModel(ctx, f.subjectID, "unpriced-alias"); err != nil {
		t.Fatal(err)
	}
	assertPartial("credential_default")
	if _, err := f.defaults.ClearCredentialDefault(ctx, f.subjectID); err != nil {
		t.Fatal(err)
	}
	assertPartial("channel_default")
	if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "observed-model", syntheticFixed(0, "openai")); err != nil {
		t.Fatal(err)
	}
	for _, id := range f.ids {
		if _, err := f.channels.ClearChannelDefault(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	f.prices, f.catalog = newCatalogPricingService(t, f.db)
	// Fixed-only configurations remain known despite missing baseline and cleared
	// lower defaults; the relocated evidence guards must include fixed mode.
	f.assertCostFamilies(t, 20, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 0, f.ids[1]: 20})
}

func TestCredentialFixedTwoCredentialsAllCostFamiliesRestartModesClear(t *testing.T) {
	f, b := newCredentialFixedFixture(t, true)
	ctx := context.Background()
	fixed := f.prices.(service.PricingCredentialFixedProvider)
	models := f.prices.(service.PricingCredentialModelsProvider)
	legacy := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	for _, id := range []string{f.subjectID, b} {
		if _, err := f.defaults.SetCredentialDefault(ctx, id, ".3"); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range []string{f.subjectID, b} {
		saved, err := fixed.SetCredentialFixed(ctx, id, " observed-model ", syntheticFixed(float64(i+1), "openai"))
		if err != nil {
			t.Fatal(err)
		}
		if saved.Mode != "fixed" || saved.Multiplier != nil || saved.Fixed == nil {
			t.Fatalf("save %+v", saved)
		}
		// A returned pointer must never modify the pinned catalog.
		*saved.Fixed.PromptPricePer1M = 999
	}
	f.assertCostFamilies(t, 30, true)
	for i, event := range f.events {
		result := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(event))
		closeCost(t, result.Cost.TotalCostUSD, float64(10*(i+1)))
		if result.PricingStyle != "claude" || result.SelectedModel != "observed-model" || result.MatchedModel != "base" || result.MatchedBy != "model_alias" || !result.ReferenceAvailable {
			t.Fatalf("style/independent matching %+v", result)
		}
		closeCost(t, result.ReferenceCost.TotalCostUSD, 36)
		if result.Multiplier != nil || result.RuleMultiplier != 1 {
			t.Fatal("stacked multiplier")
		}
	}
	list, err := models.ListCredentialModels(ctx, f.subjectID)
	if err != nil || len(list.Models) != 1 || list.Models[0].Mode != "fixed" {
		t.Fatalf("list %+v %v", list, err)
	}
	f.prices, f.catalog = newCatalogPricingService(t, f.db)
	fixed = f.prices.(service.PricingCredentialFixedProvider)
	models = f.prices.(service.PricingCredentialModelsProvider)
	f.defaults = f.prices.(service.PricingCredentialDefaultsProvider)
	f.assertCostFamilies(t, 30, true)
	if _, err := models.SetCredentialModel(ctx, f.subjectID, "observed-model", ".2"); err != nil {
		t.Fatal(err)
	}
	read, _ := models.GetCredentialModel(ctx, f.subjectID, "observed-model")
	if read.Mode != "multiplier" || read.Fixed != nil {
		t.Fatalf("inactive tariff %+v", read)
	}
	f.assertCostFamilies(t, 27.2, true)
	if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "observed-model", syntheticFixed(1, "claude")); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 30, true)
	if _, err := models.ClearCredentialModel(ctx, f.subjectID, "observed-model"); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 30.8, true)
	if _, err := models.ClearCredentialModel(ctx, b, "observed-model"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{f.subjectID, b} {
		if _, err := f.defaults.ClearCredentialDefault(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	f.assertCostFamilies(t, 216, true)
	if result := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0])); !reflect.DeepEqual(legacy, result) {
		t.Fatalf("legacy not restored %+v %+v", legacy, result)
	}
}

func TestCredentialFixedNoBaselineZeroPartialQuotaAndRetention(t *testing.T) {
	f, b := newCredentialFixedFixture(t, false)
	ctx := context.Background()
	fixed := f.prices.(service.PricingCredentialFixedProvider)
	models := f.prices.(service.PricingCredentialModelsProvider)
	for i, id := range []string{f.subjectID, b} {
		if _, err := fixed.SetCredentialFixed(ctx, id, "observed-model", syntheticFixed(float64(i+1), "openai")); err != nil {
			t.Fatal(err)
		}
	}
	f.assertCostFamilies(t, 30, true)
	result := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	selection := f.catalog.NewResolver().Selection(result)
	if !result.Available || result.ReferenceAvailable || selection.BaselineCostUSD != nil || selection.BaselineUnavailableReason != "missing_baseline" {
		t.Fatalf("invented reference %+v %+v", result, selection)
	}
	cycle := entities.QuotaCycle{Provider: "codex", AuthIndex: "synthetic-a", QuotaKey: "rate_limit.primary_window", WindowSeconds: 5 * 60 * 60, ResetAtSource: entities.QuotaResetAtSourceAbsolute, WindowStartedAt: f.start.Add(10 * time.Hour), ResetAt: f.start.Add(15 * time.Hour), FirstObservedAt: f.start.Add(10 * time.Hour), LastObservedAt: f.now, CreatedAt: f.start, UpdatedAt: f.start}
	if err := f.db.Create(&cycle).Error; err != nil {
		t.Fatal(err)
	}
	quota, err := repository.BuildCodexQuotaEfficiencyHistory(ctx, f.db, repodto.CodexQuotaEfficiencyQuery{Provider: "codex", AuthIndex: "synthetic-a", Now: f.now, RangeStart: f.start}, f.catalog.NewResolver())
	if err != nil || len(quota.Cycles) != 1 || !quota.Cycles[0].Usage.CostAvailable {
		t.Fatalf("quota %+v %v", quota, err)
	}
	closeCost(t, quota.Cycles[0].Usage.TotalCostUSD, 10)
	if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "observed-model", syntheticFixed(0, "claude")); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 20, true)
	empty := repository.UsageEventCostSubject(f.events[0])
	empty.Tokens = helper.UsageTokenCostInput{}
	if result := f.catalog.NewResolver().Calculate(empty); !result.Available || !result.ReferenceAvailable || result.Cost.TotalCostUSD != 0 {
		t.Fatalf("empty %+v", result)
	}
	// A highest exact multiplier wins over a lower alias fixed tariff even when 0.
	if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "alias-with-fixed", syntheticFixed(1, "openai")); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{".3", "0"} {
		if _, err := models.SetCredentialModel(ctx, f.subjectID, "observed-model", text); err != nil {
			t.Fatal(err)
		}
		subject := repository.UsageEventCostSubject(f.events[0])
		subject.Dimensions.ModelAlias = "alias-with-fixed"
		result := f.catalog.NewResolver().Calculate(subject)
		if result.Available || result.Mode != "multiplier" || result.UnavailableReason != "missing_baseline" {
			t.Fatalf("downgrade/free %+v", result)
		}
		usage := service.NewUsageService(f.db, f.catalog)
		filter := servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &f.start, EndTime: &f.end, EndExclusive: true}
		overview, err := usage.GetUsageOverview(ctx, filter)
		if err != nil || overview.Summary.CostAvailable {
			t.Fatalf("partial %+v %v", overview, err)
		}
		closeCost(t, overview.Summary.TotalCost, 20)
		analysis, err := usage.GetAnalysis(ctx, filter)
		if err != nil || analysis.CostBreakdown.CostAvailable {
			t.Fatalf("partial analysis %+v %v", analysis, err)
		}
		closeCost(t, analysis.CostBreakdown.TotalCostUSD, 20)
	}
	if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "observed-model", syntheticFixed(1, "openai")); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Where("event_key = ?", f.events[0].EventKey).Delete(&entities.UsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	overview, err := service.NewUsageService(f.db, f.catalog).GetUsageOverview(ctx, servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &f.start, EndTime: &f.end, EndExclusive: true})
	if err != nil || overview.Summary.CostAvailable || overview.Summary.UnavailableReason != "retained_pricing_evidence_incomplete" {
		t.Fatalf("missing evidence %+v %v", overview, err)
	}
}

func TestCredentialFixedInvalidStyleFutureBaselineAndCommitAtomic(t *testing.T) {
	f, _ := newCredentialFixedFixture(t, false)
	ctx := context.Background()
	fixed := f.prices.(service.PricingCredentialFixedProvider)
	if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "observed-model", syntheticFixed(1, "openai")); err != nil {
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
		if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "observed-model", value); !errors.Is(err, service.ErrInvalidPricingInput) {
			t.Fatalf("accepted %+v %v", value, err)
		}
		if f.catalog.Snapshot() != previous {
			t.Fatal("invalid publication")
		}
	}
	if _, err := f.prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "future", PromptPricePer1M: math.MaxFloat64, PriceMultiplier: new(0.0)}); !errors.Is(err, service.ErrInvalidPricingInput) {
		t.Fatalf("unsafe reference %v", err)
	}
	if f.catalog.Snapshot() != previous {
		t.Fatal("unsafe baseline publication")
	}
	if err := f.db.Exec(`CREATE TABLE fixed_commit_probe (subject_id TEXT, FOREIGN KEY(subject_id) REFERENCES credential_pricing_subjects(id) DEFERRABLE INITIALLY DEFERRED)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`CREATE TRIGGER fail_fixed_commit AFTER UPDATE ON credential_model_multipliers BEGIN INSERT INTO fixed_commit_probe(subject_id) VALUES ('absent-subject'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "observed-model", syntheticFixed(2, "claude")); err == nil {
		t.Fatal("COMMIT should fail")
	}
	if f.catalog.Snapshot() != previous {
		t.Fatal("failed commit published")
	}
	restarted, _ := newCatalogPricingService(t, f.db)
	read, _ := restarted.(service.PricingCredentialModelsProvider).GetCredentialModel(ctx, f.subjectID, "observed-model")
	if read.Mode != "fixed" || *read.Fixed.PromptPricePer1M != 1 {
		t.Fatalf("rollback persisted %+v", read)
	}
}

func TestCredentialFixedStyleIndependentMatchingAndFutureLoss(t *testing.T) {
	f, _ := newCredentialFixedFixture(t, true)
	ctx := context.Background()
	fixed := f.prices.(service.PricingCredentialFixedProvider)
	if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "base", syntheticFixed(1, "")); err != nil {
		t.Fatal(err)
	}
	previous := f.catalog.Snapshot()
	if err := f.prices.DeletePricing(ctx, "base"); !errors.Is(err, repository.ErrInvalidPricingSnapshot) {
		t.Fatalf("loss without explicit fallback %v", err)
	}
	if f.catalog.Snapshot() != previous {
		t.Fatal("invalid future loss published")
	}
	if _, err := f.prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "request", PricingStyle: "openai", PromptPricePer1M: 5}); err != nil {
		t.Fatal(err)
	}
	subject := repository.UsageEventCostSubject(f.events[0])
	subject.Dimensions.Model = "request"
	result := f.catalog.NewResolver().Calculate(subject)
	if result.SelectedModel != "base" || result.SelectedBy != "model_alias" || result.MatchedModel != "request" || result.PricingStyle != "openai" {
		t.Fatalf("baseline rewritten %+v", result)
	}
	closeCost(t, result.Cost.TotalCostUSD, 10)
	closeCost(t, result.ReferenceCost.TotalCostUSD, 5)
	if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "base", syntheticFixed(1, "claude")); err != nil {
		t.Fatal(err)
	}
	if err := f.prices.DeletePricing(ctx, "base"); err != nil {
		t.Fatal(err)
	}
	if err := f.prices.DeletePricing(ctx, "request"); err != nil {
		t.Fatal(err)
	}
	result = f.catalog.NewResolver().Calculate(subject)
	if !result.Available || result.ReferenceAvailable || result.PricingStyle != "claude" {
		t.Fatalf("explicit fallback %+v", result)
	}
}

func TestCredentialFixedPinnedConcurrencyAndPerEventCacheClamping(t *testing.T) {
	f := newCredentialDefaultFixture(t)
	ctx := context.Background()
	fixed := f.prices.(service.PricingCredentialFixedProvider)
	if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "observed-model", syntheticFixed(1, "openai")); err != nil {
		t.Fatal(err)
	}
	// A: ordinary1.7M/output.1M/read.4M/write.1M = 1.7+.2+1.2+.4=3.5;
	// B remains legacy29.4. Per-row clamp must not lose .1M ordinary input.
	f.assertCostFamilies(t, 32.9, true)
	old := f.catalog.NewResolver()
	var wg sync.WaitGroup
	for _, factor := range []float64{2, 3, 4} {
		wg.Go(func() {
			if _, err := fixed.SetCredentialFixed(ctx, f.subjectID, "observed-model", syntheticFixed(factor, "claude")); err != nil {
				t.Error(err)
			}
		})
	}
	for range 4 {
		wg.Go(func() {
			for range 30 {
				resolver := f.catalog.NewResolver()
				first := resolver.Calculate(repository.UsageEventCostSubject(f.events[0]))
				second := resolver.Calculate(repository.UsageEventCostSubject(f.events[0]))
				if !reflect.DeepEqual(first, second) || math.Abs(first.Cost.TotalCostUSD-1.9**first.Fixed.PromptPricePer1M) > 1e-9 || first.Mode != "fixed" {
					t.Error("mixed candidate/explanation")
				}
			}
		})
	}
	wg.Wait()
	closeCost(t, old.Calculate(repository.UsageEventCostSubject(f.events[0])).Cost.TotalCostUSD, 1.9)
	restarted, catalog := newCatalogPricingService(t, f.db)
	read, _ := restarted.(service.PricingCredentialModelsProvider).GetCredentialModel(ctx, f.subjectID, "observed-model")
	result := catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	closeCost(t, result.Cost.TotalCostUSD, 1.9**read.Fixed.PromptPricePer1M)
}

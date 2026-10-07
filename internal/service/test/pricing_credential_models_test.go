package test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/repository"
	repodto "cpa-usage-keeper/internal/repository/dto"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

func TestCredentialModelOnlyRealServiceAllCostFamiliesClearRestart(t *testing.T) {
	f := newCredentialDefaultFixture(t)
	ctx := context.Background()
	models := f.prices.(service.PricingCredentialModelsProvider)
	legacy := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	for _, text := range []string{"0.2", " 0.2x ", "20%", ".2X"} {
		saved, err := models.SetCredentialModel(ctx, f.subjectID, " observed-model ", text)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Model != "observed-model" || saved.Multiplier == nil || *saved.Multiplier != .2 || saved.SnapshotID == "" {
			t.Fatalf("save %+v", saved)
		}
		// No credential default exists: every projection/retention guard must
		// activate on a MODEL-ONLY override, including per-event cache clamping.
		if f.catalog.NewResolver().HasCredentialDefaults() {
			t.Fatal("fixture accidentally contains default")
		}
		f.assertCostFamilies(t, 33.44, true)
	}
	list, err := models.ListCredentialModels(ctx, f.subjectID)
	if err != nil || len(list.Models) != 1 {
		t.Fatalf("unique list %+v %v", list, err)
	}
	page, err := service.NewUsageService(f.db, f.catalog).ListUsageEvents(ctx, servicedto.UsageFilter{StartTime: &f.start, EndTime: &f.end, EndExclusive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range page.Events {
		if event.AuthIndex != "synthetic-a" {
			continue
		}
		if event.PricingSelection == nil || event.PricingSelection.Scope != "credential_model" || event.PricingSelection.SelectedModel != "observed-model" || event.PricingSelection.SelectedBy != "model" || event.PricingSelection.BaselineModel != "base" || event.PricingSelection.BaselineBy != "model_alias" {
			t.Fatalf("external selection %+v", event.PricingSelection)
		}
	}
	f.prices, f.catalog = newCatalogPricingService(t, f.db)
	models = f.prices.(service.PricingCredentialModelsProvider)
	read, err := models.GetCredentialModel(ctx, f.subjectID, "observed-model")
	if err != nil || read.Multiplier == nil || *read.Multiplier != .2 {
		t.Fatalf("restart %+v %v", read, err)
	}
	f.assertCostFamilies(t, 33.44, true)
	if _, err := models.ClearCredentialModel(ctx, f.subjectID, "observed-model"); err != nil {
		t.Fatal(err)
	}
	restored := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if !reflect.DeepEqual(legacy, restored) {
		t.Fatalf("legacy changed %+v %+v", legacy, restored)
	}
}

func TestCredentialModelDefaultOtherAndFutureNExplicitZeroOne(t *testing.T) {
	f := newCredentialDefaultFixture(t)
	ctx := context.Background()
	models := f.prices.(service.PricingCredentialModelsProvider)
	if _, err := f.defaults.SetCredentialDefault(ctx, f.subjectID, ".3"); err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"M", "other"} {
		if _, err := f.prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: model, PromptPricePer1M: 10, PriceMultiplier: new(.5)}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.prices.ReplacePricingRules(ctx, servicedto.ReplacePricingRulesInput{Model: model, Rules: []servicedto.PricingRuleInput{{Key: "service_tier", Value: "priority", Multiplier: new(2.0)}, {Key: "reasoning_effort", Value: "high", Multiplier: new(3.0)}}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := models.SetCredentialModel(ctx, f.subjectID, "M", "1.2x"); err != nil {
		t.Fatal(err)
	}
	// N arrives after both agreements; no copied per-model default is needed.
	if _, err := f.prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "N", PromptPricePer1M: 10, PriceMultiplier: new(0.0)}); err != nil {
		t.Fatal(err)
	}
	events := []entities.UsageEvent{}
	for _, model := range []string{"M", "other", "N"} {
		event := f.events[0]
		event.ID, event.EventKey, event.Model, event.ModelAlias = 0, "model-acceptance-"+model, model, nil
		event.InputTokens, event.OutputTokens, event.CacheReadTokens, event.CacheCreationTokens, event.TotalTokens = 1_000_000, 0, 0, 0, 1_000_000
		events = append(events, event)
	}
	if _, _, err := repository.InsertUsageEvents(f.db, events); err != nil {
		t.Fatal(err)
	}
	usage := service.NewUsageService(f.db, f.catalog)
	readCost := func(model string) float64 {
		t.Helper()
		page, err := usage.ListUsageEvents(ctx, servicedto.UsageFilter{StartTime: &f.start, EndTime: &f.end, EndExclusive: true, Model: model})
		if err != nil || len(page.Events) != 1 || !page.Events[0].CostAvailable {
			t.Fatalf("detail %+v %v", page, err)
		}
		return page.Events[0].CostUSD
	}
	closeCost(t, readCost("M"), 12)
	closeCost(t, readCost("other"), 3)
	closeCost(t, readCost("N"), 3)
	for _, test := range []struct {
		text string
		cost float64
	}{{"0x", 0}, {"1", 10}, {"120%", 12}, {"1.2X", 12}} {
		if _, err := models.SetCredentialModel(ctx, f.subjectID, "M", test.text); err != nil {
			t.Fatal(err)
		}
		closeCost(t, readCost("M"), test.cost)
	}
	if _, err := models.ClearCredentialModel(ctx, f.subjectID, "M"); err != nil {
		t.Fatal(err)
	}
	closeCost(t, readCost("M"), 3)
	if _, err := f.defaults.ClearCredentialDefault(ctx, f.subjectID); err != nil {
		t.Fatal(err)
	}
	closeCost(t, readCost("M"), 30)
	closeCost(t, readCost("N"), 0) // restores exact legacy zero
}

func TestCredentialModelSelectionIndependentBaselineAndNoDowngrade(t *testing.T) {
	f := newCredentialDefaultFixture(t)
	ctx := context.Background()
	models := f.prices.(service.PricingCredentialModelsProvider)
	if _, err := f.defaults.SetCredentialDefault(ctx, f.subjectID, ".3"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "M", PromptPricePer1M: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "A", PromptPricePer1M: 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := models.SetCredentialModel(ctx, f.subjectID, "A", ".2"); err != nil {
		t.Fatal(err)
	}
	subject := repository.UsageEventCostSubject(f.events[0])
	subject.Tokens = helper.UsageTokenCostInput{InputTokens: 1_000_000}
	subject.Dimensions.Model, subject.Dimensions.ModelAlias = "M", "A"
	result := f.catalog.NewResolver().Calculate(subject)
	closeCost(t, result.Cost.TotalCostUSD, 2)
	if result.SelectedModel != "A" || result.SelectedBy != "model_alias" || result.MatchedModel != "M" || result.MatchedBy != "model" {
		t.Fatalf("independent lookup %+v", result)
	}
	if _, err := models.SetCredentialModel(ctx, f.subjectID, "M", "1.2"); err != nil {
		t.Fatal(err)
	}
	result = f.catalog.NewResolver().Calculate(subject)
	closeCost(t, result.Cost.TotalCostUSD, 12)
	if result.SelectedModel != "M" || result.SelectedBy != "model" {
		t.Fatalf("both candidates %+v", result)
	}
	// The selected model need not itself have a price; the established alias
	// baseline remains usable. Scope selection cannot follow baseline presence.
	if err := f.prices.DeletePricing(ctx, "M"); err != nil {
		t.Fatal(err)
	}
	result = f.catalog.NewResolver().Calculate(subject)
	closeCost(t, result.Cost.TotalCostUSD, 24)
	if result.SelectedModel != "M" || result.MatchedModel != "A" || result.MatchedBy != "model_alias" {
		t.Fatalf("alias baseline %+v", result)
	}
	if err := f.prices.DeletePricing(ctx, "A"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{".3", "0"} {
		if _, err := models.SetCredentialModel(ctx, f.subjectID, "M", value); err != nil {
			t.Fatal(err)
		}
		result = f.catalog.NewResolver().Calculate(subject)
		if result.Available || result.Scope != "credential_model" || result.SelectedModel != "M" || result.UnavailableReason != "missing_baseline" {
			t.Fatalf("downgrade/free %+v", result)
		}
	}
	subject.Tokens = helper.UsageTokenCostInput{}
	if result := f.catalog.NewResolver().Calculate(subject); !result.Available || result.Cost.TotalCostUSD != 0 {
		t.Fatalf("empty %+v", result)
	}
	// Exact matching, no casefold or inferred alias.
	subject.Dimensions.Model, subject.Dimensions.ModelAlias = "m", "a"
	if result := f.catalog.NewResolver().Calculate(subject); result.Scope != "credential_default" || result.SelectedModel != "" {
		t.Fatalf("fuzzy match %+v", result)
	}
}

func TestCredentialModelInvalidUnsafeFutureBaselineAndCommitFailuresAtomic(t *testing.T) {
	f := newCredentialDefaultFixture(t)
	ctx := context.Background()
	models := f.prices.(service.PricingCredentialModelsProvider)
	if _, err := models.SetCredentialModel(ctx, f.subjectID, "observed-model", ".2"); err != nil {
		t.Fatal(err)
	}
	previous := f.catalog.Snapshot()
	for _, text := range []string{"", " ", "-1", "+1", "NaN", "Infinity", "1e2", "20%x", "1 x", "broken", strconv.FormatFloat(math.MaxFloat64, 'f', -1, 64)} {
		if _, err := models.SetCredentialModel(ctx, f.subjectID, "observed-model", text); !errors.Is(err, service.ErrInvalidPricingInput) {
			t.Fatalf("accepted %q %v", text, err)
		}
		if f.catalog.Snapshot() != previous {
			t.Fatal("invalid publication")
		}
	}
	if _, err := f.prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "future-zero", PromptPricePer1M: math.MaxFloat64, PriceMultiplier: new(0.0)}); !errors.Is(err, service.ErrInvalidPricingInput) {
		t.Fatalf("zero baseline safety %v", err)
	}
	if f.catalog.Snapshot() != previous {
		t.Fatal("unsafe baseline published")
	}
	if err := f.db.Exec(`CREATE TABLE model_commit_probe (subject_id TEXT, FOREIGN KEY(subject_id) REFERENCES credential_pricing_subjects(id) DEFERRABLE INITIALLY DEFERRED)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`CREATE TRIGGER fail_model_commit AFTER UPDATE ON credential_model_multipliers BEGIN INSERT INTO model_commit_probe(subject_id) VALUES ('absent-subject'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := models.SetCredentialModel(ctx, f.subjectID, "observed-model", ".7"); err == nil {
		t.Fatal("expected COMMIT failure")
	}
	if f.catalog.Snapshot() != previous {
		t.Fatal("failed COMMIT published")
	}
	persisted, _ := models.GetCredentialModel(ctx, f.subjectID, "observed-model")
	if persisted.Multiplier == nil || *persisted.Multiplier != .2 {
		t.Fatal("failed commit lost saved value")
	}
	if err := f.db.Exec(`DROP TRIGGER fail_model_commit`).Error; err != nil {
		t.Fatal(err)
	}
	// This saved exception can pair with ANY Model baseline when it matches
	// an alias, not only with a price row bearing its own model key.
	if _, err := models.SetCredentialModel(ctx, f.subjectID, "observed-model", strconv.FormatFloat(1e280, 'f', -1, 64)); err != nil {
		t.Fatal(err)
	}
	previous = f.catalog.Snapshot()
	if _, err := f.prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "different-future-baseline", PromptPricePer1M: 1e30, PriceMultiplier: new(0.0)}); !errors.Is(err, service.ErrInvalidPricingInput) {
		t.Fatalf("unsafe combination %v", err)
	}
	if f.catalog.Snapshot() != previous {
		t.Fatal("unsafe future candidate published")
	}
}

func TestCredentialModelConcurrentPinnedReadersAndPersistedCandidate(t *testing.T) {
	f := newCredentialDefaultFixture(t)
	ctx := context.Background()
	models := f.prices.(service.PricingCredentialModelsProvider)
	if _, err := models.SetCredentialModel(ctx, f.subjectID, "observed-model", ".2"); err != nil {
		t.Fatal(err)
	}
	old := f.catalog.NewResolver()
	var wg sync.WaitGroup
	for _, text := range []string{".3", ".4", ".5"} {
		wg.Go(func() {
			if _, err := models.SetCredentialModel(ctx, f.subjectID, "observed-model", text); err != nil {
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
				if !reflect.DeepEqual(first, second) {
					t.Error("mixed candidate")
				}
				if first.SelectedModel != "observed-model" || first.Cost.TotalCostUSD != 9.8**first.Multiplier {
					t.Error("mixed explanation")
				}
			}
		})
	}
	wg.Wait()
	closeCost(t, old.Calculate(repository.UsageEventCostSubject(f.events[0])).Cost.TotalCostUSD, 1.96)
	restarted, _ := newCatalogPricingService(t, f.db)
	current, _ := models.GetCredentialModel(ctx, f.subjectID, "observed-model")
	persisted, err := restarted.(service.PricingCredentialModelsProvider).GetCredentialModel(ctx, f.subjectID, "observed-model")
	if err != nil || !reflect.DeepEqual(current.Multiplier, persisted.Multiplier) {
		t.Fatalf("DB/publication mismatch %+v %+v %v", current, persisted, err)
	}
}

func TestCredentialModelOnlyTypedIdentityQuotaAndIncompleteHistory(t *testing.T) {
	f := newCredentialDefaultFixture(t)
	ctx := context.Background()
	models := f.prices.(service.PricingCredentialModelsProvider)
	if _, err := models.SetCredentialModel(ctx, f.subjectID, "observed-model", ".2"); err != nil {
		t.Fatal(err)
	}
	identity := entities.UsageIdentity{Name: "Synthetic API", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-a", Type: "openai", BindingIdentityStatus: "unique"}
	if err := f.db.Create(&identity).Error; err != nil {
		t.Fatal(err)
	}
	bound, err := f.prices.(service.PricingCredentialProvider).BindPricingCredential(ctx, identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := models.SetCredentialModel(ctx, bound.SubjectID, "observed-model", ".4"); err != nil {
		t.Fatal(err)
	}
	event := f.events[0]
	event.ID, event.EventKey, event.AuthType = 0, "model-mixed-typed", "apikey"
	if _, _, err := repository.InsertUsageEvents(f.db, []entities.UsageEvent{event}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, f.db, f.end); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 37.36, true)
	usage := service.NewUsageService(f.db, f.catalog)
	filter := servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &f.start, EndTime: &f.end, EndExclusive: true}
	analysis, err := usage.GetAnalysis(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.AIProviderComposition) != 1 || len(analysis.AuthFilesComposition) != 2 {
		t.Fatalf("typed composition %+v", analysis)
	}
	closeCost(t, analysis.AIProviderComposition[0].CostUSD, 3.92)
	for _, item := range analysis.AuthFilesComposition {
		if item.Key == "synthetic-a" {
			closeCost(t, item.CostUSD, 4.04)
		}
	}
	cycle := entities.QuotaCycle{Provider: "codex", AuthIndex: "synthetic-a", QuotaKey: "rate_limit.primary_window", WindowSeconds: 5 * 60 * 60, ResetAtSource: entities.QuotaResetAtSourceAbsolute, WindowStartedAt: f.start.Add(10 * time.Hour), ResetAt: f.start.Add(15 * time.Hour), FirstObservedAt: f.start.Add(10 * time.Hour), LastObservedAt: f.now, CreatedAt: f.start, UpdatedAt: f.start}
	if err := f.db.Create(&cycle).Error; err != nil {
		t.Fatal(err)
	}
	history, err := repository.BuildCodexQuotaEfficiencyHistory(ctx, f.db, repodto.CodexQuotaEfficiencyQuery{Provider: "codex", AuthIndex: "synthetic-a", Now: f.now, RangeStart: f.start}, f.catalog.NewResolver())
	if err != nil || len(history.Cycles) != 1 || !history.Cycles[0].Usage.CostAvailable {
		t.Fatalf("quota %+v %v", history, err)
	}
	closeCost(t, history.Cycles[0].Usage.TotalCostUSD, 4.04)
	// Pruned typed evidence is not recoverable from the type-less rollup.
	if err := f.db.Where("event_key = ?", event.EventKey).Delete(&entities.UsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	overview, err := usage.GetUsageOverview(ctx, filter)
	if err != nil || overview.Summary.CostAvailable || overview.Summary.UnavailableReason != "retained_pricing_evidence_incomplete" {
		t.Fatalf("guessed historical fee %+v %v", overview, err)
	}
}

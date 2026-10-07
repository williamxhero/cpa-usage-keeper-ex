package test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/providerconfig"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"gorm.io/gorm"
)

type channelFixture struct {
	credentialDefaultFixture
	channels service.PricingChannelsProvider
	ids      []string
	subjects []string
}

func newChannelFixture(t *testing.T) channelFixture {
	t.Helper()
	ctx := context.Background()
	db := openUsageServiceTestDatabase(t)
	identities := []entities.UsageIdentity{
		{Name: "OpenAI A", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-a", Type: "openai", BindingIdentityStatus: "unique"},
		{Name: "OpenAI B", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-b", Type: "openai", BindingIdentityStatus: "unique"},
	}
	if err := db.Create(&identities).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entities.CPAAPIKey{APIKey: "synthetic-downstream", DisplayKey: "Synthetic caller"}).Error; err != nil {
		t.Fatal(err)
	}
	prices, catalog := newCatalogPricingService(t, db)
	half := .5
	if _, err := prices.UpdatePricing(ctx, servicedto.UpdatePricingInput{Model: "base", PromptPricePer1M: 10, CompletionPricePer1M: 20, CacheReadPricePer1M: 2, CacheWritePricePer1M: 4, PriceMultiplier: &half}); err != nil {
		t.Fatal(err)
	}
	if _, err := prices.ReplacePricingRules(ctx, servicedto.ReplacePricingRulesInput{Model: "base", Rules: []servicedto.PricingRuleInput{{Key: "service_tier", Value: "priority", Multiplier: new(2.0)}, {Key: "reasoning_effort", Value: "high", Multiplier: new(3.0)}}}); err != nil {
		t.Fatal(err)
	}
	provider := prices.(service.PricingChannelsProvider)
	f := channelFixture{channels: provider}
	for i, identity := range identities {
		subject, err := prices.(service.PricingCredentialProvider).BindPricingCredential(ctx, identity.ID)
		if err != nil {
			t.Fatal(err)
		}
		channel, err := provider.CreatePricingChannel(ctx, service.PricingChannelInput{Name: fmt.Sprintf("Channel %c", 'A'+i), MemberSubjectIDs: []string{subject.SubjectID}})
		if err != nil {
			t.Fatal(err)
		}
		f.subjects = append(f.subjects, subject.SubjectID)
		f.ids = append(f.ids, channel.ID)
	}
	day := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	now := day.Add(12*time.Hour + 10*time.Minute)
	alias := "base"
	events := []entities.UsageEvent{}
	for i, index := range []string{"synthetic-a", "synthetic-b"} {
		events = append(events, entities.UsageEvent{EventKey: fmt.Sprintf("channel-event-%d", i), APIGroupKey: "synthetic-downstream", AuthIndex: index, AuthType: "apikey", Provider: "openai", Model: "observed-model", ModelAlias: &alias, ServiceTier: "priority", ReasoningEffort: "high", Timestamp: day.Add(12*time.Hour + time.Duration(i+1)*time.Minute), InputTokens: 1_000_000, TotalTokens: 1_000_000})
	}
	if _, _, err := repository.InsertUsageEvents(db, events); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, db, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	f.credentialDefaultFixture = credentialDefaultFixture{db: db, prices: prices, defaults: prices.(service.PricingCredentialDefaultsProvider), catalog: catalog, subjectID: f.subjects[0], events: events, start: day, end: day.Add(20 * time.Hour), now: now}
	return f
}
func (f channelFixture) set(t *testing.T, id, text string) service.PricingChannel {
	t.Helper()
	value, err := f.channels.SetChannelDefault(context.Background(), id, text)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func (f channelFixture) assertChannels(t *testing.T, expected map[string]float64) {
	t.Helper()
	usage := service.NewUsageService(f.db, f.catalog)
	for _, unit := range []string{"hour", "day"} {
		end := f.end
		if unit == "day" {
			end = f.start.AddDate(0, 0, 1)
		}
		data, err := usage.(service.UsageComparisonProvider).GetUsageOverviewComparisons(context.Background(), servicedto.UsageFilter{Range: "custom", CustomUnit: unit, StartTime: &f.start, EndTime: &end, EndExclusive: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(data.Comparisons.Channels) != len(expected) {
			t.Fatalf("channels %+v, expected %v", data.Comparisons.Channels, expected)
		}
		for id, cost := range expected {
			row := data.Comparisons.Channels[id]
			if row == nil || !row.CostAvailable {
				t.Fatalf("channel %s incomplete %+v", id, row)
			}
			closeCost(t, row.CostUSD, cost)
		}
		if data.Comparisons.PricingSnapshotID != f.catalog.Snapshot().ID() {
			t.Fatal("comparison snapshot not pinned")
		}
	}
}
func TestChannelCredentialModelCombinedPersistedPrecedenceAllCostFamilies(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	f.set(t, f.ids[0], ".2")
	f.set(t, f.ids[1], ".5")
	if _, err := f.defaults.SetCredentialDefault(ctx, f.subjects[0], ".3"); err != nil {
		t.Fatal(err)
	}
	models := f.prices.(service.PricingCredentialModelsProvider)
	// Both Model and Alias have exceptions. Model wins independently of the
	// baseline, which exists only for Alias; none of the lower layers stack.
	for model, text := range map[string]string{"observed-model": ".4", "base": ".9"} {
		if _, err := models.SetCredentialModel(ctx, f.subjects[0], model, text); err != nil {
			t.Fatal(err)
		}
	}
	assertStage := func(scope, selectedModel, selectedBy string, first, second float64) {
		t.Helper()
		f.assertCostFamilies(t, first+second, true)
		f.assertChannels(t, map[string]float64{f.ids[0]: first, f.ids[1]: second})
		page, err := service.NewUsageService(f.db, f.catalog).ListUsageEvents(ctx, servicedto.UsageFilter{StartTime: &f.start, EndTime: &f.end, EndExclusive: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range page.Events {
			if event.AuthIndex != "synthetic-a" {
				continue
			}
			if event.ChannelID != f.ids[0] || event.ChannelName != "Channel A" || event.AttributionWarning != "" || event.PricingSnapshotID != f.catalog.Snapshot().ID() {
				t.Fatalf("lost channel evidence: %+v", event)
			}
			if scope != "" && (event.PricingSelection == nil || event.PricingSelection.Scope != scope || event.PricingSelection.SelectedModel != selectedModel || event.PricingSelection.SelectedBy != selectedBy || event.PricingSelection.BaselineModel != "base" || event.PricingSelection.BaselineBy != "model_alias") {
				t.Fatalf("wrong composed selection: %+v", event.PricingSelection)
			}
		}
	}
	assertStage("credential_model", "observed-model", "model", 4, 5)
	// Recover all three persisted override layers through the full loader.
	f.prices, f.catalog = newCatalogPricingService(t, f.db)
	f.channels = f.prices.(service.PricingChannelsProvider)
	f.defaults = f.prices.(service.PricingCredentialDefaultsProvider)
	models = f.prices.(service.PricingCredentialModelsProvider)
	read, err := models.GetCredentialModel(ctx, f.subjects[0], "observed-model")
	if err != nil || read.Multiplier == nil || *read.Multiplier != .4 {
		t.Fatalf("restarted model: %+v %v", read, err)
	}
	assertStage("credential_model", "observed-model", "model", 4, 5)
	if _, err := models.ClearCredentialModel(ctx, f.subjects[0], "observed-model"); err != nil {
		t.Fatal(err)
	}
	assertStage("credential_model", "base", "model_alias", 9, 5)
	if _, err := models.ClearCredentialModel(ctx, f.subjects[0], "base"); err != nil {
		t.Fatal(err)
	}
	assertStage("credential_default", "", "", 3, 5)
	if _, err := f.defaults.ClearCredentialDefault(ctx, f.subjects[0]); err != nil {
		t.Fatal(err)
	}
	assertStage("channel_default", "", "", 2, 5)
	for _, id := range f.ids {
		if _, err := f.channels.ClearChannelDefault(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	// Full old model multiplier and both old rules return only after clearing
	// every override; membership remains usable for named comparison evidence.
	assertStage("", "", "", 30, 30)
	if f.catalog.NewResolver().HasPricingOverrides() {
		t.Fatal("cleared configuration still reports an override")
	}
}

func TestChannelMetadataCommitReloadFailureSanitizesLabelsAndRecovers(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	f.set(t, f.ids[0], ".2")
	f.set(t, f.ids[1], ".5")
	old := f.catalog.Snapshot()
	fetcher := newMetadataTestFetcher()
	fetcher.managementAPIKeysResult.Payload.APIKeys = []string{"synthetic-downstream"}
	fetcher.openAIResult = &response.OpenAICompatibilityResult{StatusCode: 200, Payload: []providerconfig.OpenAICompatibilityConfig{{Name: "Fresh provider", APIKeyEntries: []providerconfig.OpenAIApiKeyEntry{{AuthIndex: "synthetic-a", APIKey: "Channel A"}, {AuthIndex: "synthetic-b", APIKey: "synthetic-new-key-b"}}}}}
	syncer := service.NewSyncServiceWithOptions(f.db, service.SyncServiceOptions{BaseURL: "https://cpa.example.invalid", MetadataFetcher: fetcher, Now: func() time.Time { return f.now }, PricingCatalog: f.catalog})
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	callbackName := "synthetic_channel_cancel_pricing_reload"
	if err := f.db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "credential_price_defaults" {
			cancel()
			tx.AddError(cancelCtx.Err())
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := syncer.SyncMetadata(cancelCtx); err == nil {
		t.Fatal("expected pricing reload failure after metadata commit")
	}
	if err := f.db.Callback().Query().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	var directory entities.UsageIdentity
	if err := f.db.First(&directory, "identity = ? AND auth_type = ?", "synthetic-a", entities.UsageIdentityAuthTypeAIProvider).Error; err != nil || directory.LookupKey != "Channel A" {
		t.Fatalf("metadata did not commit: %+v %v", directory, err)
	}
	read, err := f.channels.GetPricingChannel(ctx, f.ids[0])
	if err != nil || read.Name != "Channel" || read.Multiplier == nil || *read.Multiplier != .2 || len(read.MemberSubjectIDs) != 1 || read.MemberSubjectIDs[0] != f.subjects[0] {
		t.Fatalf("unsafe failed-reload channel: %+v %v", read, err)
	}
	list, err := f.channels.ListPricingChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, channel := range list {
		if channel.Name != "Channel" {
			t.Fatalf("unsafe failed-reload list: %+v", channel)
		}
	}
	if old.Channels()[0].Name == "Channel" {
		t.Fatal("fallback mutated a pinned snapshot")
	}
	cost := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if cost.Scope != "" || cost.ChannelID != "" || cost.AttributionWarning != "unresolved_identity" {
		t.Fatalf("fallback retained stale attribution: %+v", cost)
	}
	closeCost(t, cost.Cost.TotalCostUSD, 30)
	if err := syncer.SyncMetadata(ctx); err != nil {
		t.Fatal(err)
	}
	read, err = f.channels.GetPricingChannel(ctx, f.ids[0])
	if err != nil || read.Name != "Channel" {
		t.Fatalf("unsafe recovered label: %+v %v", read, err)
	}
	f.assertCostFamilies(t, 7, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 2, f.ids[1]: 5})
}

func TestChannelNamesExcludeKnownSecretsAndSanitizeRefreshedMetadata(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	fileName, filePath := "synthetic-file.json", "/private/synthetic-path.json"
	if err := f.db.Model(&entities.UsageIdentity{}).Where("identity = ?", "synthetic-a").Updates(map[string]any{
		"lookup_key": "synthetic-private-source-a", "file_name": fileName, "file_path": filePath,
		"base_url": "https://synthetic-user:synthetic-password@example.invalid?key=synthetic-query-secret",
	}).Error; err != nil {
		t.Fatal(err)
	}
	before := f.catalog.Snapshot().ID()
	for _, secret := range []string{"synthetic-private-source-a", "synthetic-a", "synthetic-file.json", "synthetic-file", "synthetic-path", "synthetic-user", "synthetic-password", "synthetic-query-secret"} {
		t.Run(secret, func(t *testing.T) {
			input := service.PricingChannelInput{Name: secret, MemberSubjectIDs: []string{}}
			if _, err := f.channels.CreatePricingChannel(ctx, input); !errors.Is(err, service.ErrInvalidPricingInput) {
				t.Fatalf("secret create accepted: %v", err)
			}
			if _, err := f.channels.UpdatePricingChannel(ctx, f.ids[0], input); !errors.Is(err, service.ErrInvalidPricingInput) {
				t.Fatalf("secret rename accepted: %v", err)
			}
		})
	}
	if f.catalog.Snapshot().ID() != before {
		t.Fatal("rejected names published a snapshot")
	}
	var count int64
	if err := f.db.Model(&entities.PricingChannel{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("rejected names changed DB: %d %v", count, err)
	}
	// A previously harmless label becomes known secret evidence after metadata refresh.
	if err := f.db.Model(&entities.UsageIdentity{}).Where("identity = ?", "synthetic-a").Update("lookup_key", "Channel A").Error; err != nil {
		t.Fatal(err)
	}
	f.set(t, f.ids[0], ".2")
	f.set(t, f.ids[1], ".5")
	readback, err := f.channels.GetPricingChannel(ctx, f.ids[0])
	if err != nil || readback.Name != "Channel" || readback.ID != f.ids[0] || len(readback.MemberSubjectIDs) != 1 || *readback.Multiplier != .2 {
		t.Fatalf("unsafe metadata publication: %+v %v", readback, err)
	}
	result := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if result.ChannelName != "Channel" || result.Cost.TotalCostUSD != 2 {
		t.Fatalf("unsafe event label: %+v", result)
	}
	snapshot, err := repository.LoadPricingSnapshot(ctx, f.db)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snapshot.Channels() {
		if item.ID == f.ids[0] && item.Name != "Channel" {
			t.Fatalf("unsafe restarted label: %+v", item)
		}
	}
	f.assertChannels(t, map[string]float64{f.ids[0]: 2, f.ids[1]: 5})
}

func TestChannelDefaultRealSaveAllCostFamiliesClearRenameRestart(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	for _, text := range []string{".2", "0.2x", "20%"} {
		saved := f.set(t, f.ids[0], text)
		if saved.Multiplier == nil || *saved.Multiplier != .2 {
			t.Fatalf("canonical %+v", saved)
		}
	}
	f.set(t, f.ids[1], ".5")
	f.assertCostFamilies(t, 7, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 2, f.ids[1]: 5})
	renamed, err := f.channels.UpdatePricingChannel(ctx, f.ids[0], service.PricingChannelInput{Name: "Renamed A", MemberSubjectIDs: []string{f.subjects[0]}})
	if err != nil || renamed.ID != f.ids[0] || *renamed.Multiplier != .2 {
		t.Fatalf("rename %+v %v", renamed, err)
	}
	if _, err := f.defaults.SetCredentialDefault(ctx, f.subjects[0], ".3"); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 8, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 3, f.ids[1]: 5})
	if _, err := f.defaults.ClearCredentialDefault(ctx, f.subjects[0]); err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 7, true)
	restarted, catalog := newCatalogPricingService(t, f.db)
	f.prices = restarted
	f.catalog = catalog
	f.channels = restarted.(service.PricingChannelsProvider)
	f.defaults = restarted.(service.PricingCredentialDefaultsProvider)
	readback, err := f.channels.GetPricingChannel(ctx, f.ids[0])
	if err != nil || readback.Name != "Renamed A" || readback.MemberSubjectIDs[0] != f.subjects[0] || *readback.Multiplier != .2 {
		t.Fatalf("restart %+v %v", readback, err)
	}
	f.assertCostFamilies(t, 7, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 2, f.ids[1]: 5})
	for _, id := range f.ids {
		cleared, err := f.channels.ClearChannelDefault(ctx, id)
		if err != nil || cleared.Multiplier != nil {
			t.Fatalf("clear %+v %v", cleared, err)
		}
	}
	f.assertCostFamilies(t, 60, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 30, f.ids[1]: 30})
	legacy := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if legacy.Scope != "" || legacy.RuleMultiplier != 6 || legacy.MatchedBy != "model_alias" {
		t.Fatalf("legacy metadata %+v", legacy)
	}
}
func TestChannelDefaultZeroOneMissingBaselineAndAllFourBuckets(t *testing.T) {
	f := newChannelFixture(t)
	subject := repository.UsageEventCostSubject(f.events[0])
	for _, value := range []struct {
		text string
		cost float64
	}{{"0", 0}, {"1", 10}, {"120%", 12}} {
		f.set(t, f.ids[0], value.text)
		result := f.catalog.NewResolver().Calculate(subject)
		closeCost(t, result.Cost.TotalCostUSD, value.cost)
		if result.Scope != "channel_default" || !result.Available {
			t.Fatalf("active %+v", result)
		}
	}
	f.set(t, f.ids[0], "0")
	subject.Dimensions.Model = "missing"
	subject.Dimensions.ModelAlias = ""
	result := f.catalog.NewResolver().Calculate(subject)
	if result.Available || result.UnavailableReason != "missing_baseline" {
		t.Fatalf("invented zero %+v", result)
	}
	subject.Tokens = helper.UsageTokenCostInput{}
	if result := f.catalog.NewResolver().Calculate(subject); !result.Available || result.Cost.TotalCostUSD != 0 {
		t.Fatalf("zero-billable %+v", result)
	}
	subject = repository.UsageEventCostSubject(f.events[0])
	subject.Tokens = helper.UsageTokenCostInput{InputTokens: 1_000_000, OutputTokens: 100_000, CacheReadTokens: 200_000, CacheCreationTokens: 100_000}
	f.set(t, f.ids[0], ".2")
	result = f.catalog.NewResolver().Calculate(subject)
	closeCost(t, result.Cost.UncachedInputCostUSD, 1.4)
	closeCost(t, result.Cost.OutputCostUSD, .4)
	closeCost(t, result.Cost.CacheReadCostUSD, .08)
	closeCost(t, result.Cost.CacheWriteCostUSD, .08)
	closeCost(t, result.Cost.TotalCostUSD, 1.96)
	// Persist the four-bucket and abnormal same-dimension requests, so every query
	// family has to use channel-only evidence and per-event nonnegative clamping.
	more := []entities.UsageEvent{}
	for i, tokens := range []helper.UsageTokenCostInput{subject.Tokens, {InputTokens: 100_000, CacheReadTokens: 200_000}, {InputTokens: 1_000_000}} {
		event := f.events[0]
		event.ID = 0
		event.EventKey = fmt.Sprintf("channel-buckets-%d", i)
		event.Timestamp = f.now.Add(-time.Duration(i+1) * time.Minute)
		event.InputTokens = tokens.InputTokens
		event.OutputTokens = tokens.OutputTokens
		event.CacheReadTokens = tokens.CacheReadTokens
		event.CacheCreationTokens = tokens.CacheCreationTokens
		event.TotalTokens = event.InputTokens + event.OutputTokens
		more = append(more, event)
	}
	if _, _, err := repository.InsertUsageEvents(f.db, more); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(context.Background(), f.db, f.end.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	f.set(t, f.ids[1], ".5")
	f.assertCostFamilies(t, 11.04, true)
	f.assertChannels(t, map[string]float64{f.ids[0]: 6.04, f.ids[1]: 5})
}
func TestChannelMembershipConflictUnknownAndDependencyConfirmedDeletion(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	f.set(t, f.ids[0], ".2")
	f.set(t, f.ids[1], ".5")
	before := f.catalog.Snapshot().ID()
	for _, members := range [][]string{{f.subjects[0], f.subjects[0]}, {f.subjects[1]}, {"missing-subject"}, nil} {
		_, err := f.channels.UpdatePricingChannel(ctx, f.ids[0], service.PricingChannelInput{Name: "Changed", MemberSubjectIDs: members})
		if err == nil {
			t.Fatal("conflict accepted")
		}
		if f.catalog.Snapshot().ID() != before {
			t.Fatal("failed mutation published")
		}
	}
	if err := f.channels.DeletePricingChannel(ctx, f.ids[0], false); !errors.Is(err, service.ErrPricingChannelDependencies) {
		t.Fatalf("unsafe delete %v", err)
	}
	f.assertCostFamilies(t, 7, true)
	// Later shared index evidence disables BOTH credential and channel overrides.
	if _, err := f.defaults.SetCredentialDefault(ctx, f.subjects[0], ".3"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&entities.UsageIdentity{}).Where("identity = ?", "synthetic-a").Update("binding_identity_status", "ambiguous").Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.LoadPricingSnapshot(ctx, f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.catalog.Replace(snapshot)
	result := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if result.Scope != "" || result.ChannelID != "" || result.AttributionWarning == "" {
		t.Fatalf("guessed ambiguous %+v", result)
	}
	closeCost(t, result.Cost.TotalCostUSD, 30)
	f.assertCostFamilies(t, 35, true)
	f.assertChannels(t, map[string]float64{repository.UnknownPricingChannel: 30, f.ids[1]: 5})
	if _, err := f.channels.CreatePricingChannel(ctx, service.PricingChannelInput{Name: "False independent", MemberSubjectIDs: []string{f.subjects[0]}}); err == nil {
		t.Fatal("shared independent membership accepted")
	}
	if err := f.channels.DeletePricingChannel(ctx, f.ids[0], true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.defaults.GetCredentialDefault(ctx, f.subjects[0]); err != nil {
		t.Fatal("channel deletion removed credential history")
	}
}
func TestChannelDefaultFailureCandidateCommitAndConcurrentPinnedReaders(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	f.set(t, f.ids[0], ".2")
	old := f.catalog.NewResolver()
	for _, text := range []string{"", "-1", "NaN", "Infinity", "20%x", "1e4", "99999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999"} {
		if _, err := f.channels.SetChannelDefault(ctx, f.ids[0], text); !errors.Is(err, service.ErrInvalidPricingInput) {
			t.Fatalf("invalid %s: %v", text, err)
		}
		if f.catalog.Snapshot().ID() != old.SnapshotID() {
			t.Fatal("invalid update published")
		}
	}
	if err := f.db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`CREATE TABLE channel_commit_probe (channel_id TEXT, FOREIGN KEY(channel_id) REFERENCES pricing_channels(id) DEFERRABLE INITIALLY DEFERRED)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`CREATE TRIGGER fail_channel_commit AFTER UPDATE ON channel_price_defaults BEGIN INSERT INTO channel_commit_probe(channel_id) VALUES ('missing'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.channels.SetChannelDefault(ctx, f.ids[0], ".8"); err == nil {
		t.Fatal("commit failure accepted")
	}
	if f.catalog.Snapshot().ID() != old.SnapshotID() {
		t.Fatal("commit failure published")
	}
	restarted, _ := newCatalogPricingService(t, f.db)
	saved, err := restarted.(service.PricingChannelsProvider).GetPricingChannel(ctx, f.ids[0])
	if err != nil || *saved.Multiplier != .2 {
		t.Fatalf("failed commit persisted %+v %v", saved, err)
	}
	if err := f.db.Exec("DROP TRIGGER fail_channel_commit").Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				result := old.Calculate(repository.UsageEventCostSubject(f.events[0]))
				if result.Cost.TotalCostUSD != 2 || result.ChannelName != "Channel A" {
					t.Errorf("pinned mixed %+v", result)
				}
			}
		}()
	}
	f.set(t, f.ids[0], ".8")
	if _, err := f.channels.UpdatePricingChannel(ctx, f.ids[0], service.PricingChannelInput{Name: "New A", MemberSubjectIDs: []string{f.subjects[0]}}); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	current := f.catalog.NewResolver().Calculate(repository.UsageEventCostSubject(f.events[0]))
	if current.Cost.TotalCostUSD != 8 || current.ChannelName != "New A" {
		t.Fatalf("new candidate %+v", current)
	}
	// A failed candidate after an otherwise valid member mutation rolls back rows.
	if err := f.db.Callback().Query().Before("gorm:query").Register("channel_candidate_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "channel_price_defaults" {
			tx.AddError(errors.New("synthetic candidate failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	before := f.catalog.Snapshot().ID()
	if _, err := f.channels.UpdatePricingChannel(ctx, f.ids[0], service.PricingChannelInput{Name: "Rejected A", MemberSubjectIDs: []string{}}); err == nil {
		t.Fatal("candidate failure accepted")
	}
	if f.catalog.Snapshot().ID() != before {
		t.Fatal("candidate failure published")
	}
	if err := f.db.Callback().Query().Remove("channel_candidate_failure"); err != nil {
		t.Fatal(err)
	}
	readback, _ := newCatalogPricingService(t, f.db)
	saved, err = readback.(service.PricingChannelsProvider).GetPricingChannel(ctx, f.ids[0])
	if err != nil || saved.Name != "New A" || len(saved.MemberSubjectIDs) != 1 {
		t.Fatalf("candidate failure persisted %+v %v", saved, err)
	}
}

func TestChannelOnlyExactTypedEvidenceAndUnknownRawTypes(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	f.set(t, f.ids[0], ".2")
	f.set(t, f.ids[1], ".5")
	// A second trusted type at the same exact index is distinguishable in raw
	// events, but neither a rollup nor an unknown raw type may borrow it.
	other := entities.UsageIdentity{Name: "Synthetic OAuth", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "synthetic-a", Type: "codex", BindingIdentityStatus: "unique"}
	if err := f.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	subject, err := f.prices.(service.PricingCredentialProvider).BindPricingCredential(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	channel, err := f.channels.CreatePricingChannel(ctx, service.PricingChannelInput{Name: "OAuth channel", MemberSubjectIDs: []string{subject.SubjectID}})
	if err != nil {
		t.Fatal(err)
	}
	f.set(t, channel.ID, ".8")
	start := f.start.Add(10 * time.Hour)
	end := start.Add(time.Hour)
	events := []entities.UsageEvent{}
	for i, evidence := range []struct{ index, kind string }{{"synthetic-a", "apikey"}, {"synthetic-a", "oauth"}, {"synthetic-a", ""}, {"synthetic-a", " apikey "}, {" synthetic-a ", "apikey"}} {
		events = append(events, entities.UsageEvent{EventKey: fmt.Sprintf("channel-exact-%d", i), APIGroupKey: "synthetic-downstream", Model: "base", AuthType: evidence.kind, AuthIndex: evidence.index, Timestamp: start.Add(time.Duration(i) * time.Minute), InputTokens: 1_000_000, TotalTokens: 1_000_000})
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
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.0
	unknown := 0
	for _, event := range page.Events {
		sum += event.CostUSD
		if event.ChannelID == "" {
			unknown++
			if event.AttributionWarning == "" {
				t.Fatal("unknown without warning")
			}
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
	// Index-only window still includes unknown types, but never selects a random
	// typed member. The padded original index belongs outside this exact filter.
	window, err := repository.SumUsageWindowStatsByAuthIndex(ctx, f.db, "synthetic-a", start, &end, f.catalog.NewResolver())
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, window.Cost, 20)
}

func TestChannelOnlyHistoricalEvidenceIncompleteAndMissingBaseline(t *testing.T) {
	f := newChannelFixture(t)
	ctx := context.Background()
	f.set(t, f.ids[0], ".2")
	f.set(t, f.ids[1], ".5")
	// Retained history can be split across hot/archive, including a duplicate ID.
	var event entities.UsageEvent
	if err := f.db.First(&event, "event_key = ?", f.events[0].EventKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("INSERT INTO usage_events_archive ("+entities.UsageEventStorageColumns+") SELECT "+entities.UsageEventStorageColumns+" FROM usage_events WHERE id = ?", event.ID).Error; err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 7, false)
	if err := f.db.Delete(&event).Error; err != nil {
		t.Fatal(err)
	}
	f.assertCostFamilies(t, 7, false)
	if err := f.db.Table("usage_events_archive").Where("id = ?", event.ID).Delete(&entities.UsageEventArchive{}).Error; err != nil {
		t.Fatal(err)
	}
	usage := service.NewUsageService(f.db, f.catalog)
	filter := servicedto.UsageFilter{Range: "custom", CustomUnit: "hour", StartTime: &f.start, EndTime: &f.end, EndExclusive: true}
	overview, err := usage.GetUsageOverview(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, overview.Summary.TotalCost, 5)
	if overview.Summary.CostAvailable || overview.Summary.UnavailableReason != "retained_pricing_evidence_incomplete" {
		t.Fatalf("incomplete %+v", overview.Summary)
	}
	comparisons, err := usage.(service.UsageComparisonProvider).GetUsageOverviewComparisons(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if comparisons.Comparisons.Channels[f.ids[0]] != nil {
		t.Fatal("guessed uncovered named channel")
	}
	if comparisons.Comparisons.Channels[repository.UnknownPricingChannel].CostAvailable {
		t.Fatal("unknown history complete")
	}
	analysis, err := usage.GetAnalysis(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	closeCost(t, analysis.CostBreakdown.TotalCostUSD, 5)
	if analysis.CostBreakdown.CostAvailable {
		t.Fatal("analysis complete")
	}
	window, err := repository.SumUsageWindowStatsByAuthIndex(ctx, f.db, "synthetic-a", f.start, &f.end, f.catalog.NewResolver())
	if err != nil {
		t.Fatal(err)
	}
	if window.CostAvailable {
		t.Fatal("window complete")
	}
	f.set(t, f.ids[1], "0")
	missing := entities.UsageEvent{EventKey: "channel-missing-baseline", Model: "missing", AuthIndex: "synthetic-b", AuthType: "apikey", APIGroupKey: "synthetic-downstream", Timestamp: f.now.Add(-time.Minute), InputTokens: 1, TotalTokens: 1}
	if _, _, err := repository.InsertUsageEvents(f.db, []entities.UsageEvent{missing}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AggregateUsageOverviewStats(ctx, f.db, f.end); err != nil {
		t.Fatal(err)
	}
	page, err := usage.ListUsageEvents(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range page.Events {
		if row.Model == "missing" && row.CostAvailable {
			t.Fatal("zero invented free missing baseline")
		}
	}
}

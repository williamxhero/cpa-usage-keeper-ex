package test

import (
	"math"
	"testing"

	"cpa-usage-keeper/internal/pricing"
)

func TestChannelModelCompilerAtomicSafetyAndDefensiveCopies(t *testing.T) {
	bindings := []pricing.CredentialBinding{{SubjectID: "cred_a", AuthType: "apikey", AuthIndex: "synthetic-a", Status: "unique"}}
	channel := pricing.ChannelConfig{ID: "chan_a", Name: "Channel A", MemberSubjectIDs: []string{"cred_a"}}
	fixed := pricing.FixedTariff{PromptPricePer1M: new(1.0), CompletionPricePer1M: new(2.0), CacheReadPricePer1M: new(3.0), CacheWritePricePer1M: new(4.0), PricingStyle: "openai"}
	valid := pricing.ChannelModelConfig{ChannelID: "chan_a", Model: "exact", Mode: pricing.ModeFixed, Fixed: &fixed, Multiplier: 99}
	compile := func(configs []pricing.ChannelModelConfig) (*pricing.Snapshot, error) {
		return pricing.CompileSnapshotWithCredentials(nil, bindings, nil, pricing.OverrideConfig{Channels: []pricing.ChannelConfig{channel}, ChannelModels: configs})
	}
	cases := map[string][]pricing.ChannelModelConfig{
		"orphan":                     {{ChannelID: "missing", Model: "exact", Multiplier: 1}},
		"blank model":                {{ChannelID: "chan_a", Model: "  ", Multiplier: 1}},
		"duplicate normalized model": {{ChannelID: "chan_a", Model: "exact", Multiplier: 1}, {ChannelID: "chan_a", Model: " exact ", Multiplier: 2}},
		"negative":                   {{ChannelID: "chan_a", Model: "exact", Multiplier: -1}},
		"NaN":                        {{ChannelID: "chan_a", Model: "exact", Multiplier: math.NaN()}},
		"infinite":                   {{ChannelID: "chan_a", Model: "exact", Multiplier: math.Inf(1)}},
		"mode":                       {{ChannelID: "chan_a", Model: "exact", Mode: "stacked"}},
		"missing fixed":              {{ChannelID: "chan_a", Model: "exact", Mode: pricing.ModeFixed}},
	}
	for name, configs := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := compile(configs); err == nil {
				t.Fatal("invalid candidate compiled")
			}
		})
	}
	input := []pricing.ChannelModelConfig{valid, {ChannelID: "chan_a", Model: "Exact", Multiplier: 1, Fixed: &fixed}}
	snapshot, err := compile(input)
	if err != nil {
		t.Fatal(err)
	}
	*fixed.PromptPricePer1M = 999
	input[0].Model = "changed"
	output := snapshot.ChannelModelConfigs("chan_a")
	if len(output) != 2 || output[0].Model != "Exact" || output[0].Fixed != nil || output[1].Model != "exact" || output[1].Multiplier != 0 || *output[1].Fixed.PromptPricePer1M != 1 {
		t.Fatalf("input mode/copy leaked %+v", output)
	}
	*output[1].Fixed.CacheReadPricePer1M = 999
	output[1].Model = "changed"
	item, ok := snapshot.ChannelModelPricing("chan_a", "exact")
	if !ok || *item.Fixed.CacheReadPricePer1M != 3 {
		t.Fatalf("output copy leaked %+v", item)
	}
	*item.Fixed.CacheWritePricePer1M = 999
	item, _ = snapshot.ChannelModelPricing("chan_a", "exact")
	if *item.Fixed.CacheWritePricePer1M != 4 {
		t.Fatal("getter pointers leaked")
	}
	fallback := snapshot.WithoutCredentialAttribution()
	items := fallback.ChannelModelConfigs("chan_a")
	if len(items) != 2 || *items[1].Fixed.PromptPricePer1M != 1 {
		t.Fatal("metadata failure discarded prices")
	}
	if _, ok := snapshot.ChannelModelPricing("chan_a", "EXACT"); ok {
		t.Fatal("case-folded model")
	}
}

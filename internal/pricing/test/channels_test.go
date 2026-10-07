package test

import (
	"math"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/pricing"
)

func TestChannelSnapshotDefensivelyCopiesConfiguration(t *testing.T) {
	value := .2
	channels := []pricing.ChannelConfig{{ID: "chan_a", Name: "Channel A", MemberSubjectIDs: []string{"cred_a"}, Multiplier: &value}}
	snapshot, err := pricing.CompileSnapshotWithCredentials(nil, []pricing.CredentialBinding{{SubjectID: "cred_a", AuthType: "apikey", AuthIndex: "synthetic-a", Status: "unique"}}, nil, pricing.OverrideConfig{Channels: channels})
	if err != nil {
		t.Fatal(err)
	}
	channels[0].Name = "Changed input"
	channels[0].MemberSubjectIDs[0] = "other"
	value = 10
	output := snapshot.Channels()
	if output[0].Name != "Channel A" || output[0].MemberSubjectIDs[0] != "cred_a" || *output[0].Multiplier != .2 {
		t.Fatalf("mutable input leaked: %+v", output)
	}
	output[0].Name = "Changed output"
	output[0].MemberSubjectIDs[0] = "other"
	*output[0].Multiplier = 20
	final := snapshot.Channels()[0]
	if final.Name != "Channel A" || final.MemberSubjectIDs[0] != "cred_a" || *final.Multiplier != .2 {
		t.Fatalf("mutable output leaked: %+v", final)
	}
}

func TestChannelCompilerRejectsInvalidFullCandidates(t *testing.T) {
	bindings := []pricing.CredentialBinding{{SubjectID: "cred_a", AuthType: "apikey", AuthIndex: "synthetic-a", Status: "unique"}}
	valid := pricing.ChannelConfig{ID: "chan_a", Name: "Channel A", MemberSubjectIDs: []string{"cred_a"}}
	cases := map[string][]pricing.ChannelConfig{
		"duplicate channel":      {valid, valid},
		"conflicting membership": {valid, {ID: "chan_b", Name: "Channel B", MemberSubjectIDs: []string{"cred_a"}}},
		"duplicate membership":   {{ID: "chan_a", Name: "Channel A", MemberSubjectIDs: []string{"cred_a", "cred_a"}}},
		"missing subject":        {{ID: "chan_a", Name: "Channel A", MemberSubjectIDs: []string{"missing"}}},
		"blank ID":               {{Name: "Channel A"}},
		"blank name":             {{ID: "chan_a", Name: " "}},
		"unsafe name":            {{ID: "chan_a", Name: "https://example.invalid?token=private"}},
		"long name":              {{ID: "chan_a", Name: strings.Repeat("a", 129)}},
		"negative":               {{ID: "chan_a", Name: "Channel A", Multiplier: new(-1.0)}},
		"infinite":               {{ID: "chan_a", Name: "Channel A", Multiplier: new(math.Inf(1))}},
		"NaN":                    {{ID: "chan_a", Name: "Channel A", Multiplier: new(math.NaN())}},
	}
	for name, configs := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := pricing.CompileSnapshotWithCredentials(nil, bindings, nil, pricing.OverrideConfig{Channels: configs}); err == nil {
				t.Fatal("invalid candidate compiled")
			}
		})
	}
	if _, err := pricing.CompileSnapshotWithCredentials(nil, bindings, nil, pricing.OverrideConfig{}, pricing.OverrideConfig{}); err == nil {
		t.Fatal("multiple optional configs accepted")
	}
}

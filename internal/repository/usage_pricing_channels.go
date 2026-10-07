package repository

import (
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository/dto"
	"time"
)

const UnknownPricingChannel = "__unbound_channel__"

func applyPricingChannelComparison(comparisons *dto.UsageOverviewComparisonsRecord, resolver pricing.Resolver, subject pricing.CostSubject, row dto.UsageComparisonItemRecord) {
	if !resolver.HasChannels() {
		return
	}
	if comparisons.Channels == nil {
		comparisons.Channels = map[string]*dto.UsageComparisonItemRecord{}
	}
	id, name, _ := resolver.ChannelAttribution(subject)
	if id == "" {
		id = UnknownPricingChannel
		name = "Unbound / unknown channel"
	}
	row.Label = name
	addUsageOverviewComparison(comparisons.Channels, id, row)
}

func pricingIdentitySubject(identity usagePricingIdentity) pricing.CostSubject {
	return pricing.CostSubject{AuthType: identity.AuthType, IdentityAuthIndex: identity.AuthIndex, ObservedIdentity: true}
}

// Split only a fully covered selected cohort, whose typed fees were calculated
// by the same resolver. Uncovered history cannot invent named channel evidence.
func applyPricingRollupChannelComparison(comparisons *dto.UsageOverviewComparisonsRecord, resolver pricing.Resolver, evidence usagePricingEvidence, covered bool, row dto.UsageComparisonItemRecord, bucket time.Time, byDay bool) {
	comparisons.PricingSnapshotID = resolver.SnapshotID()
	if !resolver.HasChannels() {
		return
	}
	row.Bucket, _ = usageOverviewBucket(bucket, byDay)
	if covered && evidence.Selected {
		for identity, typed := range evidence.Typed {
			typed.Bucket = row.Bucket
			applyPricingChannelComparison(comparisons, resolver, pricingIdentitySubject(identity), typed)
		}
		return
	}
	// Preserve the original legacy aggregate cost (including its clamping) when no
	// override selected. Name it only if every exact constituent proves one channel.
	subject := pricing.CostSubject{ObservedIdentity: true}
	if covered {
		channel := ""
		unique := true
		for identity := range evidence.Typed {
			candidate := pricingIdentitySubject(identity)
			id, _, _ := resolver.ChannelAttribution(candidate)
			if channel == "" {
				channel = id
				subject = candidate
			}
			if id == "" || channel != id {
				unique = false
				break
			}
		}
		if !unique {
			subject = pricing.CostSubject{ObservedIdentity: true}
		}
	}
	applyPricingChannelComparison(comparisons, resolver, subject, row)
}

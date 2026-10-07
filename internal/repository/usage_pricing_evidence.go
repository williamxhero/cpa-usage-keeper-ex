package repository

import (
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository/dto"
	"cpa-usage-keeper/internal/timeutil"
	"fmt"
	"gorm.io/gorm"
	"strings"
	"time"
)

type usagePricingEvidenceKey struct {
	Bucket     time.Time
	Dimensions pricing.UsageDimensions
}

type usagePricingIdentity struct{ AuthType, AuthIndex string }

type usagePricingEvidence struct {
	Requests    int64
	Tokens      helper.UsageTokenCostInput
	TotalTokens int64
	Selected    bool
	Result      pricing.CostResult
	Typed       map[usagePricingIdentity]dto.UsageComparisonItemRecord
}

func (e usagePricingEvidence) covers(requests, totalTokens int64, tokens helper.UsageTokenCostInput) bool {
	return e.Requests == requests && e.Tokens == tokens && e.TotalTokens == totalTokens
}

type usagePricingEvidenceMap map[usagePricingEvidenceKey]usagePricingEvidence

func pricingEvidenceBucket(timestamp time.Time, grain string) time.Time {
	timestamp = timeutil.NormalizeStorageTime(timestamp)
	if grain == "daily" {
		return time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, timestamp.Location())
	}
	if grain == "range" {
		return time.Time{}
	}
	return timestamp.Truncate(time.Hour)
}

// loadUsagePricingEvidence reads only retained pricing facts, not raw_json or
// secrets. Hot/archive share IDs and move atomically; the anti-join also guards
// against counting an accidentally duplicated stored ID twice. It does not
// change which events an existing endpoint includes in its usage totals.
func loadUsagePricingEvidence(db *gorm.DB, filter dto.UsageQueryFilter, start, end time.Time, grain string, resolver pricing.Resolver) (usagePricingEvidenceMap, error) {
	if !resolver.HasPricingOverrides() && !resolver.HasChannels() {
		return nil, nil
	}
	columns := "id, api_group_key, model, model_alias, auth_index, auth_type, service_tier, response_service_tier, reasoning_effort, endpoint, executor_type, timestamp, failed, input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_creation_tokens, total_tokens"
	// Only committed Overview members can explain its rollups. A newly
	// ingested historical request must never replace a pruned old request with
	// equal token totals. Callers pin rows, watermark, and evidence in one read
	// transaction whenever defaults are active.
	checkpoint, err := LoadUsageAggregationCheckpointSnapshot(db.Statement.Context, db)
	if err != nil {
		return nil, err
	}
	where := "timestamp >= ? AND timestamp < ? AND id <= ?"
	args := []any{timeutil.FormatStorageTime(start), timeutil.FormatStorageTime(end), checkpoint.OverviewCursor}
	if key := strings.TrimSpace(filter.APIGroupKey); key != "" {
		where += " AND api_group_key = ?"
		args = append(args, key)
	}
	sql := "SELECT " + columns + " FROM usage_events WHERE " + where + " UNION ALL SELECT " + columns + " FROM usage_events_archive WHERE " + where + " AND NOT EXISTS (SELECT 1 FROM usage_events hot WHERE hot.id = usage_events_archive.id)"
	args = append(args, args...)
	rows, err := db.Raw(sql, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("load retained pricing evidence: %w", err)
	}
	defer rows.Close()
	result := make(usagePricingEvidenceMap)
	for rows.Next() {
		var event entities.UsageEvent
		if err := db.ScanRows(rows, &event); err != nil {
			return nil, err
		}
		subject := UsageEventCostSubject(event)
		key := usagePricingEvidenceKey{Bucket: pricingEvidenceBucket(event.Timestamp, grain), Dimensions: subject.Dimensions}
		evidence, exists := result[key]
		if !exists {
			evidence.Result.Available = true
			evidence.Typed = make(map[usagePricingIdentity]dto.UsageComparisonItemRecord)
		}
		evidence.Requests++
		evidence.Tokens.InputTokens += event.InputTokens
		evidence.Tokens.OutputTokens += event.OutputTokens
		evidence.Tokens.CacheReadTokens += event.CacheReadTokens
		evidence.Tokens.CacheCreationTokens += event.CacheCreationTokens
		evidence.TotalTokens += event.TotalTokens
		evidence.Selected = evidence.Selected || resolver.UsesPricingOverride(subject)
		cost := resolver.Calculate(subject)
		// Preserve safe matching evidence for existing availability guards. A
		// cohort may contain several typed credentials, so it has no single ID
		// or multiplier, but must not masquerade as an unmatched model.
		if cost.Scope != "" {
			evidence.Result.Scope = "pricing_override"
		}
		if cost.MatchedModel != "" {
			evidence.Result.MatchedModel = cost.MatchedModel
		}
		evidence.Result.Available = evidence.Result.Available && cost.Available
		evidence.Result.Cost = addUsagePricingCost(evidence.Result.Cost, cost.Cost)
		identity := usagePricingIdentity{event.AuthType, event.AuthIndex}
		typed, present := evidence.Typed[identity]
		if !present {
			typed.CostAvailable = true
		}
		typed.Requests++
		if event.Failed {
			typed.Failures++
		}
		typed.InputTokens += event.InputTokens
		typed.OutputTokens += event.OutputTokens
		typed.CacheReadTokens += event.CacheReadTokens
		typed.CacheCreationTokens += event.CacheCreationTokens
		typed.ReasoningTokens += event.ReasoningTokens
		typed.TotalTokens += event.TotalTokens
		typed.CostUSD += cost.Cost.TotalCostUSD
		typed.CostAvailable = typed.CostAvailable && cost.Available
		evidence.Typed[identity] = typed
		if !cost.Available {
			evidence.Result.UnavailableReason = "missing_baseline"
		}
		result[key] = evidence
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func addUsagePricingCost(a, b helper.UsageTokenCostBreakdown) helper.UsageTokenCostBreakdown {
	a.UncachedInputCostUSD += b.UncachedInputCostUSD
	a.OutputCostUSD += b.OutputCostUSD
	a.CacheReadCostUSD += b.CacheReadCostUSD
	a.CacheWriteCostUSD += b.CacheWriteCostUSD
	a.TotalCostUSD += b.TotalCostUSD
	return a
}

// The coverage check is intentionally stronger than just a row count. A rollup
// may lag raw data, or raw retention may leave only part of a cohort. Neither
// case proves per-event clamping or exact auth_type for that aggregate.
func calculateUsageRollupCost(resolver pricing.Resolver, legacySubject pricing.CostSubject, bucket time.Time, grain string, requests, totalTokens int64, rawTokens helper.UsageTokenCostInput, evidence usagePricingEvidenceMap) pricing.CostResult {
	legacy := resolver.Calculate(legacySubject)
	if !resolver.HasPricingOverrides() {
		return legacy
	}
	key := usagePricingEvidenceKey{Bucket: pricingEvidenceBucket(bucket, grain), Dimensions: legacySubject.Dimensions}
	retained, exists := evidence[key]
	selected := resolver.UsesPricingOverride(legacySubject)
	if exists && retained.covers(requests, totalTokens, rawTokens) {
		if retained.Selected {
			return retained.Result
		}
		// Exact retained evidence may disprove a type-less index attribution.
		// No new selection means the original rollup normalization/grouping
		// must remain unchanged, not be replaced by per-request legacy costs.
		if selected {
			return resolver.CalculateLegacy(legacySubject)
		}
		return legacy
	}
	if selected || retained.Selected || resolver.MayUsePricingOverride(legacySubject) {
		legacy.Cost = helper.UsageTokenCostBreakdown{}
		legacy.Available = !helper.UsageTokenInputRequiresPricing(legacySubject.Tokens)
		if !legacy.Available {
			legacy.UnavailableReason = "retained_pricing_evidence_incomplete"
		}
	}
	return legacy
}

// Typed retained facts split upstream composition only after the parent cohort
// has passed membership/coverage validation. Never copy a mixed fee to both
// OAuth and API-key identities merely because their indexes collide.
func pricingTypedIdentityLookup(lookup analysisIdentityLookup, authType string) analysisIdentityLookup {
	var kind entities.UsageIdentityAuthType
	switch authType {
	case "oauth":
		kind = entities.UsageIdentityAuthTypeAuthFile
	case "apikey":
		kind = entities.UsageIdentityAuthTypeAIProvider
	default:
		return analysisIdentityLookup{}
	}
	return analysisIdentityLookup{kind: lookup[kind]}
}

// Include original indexes hidden by existing rollup normalization, without
// changing legacy identity lookup when no credential evidence is present.
func extendPricingIdentityLookup(db *gorm.DB, lookup analysisIdentityLookup, evidence usagePricingEvidenceMap) (analysisIdentityLookup, error) {
	indexes := []string{}
	seen := map[string]bool{}
	for _, cohort := range evidence {
		for identity := range cohort.Typed {
			if identity.AuthIndex != "" && !seen[identity.AuthIndex] {
				indexes = append(indexes, identity.AuthIndex)
				seen[identity.AuthIndex] = true
			}
		}
	}
	if len(indexes) == 0 {
		return lookup, nil
	}
	exact, err := loadAnalysisIdentityLookup(db, indexes)
	if err != nil {
		return nil, err
	}
	for kind, values := range exact {
		if lookup[kind] == nil {
			lookup[kind] = map[string]analysisIdentityInfo{}
		}
		for index, value := range values {
			lookup[kind][index] = value
		}
	}
	return lookup, nil
}

func pricingExactIdentity(lookup analysisIdentityLookup, identity usagePricingIdentity) (analysisIdentityInfo, bool) {
	typed := pricingTypedIdentityLookup(lookup, identity.AuthType)
	for kind := range typed {
		return typed.find(kind, identity.AuthIndex)
	}
	return analysisIdentityInfo{}, false
}

func applyAnalysisPricingIdentityComposition(lookup analysisIdentityLookup, authFiles, providers map[string]*dto.AnalysisCompositionRecord, row analysisOverviewStatProjection, grain string, result pricing.CostResult, evidence usagePricingEvidenceMap) {
	subject := newUsagePricingCostSubject(row.APIGroupKey, row.Model, row.AuthIndex, row.ModelAlias, row.ServiceTier, row.ResponseServiceTier, row.ReasoningEffort, row.Endpoint, row.ExecutorType, 0, 0, 0, 0)
	key := usagePricingEvidenceKey{Bucket: pricingEvidenceBucket(row.BucketStart, grain), Dimensions: subject.Dimensions}
	retained := evidence[key]
	// Available-zero is not proof of retained request/token coverage.
	if result.Scope != "" && retained.Selected && retained.covers(row.RequestCount, row.TotalTokens, helper.UsageTokenCostInput{InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, CacheReadTokens: row.CacheReadTokens, CacheCreationTokens: row.CacheCreationTokens}) {
		for exact, typed := range retained.Typed {
			identity, found := pricingExactIdentity(lookup, exact)
			if !found {
				continue
			} // Never assign another original index's fee.
			totals := authFiles
			if identity.authType == entities.UsageIdentityAuthTypeAIProvider {
				totals = providers
			}
			applyAnalysisIdentityCompositionTotal(totals, identity, typed.Requests, typed.InputTokens, typed.OutputTokens, typed.CacheReadTokens, typed.CacheCreationTokens, typed.ReasoningTokens, typed.TotalTokens, typed.CostUSD, typed.CostAvailable)
		}
		return
	}
	applyAnalysisIdentityComposition(lookup, authFiles, providers, row.AuthIndex, row.RequestCount, row.InputTokens, row.OutputTokens, row.CacheReadTokens, row.CacheCreationTokens, row.ReasoningTokens, row.TotalTokens, result.Cost, result.Available)
}

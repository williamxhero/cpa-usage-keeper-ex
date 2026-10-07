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

type usagePricingEvidence struct {
	Requests    int64
	Tokens      helper.UsageTokenCostInput
	TotalTokens int64
	Selected    bool
	Result      pricing.CostResult
	Typed       map[string]dto.UsageComparisonItemRecord
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
	if !resolver.HasCredentialDefaults() {
		return nil, nil
	}
	columns := "id, api_group_key, model, model_alias, auth_index, auth_type, service_tier, response_service_tier, reasoning_effort, endpoint, executor_type, timestamp, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_tokens"
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
			evidence.Typed = make(map[string]dto.UsageComparisonItemRecord)
		}
		evidence.Requests++
		evidence.Tokens.InputTokens += event.InputTokens
		evidence.Tokens.OutputTokens += event.OutputTokens
		evidence.Tokens.CacheReadTokens += event.CacheReadTokens
		evidence.Tokens.CacheCreationTokens += event.CacheCreationTokens
		evidence.TotalTokens += event.TotalTokens
		evidence.Selected = evidence.Selected || resolver.UsesCredentialDefault(subject)
		cost := resolver.Calculate(subject)
		// Preserve safe matching evidence for existing availability guards. A
		// cohort may contain several typed credentials, so it has no single ID
		// or multiplier, but must not masquerade as an unmatched model.
		if cost.Scope != "" {
			evidence.Result.Scope = "credential_default"
		}
		if cost.MatchedModel != "" {
			evidence.Result.MatchedModel = cost.MatchedModel
		}
		evidence.Result.Available = evidence.Result.Available && cost.Available
		evidence.Result.Cost = addUsagePricingCost(evidence.Result.Cost, cost.Cost)
		typed, present := evidence.Typed[event.AuthType]
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
		evidence.Typed[event.AuthType] = typed
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
	if !resolver.HasCredentialDefaults() {
		return legacy
	}
	key := usagePricingEvidenceKey{Bucket: pricingEvidenceBucket(bucket, grain), Dimensions: legacySubject.Dimensions}
	retained, exists := evidence[key]
	selected := resolver.UsesCredentialDefault(legacySubject)
	if exists && retained.Requests == requests && retained.Tokens == rawTokens && retained.TotalTokens == totalTokens {
		if retained.Selected {
			return retained.Result
		}
		// Exact retained evidence may disprove a type-less index attribution.
		if selected {
			return retained.Result
		}
		return legacy
	}
	if selected || retained.Selected || resolver.MayUseCredentialDefault(legacySubject) {
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

func applyAnalysisPricingIdentityComposition(lookup analysisIdentityLookup, authFiles, providers map[string]*dto.AnalysisCompositionRecord, row analysisOverviewStatProjection, grain string, result pricing.CostResult, evidence usagePricingEvidenceMap) {
	if result.Scope == "credential_default" && result.UnavailableReason != "retained_pricing_evidence_incomplete" {
		subject := newUsagePricingCostSubject(row.APIGroupKey, row.Model, row.AuthIndex, row.ModelAlias, row.ServiceTier, row.ResponseServiceTier, row.ReasoningEffort, row.Endpoint, row.ExecutorType, 0, 0, 0, 0)
		key := usagePricingEvidenceKey{Bucket: pricingEvidenceBucket(row.BucketStart, grain), Dimensions: subject.Dimensions}
		for authType, typed := range evidence[key].Typed {
			applyAnalysisIdentityComposition(pricingTypedIdentityLookup(lookup, authType), authFiles, providers, row.AuthIndex, typed.Requests, typed.InputTokens, typed.OutputTokens, typed.CacheReadTokens, typed.CacheCreationTokens, typed.ReasoningTokens, typed.TotalTokens, helper.UsageTokenCostBreakdown{TotalCostUSD: typed.CostUSD}, typed.CostAvailable)
		}
		return
	}
	applyAnalysisIdentityComposition(lookup, authFiles, providers, row.AuthIndex, row.RequestCount, row.InputTokens, row.OutputTokens, row.CacheReadTokens, row.CacheCreationTokens, row.ReasoningTokens, row.TotalTokens, result.Cost, result.Available)
}

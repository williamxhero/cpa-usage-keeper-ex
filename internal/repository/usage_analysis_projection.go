package repository

import (
	"fmt"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository/dto"
	"cpa-usage-keeper/internal/timeutil"

	"gorm.io/gorm"
)

// analysisOverviewStatProjection 同时承接 hourly 与 daily 聚合表的 Analysis 窄投影。
type analysisOverviewStatProjection struct {
	BucketStart         time.Time
	APIGroupKey         string
	Model               string
	AuthIndex           string
	ModelAlias          string
	ServiceTier         string
	ResponseServiceTier string
	ReasoningEffort     string
	Endpoint            string
	ExecutorType        string
	RequestCount        int64
	InputTokens         int64
	OutputTokens        int64
	ReasoningTokens     int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	TotalTokens         int64
}

var analysisOverviewProjectionFixedColumns = [...]string{
	"bucket_start",
	"api_group_key",
	"model",
	"auth_index",
	"model_alias",
	"request_count",
	"input_tokens",
	"output_tokens",
	"reasoning_tokens",
	"cache_read_tokens",
	"cache_creation_tokens",
	"total_tokens",
}

func analysisOverviewProjectionColumns(activeFields pricing.ActiveFields) string {
	columns := make([]string, 0, len(analysisOverviewProjectionFixedColumns)+activeFields.Len())
	seen := make(map[string]struct{}, len(analysisOverviewProjectionFixedColumns)+activeFields.Len())
	for _, column := range analysisOverviewProjectionFixedColumns {
		columns = append(columns, column)
		seen[column] = struct{}{}
	}
	// 价格维度只能来自编译后的固定枚举；展示必需列已固定读取，这里只追加实际启用且未重复的条件列。
	for _, column := range UsagePricingDimensionColumns(activeFields) {
		if _, ok := seen[column]; ok {
			continue
		}
		columns = append(columns, column)
		seen[column] = struct{}{}
	}
	return strings.Join(columns, ", ")
}

func loadAnalysisOverviewHourlyStatsWithFilter(db *gorm.DB, filter dto.UsageQueryFilter, start, end time.Time, activeFields pricing.ActiveFields) ([]analysisOverviewStatProjection, error) {
	query := db.Model(&entities.UsageOverviewHourlyStat{}).
		Joins("INNER JOIN cpa_api_keys ON cpa_api_keys.api_key = usage_overview_hourly_stats.api_group_key AND cpa_api_keys.is_deleted = ?", false)
	return loadAnalysisOverviewStatProjection(query, filter, start, end, "hourly", activeFields)
}

func loadAnalysisOverviewDailyStatsWithFilter(db *gorm.DB, filter dto.UsageQueryFilter, start, end time.Time, activeFields pricing.ActiveFields) ([]analysisOverviewStatProjection, error) {
	query := db.Model(&entities.UsageOverviewDailyStat{}).
		Joins("INNER JOIN cpa_api_keys ON cpa_api_keys.api_key = usage_overview_daily_stats.api_group_key AND cpa_api_keys.is_deleted = ?", false)
	return loadAnalysisOverviewStatProjection(query, filter, start, end, "daily", activeFields)
}

func loadAnalysisOverviewStatProjection(query *gorm.DB, filter dto.UsageQueryFilter, start, end time.Time, grain string, activeFields pricing.ActiveFields) ([]analysisOverviewStatProjection, error) {
	rows := make([]analysisOverviewStatProjection, 0)
	query = query.
		Select(analysisOverviewProjectionColumns(activeFields)).
		Where("bucket_start >= ? AND bucket_start < ?", timeutil.FormatStorageTime(start), timeutil.FormatStorageTime(end)).
		Order("bucket_start asc")
	if apiGroupKey := strings.TrimSpace(filter.APIGroupKey); apiGroupKey != "" {
		query = query.Where("api_group_key = ?", apiGroupKey)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load usage overview %s stats: %w", grain, err)
	}
	return rows, nil
}

// analysisReferenceCohorts reconciles the complete membership of each narrow
// projection coordinate. Inactive dimensions may produce several physical rows;
// their reference is emitted once without regrouping any configured legacy fee.
func analysisReferenceCohorts(resolver pricing.Resolver, rows []analysisOverviewStatProjection, grain string, evidence usagePricingEvidenceMap) map[usagePricingEvidenceKey]pricing.PriceEstimate {
	totals := make(map[usagePricingEvidenceKey]analysisOverviewStatProjection)
	for _, row := range rows {
		key := analysisReferenceKey(row, grain)
		total := totals[key]
		total.RequestCount += row.RequestCount
		total.InputTokens += row.InputTokens
		total.OutputTokens += row.OutputTokens
		total.CacheReadTokens += row.CacheReadTokens
		total.CacheCreationTokens += row.CacheCreationTokens
		total.TotalTokens += row.TotalTokens
		totals[key] = total
	}
	result := make(map[usagePricingEvidenceKey]pricing.PriceEstimate, len(totals))
	for key, row := range totals {
		retained, exists := evidence[key]
		subject := pricing.CostSubject{Dimensions: key.Dimensions}
		result[key] = referenceForUsageCohort(resolver, subject, retained, exists, row.RequestCount, row.TotalTokens, helper.UsageTokenCostInput{InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, CacheReadTokens: row.CacheReadTokens, CacheCreationTokens: row.CacheCreationTokens})
	}
	return result
}

func analysisReferenceKey(row analysisOverviewStatProjection, grain string) usagePricingEvidenceKey {
	subject := newUsagePricingCostSubject(row.APIGroupKey, row.Model, row.AuthIndex, row.ModelAlias, row.ServiceTier, row.ResponseServiceTier, row.ReasoningEffort, row.Endpoint, row.ExecutorType, 0, 0, 0, 0)
	return usagePricingEvidenceKey{Bucket: pricingEvidenceBucket(row.BucketStart, grain), Dimensions: subject.Dimensions}
}

func analysisRowDualCosts(result pricing.CostResult, row analysisOverviewStatProjection, grain string, references map[usagePricingEvidenceKey]pricing.PriceEstimate) pricing.DualCosts {
	dual := result.DualCosts()
	key := analysisReferenceKey(row, grain)
	dual.Reference = references[key]
	delete(references, key)
	return dual
}

func calculateAnalysisOverviewProjectionCost(costResolver pricing.Resolver, row analysisOverviewStatProjection, grain string, evidence usagePricingEvidenceMap) pricing.CostResult {
	subject := newUsagePricingCostSubject(
		row.APIGroupKey,
		row.Model,
		row.AuthIndex,
		row.ModelAlias,
		row.ServiceTier,
		row.ResponseServiceTier,
		row.ReasoningEffort,
		row.Endpoint,
		row.ExecutorType,
		row.InputTokens,
		row.OutputTokens,
		row.CacheReadTokens,
		row.CacheCreationTokens,
	)
	return calculateUsageRollupCost(costResolver, subject, row.BucketStart, grain, row.RequestCount, row.TotalTokens, helper.UsageTokenCostInput{InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, CacheReadTokens: row.CacheReadTokens, CacheCreationTokens: row.CacheCreationTokens}, evidence)
}

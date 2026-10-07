package dto

import (
	"time"

	"cpa-usage-keeper/internal/pricing"
)

type AnalysisGranularity string

const (
	AnalysisGranularityHourly AnalysisGranularity = "hourly"
	AnalysisGranularityDaily  AnalysisGranularity = "daily"
)

type AnalysisTokenUsageBucket struct {
	DualCosts           pricing.DualCosts
	Bucket              time.Time
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	ReasoningTokens     int64
	TotalTokens         int64
	Requests            int64
	CostUSD             float64
	CostAvailable       bool
}

type AnalysisModelUsage struct {
	Bucket      time.Time
	Model       string
	TotalTokens int64
	Requests    int64
}

type AnalysisCompositionItem struct {
	DualCosts           pricing.DualCosts
	Key                 string
	Label               string
	TotalTokens         int64
	Requests            int64
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	ReasoningTokens     int64
	CostUSD             float64
	CostAvailable       bool
}

type AnalysisHeatmapCell struct {
	DualCosts           pricing.DualCosts
	APIKey              string
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	ReasoningTokens     int64
	TotalTokens         int64
	Requests            int64
	CostUSD             float64
	CostAvailable       bool
}

type AnalysisCostBreakdown struct {
	DualCosts            pricing.DualCosts
	UnavailableReason    string
	PricingSnapshotID    string
	UncachedInputCostUSD float64
	CacheReadCostUSD     float64
	CacheWriteCostUSD    float64
	OutputCostUSD        float64
	TotalCostUSD         float64
	CostAvailable        bool
}

type AnalysisModelEfficiencyItem struct {
	DualCosts              pricing.DualCosts
	Model                  string
	Requests               int64
	InputTokens            int64
	OutputTokens           int64
	CacheReadTokens        int64
	CacheCreationTokens    int64
	ReasoningTokens        int64
	TotalTokens            int64
	CostUSD                float64
	CostAvailable          bool
	CostPerRequestUSD      float64
	OutputTokensPerRequest float64
	CacheReadRate          float64
}

type AnalysisLatencyPoint struct {
	TTFTMS    int64
	LatencyMS int64
}

type AnalysisLatencyDensityCell struct {
	TTFTMinMS    int64
	TTFTMaxMS    int64
	LatencyMinMS int64
	LatencyMaxMS int64
	Count        int64
	Intensity    float64
}

type AnalysisLatencyDiagnostics struct {
	Points       []AnalysisLatencyPoint
	Density      []AnalysisLatencyDensityCell
	TotalPoints  int64
	Sampled      bool
	P95TTFTMS    int64
	P95LatencyMS int64
	MaxTTFTMS    int64
	MaxLatencyMS int64
}

type AnalysisSnapshot struct {
	PricingSnapshotID     string
	Granularity           AnalysisGranularity
	RangeStart            *time.Time
	RangeEnd              *time.Time
	TokenUsage            []AnalysisTokenUsageBucket
	ModelUsage            []AnalysisModelUsage
	APIKeyComposition     []AnalysisCompositionItem
	ModelComposition      []AnalysisCompositionItem
	AuthFilesComposition  []AnalysisCompositionItem
	AIProviderComposition []AnalysisCompositionItem
	Heatmap               []AnalysisHeatmapCell
	CostBreakdown         AnalysisCostBreakdown
	ModelEfficiency       []AnalysisModelEfficiencyItem
}

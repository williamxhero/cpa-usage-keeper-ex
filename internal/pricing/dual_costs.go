package pricing

import "cpa-usage-keeper/internal/helper"

// PriceEstimate is independent of the historical endpoint availability contract.
// Components and total retain only known subtotals; known zero is not missing.
type PriceEstimate struct {
	TotalCostUSD         *float64 `json:"total_cost_usd"`
	UncachedInputCostUSD float64  `json:"uncached_input_cost_usd"`
	OutputCostUSD        float64  `json:"output_cost_usd"`
	CacheReadCostUSD     float64  `json:"cache_read_cost_usd"`
	CacheWriteCostUSD    float64  `json:"cache_write_cost_usd"`
	Complete             bool     `json:"complete"`
	HasKnown             bool     `json:"has_known"`
	Status               string   `json:"status"`
	UnavailableReason    string   `json:"unavailable_reason,omitempty"`
}

type DualCosts struct {
	Configured PriceEstimate `json:"configured"`
	Reference  PriceEstimate `json:"reference"`
}

func estimate(cost helper.UsageTokenCostBreakdown, available bool, reason string) PriceEstimate {
	p := PriceEstimate{Complete: available, HasKnown: available, UnavailableReason: reason}
	if available {
		if !finiteBreakdown(cost) {
			return estimate(helper.UsageTokenCostBreakdown{}, false, "reference_overflow")
		}
		p.UncachedInputCostUSD, p.OutputCostUSD = cost.UncachedInputCostUSD, cost.OutputCostUSD
		p.CacheReadCostUSD, p.CacheWriteCostUSD = cost.CacheReadCostUSD, cost.CacheWriteCostUSD
		total := cost.TotalCostUSD
		p.TotalCostUSD = &total
		p.UnavailableReason = ""
	}
	p.setStatus()
	return p
}

func (p *PriceEstimate) setStatus() {
	switch {
	case p.Complete:
		p.Status = "complete"
	case p.HasKnown:
		p.Status = "partial"
	default:
		p.Status = "unavailable"
	}
}

// Normalized gives the existing no-usage known-zero convention without counting
// an empty accumulator as a priced request when its first constituent is missing.
func (p PriceEstimate) Normalized() PriceEstimate {
	if p.Status == "" {
		return estimate(helper.UsageTokenCostBreakdown{}, true, "")
	}
	if p.TotalCostUSD != nil {
		total := *p.TotalCostUSD
		p.TotalCostUSD = &total
	}
	return p
}
func (d DualCosts) Normalized() DualCosts {
	return DualCosts{d.Configured.Normalized(), d.Reference.Normalized()}
}

func (p *PriceEstimate) Merge(other PriceEstimate) {
	if other.Status == "" {
		return
	}
	if p.Status == "" {
		*p = other.Normalized()
		return
	}
	p.Complete = p.Complete && other.Complete
	if p.UnavailableReason == "" {
		p.UnavailableReason = other.UnavailableReason
	}
	if other.HasKnown {
		cost := helper.UsageTokenCostBreakdown{UncachedInputCostUSD: p.UncachedInputCostUSD + other.UncachedInputCostUSD, OutputCostUSD: p.OutputCostUSD + other.OutputCostUSD, CacheReadCostUSD: p.CacheReadCostUSD + other.CacheReadCostUSD, CacheWriteCostUSD: p.CacheWriteCostUSD + other.CacheWriteCostUSD}
		if p.TotalCostUSD != nil {
			cost.TotalCostUSD = *p.TotalCostUSD
		}
		if other.TotalCostUSD != nil {
			cost.TotalCostUSD += *other.TotalCostUSD
		}
		if finiteBreakdown(cost) {
			p.UncachedInputCostUSD, p.OutputCostUSD = cost.UncachedInputCostUSD, cost.OutputCostUSD
			p.CacheReadCostUSD, p.CacheWriteCostUSD = cost.CacheReadCostUSD, cost.CacheWriteCostUSD
			p.TotalCostUSD = &cost.TotalCostUSD
			p.HasKnown = true
		} else {
			p.Complete = false
			p.UnavailableReason = "reference_overflow"
		}
	}
	p.setStatus()
}
func (d *DualCosts) Merge(other DualCosts) {
	d.Configured.Merge(other.Configured)
	d.Reference.Merge(other.Reference)
}
func (d *DualCosts) Add(result CostResult) { d.Merge(result.DualCosts()) }
func (result CostResult) DualCosts() DualCosts {
	reason := result.UnavailableReason
	if !result.Available && reason == "" {
		reason = "missing_price"
	}
	reference := result.ReferenceEstimate
	if reference.Status == "" {
		reference = estimate(result.ReferenceCost, result.ReferenceAvailable, result.ReferenceUnavailableReason)
	}
	configured := result.ConfiguredEstimate
	if configured.Status == "" {
		configured = estimate(result.Cost, result.Available, reason)
	}
	return DualCosts{configured, reference}
}
func finiteBreakdown(cost helper.UsageTokenCostBreakdown) bool {
	for _, value := range []float64{cost.UncachedInputCostUSD, cost.OutputCostUSD, cost.CacheReadCostUSD, cost.CacheWriteCostUSD, cost.TotalCostUSD} {
		if !isNonNegativeFinite(value) {
			return false
		}
	}
	return true
}

// WithReference attaches independently reconciled retained-event reference facts.
func (result CostResult) WithReference(p PriceEstimate) CostResult {
	result.ReferenceEstimate = p
	result.ReferenceAvailable = p.Complete
	result.ReferenceUnavailableReason = p.UnavailableReason
	result.ReferenceCost = helper.UsageTokenCostBreakdown{UncachedInputCostUSD: p.UncachedInputCostUSD, OutputCostUSD: p.OutputCostUSD, CacheReadCostUSD: p.CacheReadCostUSD, CacheWriteCostUSD: p.CacheWriteCostUSD}
	if p.TotalCostUSD != nil {
		result.ReferenceCost.TotalCostUSD = *p.TotalCostUSD
	}
	return result
}

func (p PriceEstimate) Scale(factor float64) PriceEstimate {
	p = p.Normalized()
	p.UncachedInputCostUSD *= factor
	p.OutputCostUSD *= factor
	p.CacheReadCostUSD *= factor
	p.CacheWriteCostUSD *= factor
	if p.TotalCostUSD != nil {
		total := *p.TotalCostUSD * factor
		p.TotalCostUSD = &total
	}
	cost := helper.UsageTokenCostBreakdown{UncachedInputCostUSD: p.UncachedInputCostUSD, OutputCostUSD: p.OutputCostUSD, CacheReadCostUSD: p.CacheReadCostUSD, CacheWriteCostUSD: p.CacheWriteCostUSD}
	if p.TotalCostUSD != nil {
		cost.TotalCostUSD = *p.TotalCostUSD
	}
	if !isNonNegativeFinite(factor) || !finiteBreakdown(cost) {
		return estimate(helper.UsageTokenCostBreakdown{}, false, "reference_overflow")
	}
	return p
}
func (d DualCosts) Scale(factor float64) DualCosts {
	return DualCosts{d.Configured.Scale(factor), d.Reference.Scale(factor)}
}

// IncompleteReference clears unproven arithmetic; retained known subtotals are
// reconciled independently by WithReference without fabricating coverage.
func (result CostResult) IncompleteReference(reason string) CostResult {
	result.ReferenceEstimate = PriceEstimate{}
	result.ReferenceCost = helper.UsageTokenCostBreakdown{}
	result.ReferenceAvailable = false
	result.ReferenceUnavailableReason = reason
	return result
}

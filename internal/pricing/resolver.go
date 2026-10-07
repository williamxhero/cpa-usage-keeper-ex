package pricing

import (
	"cpa-usage-keeper/internal/helper"
)

// CostSubject 是所有 usage 来源进入计价领域的唯一固定输入。
type CostSubject struct {
	Dimensions UsageDimensions
	Tokens     helper.UsageTokenCostInput
	// Exact identity evidence is independent of normalized legacy rule fields.
	// ObservedIdentity distinguishes unknown raw type from a type-less rollup.
	AuthType          string
	IdentityAuthIndex string
	ObservedIdentity  bool
}

func NewCostSubject(dimensions UsageDimensions, tokens helper.UsageTokenCostInput) CostSubject {
	return CostSubject{
		Dimensions:        canonicalizeUsageDimensions(dimensions),
		Tokens:            tokens,
		IdentityAuthIndex: dimensions.AuthIndex,
	}
}

type CostResult struct {
	Cost                helper.UsageTokenCostBreakdown
	Available           bool
	PricingStyle        string
	MatchedModel        string
	MatchedBy           string
	RuleMultiplier      float64
	CredentialSubjectID string
	Scope               string
	Multiplier          *float64
	UnavailableReason   string
	SelectedModel       string
	SelectedBy          string
	Mode                string
	Fixed               *FixedTariff
	ReferenceCost       helper.UsageTokenCostBreakdown
	ReferenceAvailable  bool
}

// Resolver 在创建时固定绑定一个 Snapshot，确保单个响应不会混用新旧价格。
type Resolver struct {
	snapshot *Snapshot
}

func (r Resolver) ActiveFields() ActiveFields {
	if r.snapshot == nil {
		return 0
	}
	return r.snapshot.activeFields
}

func (r Resolver) Calculate(subject CostSubject) CostResult {
	model, matchedModel, matchedBy, found := r.matchModel(subject.Dimensions)
	if config, selectedBy, selected := r.credentialModel(subject); selected {
		var result CostResult
		if config.Mode == ModeFixed {
			style := config.Fixed.PricingStyle
			if found {
				style = model.pricing.PricingStyle
			}
			result = CostResult{Available: true, PricingStyle: style, CredentialSubjectID: config.SubjectID, RuleMultiplier: 1, Fixed: cloneFixed(config.Fixed), MatchedModel: matchedModel, MatchedBy: matchedBy}
			result.Cost = helper.CalculateUsageTokenCostBreakdown(subject.Tokens, config.Fixed.pricing(style))
		} else {
			result = calculateCredentialDefault(subject, config.SubjectID, config.Multiplier, model, matchedModel, matchedBy, found)
		}
		result.Scope, result.Mode = "credential_model", config.Mode
		result.SelectedModel, result.SelectedBy = config.Model, selectedBy
		return withBaselineReference(result, subject, model, found)
	}
	if id, multiplier, selected := r.credentialDefault(subject); selected {
		return withBaselineReference(calculateCredentialDefault(subject, id, multiplier, model, matchedModel, matchedBy, found), subject, model, found)
	}
	result := r.CalculateLegacy(subject)
	if r.HasPricingOverrides() {
		result = withBaselineReference(result, subject, model, found)
	}
	return result
}

func withBaselineReference(result CostResult, subject CostSubject, model compiledModel, found bool) CostResult {
	result.ReferenceAvailable = found || !helper.UsageTokenInputRequiresPricing(subject.Tokens)
	if found {
		result.ReferenceCost = helper.CalculateUsageTokenCostBreakdown(subject.Tokens, unadjustedPricing(model))
	}
	return result
}

func calculateCredentialDefault(subject CostSubject, id string, multiplier float64, model compiledModel, matchedModel, matchedBy string, found bool) CostResult {
	result := CostResult{Available: !helper.UsageTokenInputRequiresPricing(subject.Tokens), RuleMultiplier: 1, CredentialSubjectID: id, Scope: "credential_default", Mode: ModeMultiplier, Multiplier: &multiplier, MatchedModel: matchedModel, MatchedBy: matchedBy}
	if !found {
		if !result.Available {
			result.UnavailableReason = "missing_baseline"
		}
		return result
	}
	result.Available = true
	result.PricingStyle = model.pricing.PricingStyle
	result.Cost = helper.ScaleUsageTokenCostBreakdown(helper.CalculateUsageTokenCostBreakdown(subject.Tokens, unadjustedPricing(model)), multiplier)
	return result
}

// CalculateLegacy preserves legacy grouping for subjects already rejected by
// exact typed attribution. It never retries a type-less credential lookup.
func (r Resolver) CalculateLegacy(subject CostSubject) CostResult {
	model, matchedModel, matchedBy, found := r.matchModel(subject.Dimensions)
	if !found {
		return CostResult{
			Available:      !helper.UsageTokenInputRequiresPricing(subject.Tokens),
			RuleMultiplier: 1,
		}
	}

	breakdown := helper.CalculateUsageTokenCostBreakdown(subject.Tokens, model.pricing)
	ruleMultiplier := 1.0
	if model.pricing.PriceMultiplier == nil || *model.pricing.PriceMultiplier != 0 {
		ruleMultiplier = matchingRuleMultiplier(model.rules, subject.Dimensions)
		breakdown = helper.ScaleUsageTokenCostBreakdown(breakdown, ruleMultiplier)
	}
	return CostResult{
		Cost:           breakdown,
		Available:      true,
		PricingStyle:   model.pricing.PricingStyle,
		MatchedModel:   matchedModel,
		MatchedBy:      matchedBy,
		RuleMultiplier: ruleMultiplier,
	}
}

func (r Resolver) matchModel(dimensions UsageDimensions) (compiledModel, string, string, bool) {
	if r.snapshot == nil {
		return compiledModel{}, "", "", false
	}
	if model, ok := r.snapshot.modelsByName[dimensions.Model]; ok {
		return model, dimensions.Model, "model", true
	}
	if model, ok := r.snapshot.modelsByName[dimensions.ModelAlias]; ok {
		return model, dimensions.ModelAlias, "model_alias", true
	}
	return compiledModel{}, "", "", false
}

func matchingRuleMultiplier(rules []compiledRule, dimensions UsageDimensions) float64 {
	multiplier := 1.0
	for _, rule := range rules {
		if dimensions.Value(rule.field) != rule.value {
			continue
		}
		if rule.multiplier == 0 {
			return 0
		}
		multiplier *= rule.multiplier
	}
	return multiplier
}

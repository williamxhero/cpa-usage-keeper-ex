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
	Cost                       helper.UsageTokenCostBreakdown
	Available                  bool
	PricingStyle               string
	MatchedModel               string
	MatchedBy                  string
	RuleMultiplier             float64
	CredentialSubjectID        string
	Scope                      string
	Multiplier                 *float64
	UnavailableReason          string
	ChannelID                  string
	ChannelName                string
	AttributionWarning         string
	SelectedModel              string
	SelectedBy                 string
	Mode                       string
	Fixed                      *FixedTariff
	ReferenceCost              helper.UsageTokenCostBreakdown
	ReferenceAvailable         bool
	ConfiguredEstimate         PriceEstimate
	ReferenceEstimate          PriceEstimate
	ReferenceUnavailableReason string
	LegacyModelMultiplier      float64
	LegacyRuleMultiplier       float64
	FinalMultiplier            float64
	matchedRules               [ruleFieldCount]MatchedRule
	matchedRuleCount           int
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
	var result CostResult
	if config, selectedBy, selected := r.credentialModel(subject); selected {
		result = calculateModelOverride(subject, config.SubjectID, config.Mode, config.Multiplier, config.Fixed, model, matchedModel, matchedBy, found)
		result.Scope, result.Mode = "credential_model", config.Mode
		result.SelectedModel, result.SelectedBy = config.Model, selectedBy
	} else if id, multiplier, selected := r.credentialDefault(subject); selected {
		result = calculateCredentialDefault(subject, id, multiplier, model, matchedModel, matchedBy, found)
	} else if config, selectedBy, selected := r.channelModel(subject); selected {
		result = calculateModelOverride(subject, r.credentialSubject(subject), config.Mode, config.Multiplier, config.Fixed, model, matchedModel, matchedBy, found)
		result.Scope, result.Mode = "channel_model", config.Mode
		result.SelectedModel, result.SelectedBy = config.Model, selectedBy
	} else if _, multiplier, selected := r.channelDefault(subject); selected {
		result = calculateCredentialDefault(subject, r.credentialSubject(subject), multiplier, model, matchedModel, matchedBy, found)
		result.Scope = "channel_default"
	} else {
		result = r.CalculateLegacy(subject)
	}
	result = withBaselineReference(result, subject, model, found)
	if result.CredentialSubjectID == "" {
		result.CredentialSubjectID = r.credentialSubject(subject)
	}
	result.LegacyModelMultiplier = 1
	if found && model.pricing.PriceMultiplier != nil {
		result.LegacyModelMultiplier = *model.pricing.PriceMultiplier
	}
	result.LegacyRuleMultiplier = result.RuleMultiplier
	result.FinalMultiplier = result.LegacyModelMultiplier * result.RuleMultiplier
	if result.Scope != "" {
		// Explain replaced adjustments from the same pinned baseline, without
		// changing the selected configured result or its historical fields.
		legacy := r.CalculateLegacy(subject)
		result.LegacyRuleMultiplier = legacy.RuleMultiplier
		result.matchedRules, result.matchedRuleCount = legacy.matchedRules, legacy.matchedRuleCount
		result.FinalMultiplier = 1
		if result.Multiplier != nil {
			result.FinalMultiplier = *result.Multiplier
		}
	}
	result.ChannelID, result.ChannelName, result.AttributionWarning = r.ChannelAttribution(subject)
	return result
}

// Scope selection precedes this shared calculator; missing baselines never
// cause a selected multiplier to fall through to a lower fixed configuration.
func calculateModelOverride(subject CostSubject, id, mode string, multiplier float64, fixed *FixedTariff, model compiledModel, matchedModel, matchedBy string, found bool) CostResult {
	if mode != ModeFixed {
		return calculateCredentialDefault(subject, id, multiplier, model, matchedModel, matchedBy, found)
	}
	style := fixed.PricingStyle
	if found {
		style = model.pricing.PricingStyle
	}
	result := CostResult{Available: true, PricingStyle: style, CredentialSubjectID: id, RuleMultiplier: 1, Fixed: cloneFixed(fixed), MatchedModel: matchedModel, MatchedBy: matchedBy}
	result.Cost = helper.CalculateUsageTokenCostBreakdown(subject.Tokens, fixed.pricing(style))
	return result
}

func withBaselineReference(result CostResult, subject CostSubject, model compiledModel, found bool) CostResult {
	result.ReferenceAvailable = found || !helper.UsageTokenInputRequiresPricing(subject.Tokens)
	if found {
		result.ReferenceCost = helper.CalculateUsageTokenCostBreakdown(subject.Tokens, unadjustedPricing(model))
		if !finiteBreakdown(result.ReferenceCost) {
			return result.IncompleteReference("reference_overflow")
		}
	}
	if !result.ReferenceAvailable {
		result.ReferenceUnavailableReason = "missing_baseline"
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
		return withBaselineReference(CostResult{
			Available:      !helper.UsageTokenInputRequiresPricing(subject.Tokens),
			RuleMultiplier: 1, LegacyModelMultiplier: 1, LegacyRuleMultiplier: 1, FinalMultiplier: 1,
		}, subject, model, false)
	}

	breakdown := helper.CalculateUsageTokenCostBreakdown(subject.Tokens, model.pricing)
	ruleMultiplier := 1.0
	if model.pricing.PriceMultiplier == nil || *model.pricing.PriceMultiplier != 0 {
		ruleMultiplier = matchingRuleMultiplier(model.rules, subject.Dimensions)
		breakdown = helper.ScaleUsageTokenCostBreakdown(breakdown, ruleMultiplier)
	}
	modelMultiplier := 1.0
	if model.pricing.PriceMultiplier != nil {
		modelMultiplier = *model.pricing.PriceMultiplier
	}
	var matchedRules [ruleFieldCount]MatchedRule
	matchedRuleCount := 0
	if modelMultiplier != 0 {
		for _, rule := range model.rules {
			if subject.Dimensions.Value(rule.field) == rule.value {
				matchedRules[matchedRuleCount] = MatchedRule{Key: rule.field.String(), Multiplier: rule.multiplier}
				matchedRuleCount++
			}
		}
	}
	return withBaselineReference(CostResult{
		Cost: breakdown, Available: true, PricingStyle: model.pricing.PricingStyle,
		MatchedModel: matchedModel, MatchedBy: matchedBy, RuleMultiplier: ruleMultiplier,
		LegacyModelMultiplier: modelMultiplier, LegacyRuleMultiplier: ruleMultiplier, FinalMultiplier: modelMultiplier * ruleMultiplier, matchedRules: matchedRules, matchedRuleCount: matchedRuleCount,
	}, subject, model, true)
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

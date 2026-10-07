package pricing

// CostSelection is safe request-level evidence from the very snapshot that
// calculated the cost. Override matching is distinct from baseline matching.
type MatchedRule struct {
	Key        string  `json:"key"`
	Multiplier float64 `json:"multiplier"`
}

type CostSelection struct {
	ChannelID                 string        `json:"channel_id,omitempty"`
	ChannelName               string        `json:"channel_name,omitempty"`
	AttributionWarning        string        `json:"attribution_warning,omitempty"`
	Legacy                    bool          `json:"legacy"`
	LegacyAdjustmentsReplaced bool          `json:"legacy_adjustments_replaced"`
	LegacyModelMultiplier     float64       `json:"legacy_model_multiplier"`
	LegacyRuleMultiplier      float64       `json:"legacy_rule_multiplier"`
	FinalMultiplier           float64       `json:"final_multiplier"`
	MatchedRules              []MatchedRule `json:"matched_rules"`
	DualCosts                 DualCosts     `json:"dual_costs"`
	Scope                     string        `json:"scope"`
	SubjectID                 string        `json:"subject_id,omitempty"`
	SubjectName               string        `json:"subject_name,omitempty"`
	SelectedModel             string        `json:"selected_model,omitempty"`
	SelectedBy                string        `json:"selected_by,omitempty"`
	BaselineModel             string        `json:"baseline_model,omitempty"`
	BaselineBy                string        `json:"baseline_by,omitempty"`
	Multiplier                *float64      `json:"multiplier,omitempty"`
	UnavailableReason         string        `json:"unavailable_reason,omitempty"`
	SnapshotID                string        `json:"snapshot_id"`
	Mode                      string        `json:"mode"`
	Fixed                     *FixedTariff  `json:"fixed,omitempty"`
	PricingStyle              string        `json:"pricing_style,omitempty"`
	BaselineCostUSD           *float64      `json:"baseline_cost_usd"`
	BaselineAvailable         bool          `json:"baseline_available"`
	BaselineUnavailableReason string        `json:"baseline_unavailable_reason,omitempty"`
}

func (r Resolver) Selection(result CostResult) *CostSelection {
	scope := result.Scope
	if scope == "" {
		scope = "legacy"
	}
	mode := result.Mode
	if mode == "" {
		mode = "legacy"
	}
	var reference *float64
	reason := ""
	if result.ReferenceAvailable {
		value := result.ReferenceCost.TotalCostUSD
		reference = &value
	} else {
		reason = result.ReferenceUnavailableReason
	}
	rules := append([]MatchedRule{}, result.matchedRules[:result.matchedRuleCount]...)
	return &CostSelection{ChannelID: result.ChannelID, ChannelName: result.ChannelName, AttributionWarning: result.AttributionWarning, Legacy: scope == "legacy", LegacyAdjustmentsReplaced: scope != "legacy", LegacyModelMultiplier: result.LegacyModelMultiplier, LegacyRuleMultiplier: result.LegacyRuleMultiplier, FinalMultiplier: result.FinalMultiplier, MatchedRules: rules, DualCosts: result.DualCosts(), Mode: mode, Fixed: cloneFixed(result.Fixed), PricingStyle: result.PricingStyle, BaselineCostUSD: reference, BaselineAvailable: result.ReferenceAvailable, BaselineUnavailableReason: reason, Scope: scope, SubjectID: result.CredentialSubjectID, SubjectName: r.snapshot.CredentialName(result.CredentialSubjectID), SelectedModel: result.SelectedModel, SelectedBy: result.SelectedBy, BaselineModel: result.MatchedModel, BaselineBy: result.MatchedBy, Multiplier: result.Multiplier, UnavailableReason: result.UnavailableReason, SnapshotID: r.SnapshotID()}
}

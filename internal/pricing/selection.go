package pricing

// CostSelection is safe request-level evidence from the very snapshot that
// calculated the cost. Override matching is distinct from baseline matching.
type CostSelection struct {
	Scope                     string       `json:"scope"`
	SubjectID                 string       `json:"subject_id,omitempty"`
	SubjectName               string       `json:"subject_name,omitempty"`
	SelectedModel             string       `json:"selected_model,omitempty"`
	SelectedBy                string       `json:"selected_by,omitempty"`
	BaselineModel             string       `json:"baseline_model,omitempty"`
	BaselineBy                string       `json:"baseline_by,omitempty"`
	Multiplier                *float64     `json:"multiplier,omitempty"`
	UnavailableReason         string       `json:"unavailable_reason,omitempty"`
	SnapshotID                string       `json:"snapshot_id"`
	Mode                      string       `json:"mode"`
	Fixed                     *FixedTariff `json:"fixed,omitempty"`
	PricingStyle              string       `json:"pricing_style,omitempty"`
	BaselineCostUSD           *float64     `json:"baseline_cost_usd"`
	BaselineAvailable         bool         `json:"baseline_available"`
	BaselineUnavailableReason string       `json:"baseline_unavailable_reason,omitempty"`
}

func (r Resolver) Selection(result CostResult) *CostSelection {
	if !r.HasPricingOverrides() {
		return nil
	}
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
		reason = "missing_baseline"
	}
	return &CostSelection{Mode: mode, Fixed: cloneFixed(result.Fixed), PricingStyle: result.PricingStyle, BaselineCostUSD: reference, BaselineAvailable: result.ReferenceAvailable, BaselineUnavailableReason: reason, Scope: scope, SubjectID: result.CredentialSubjectID, SubjectName: r.snapshot.CredentialName(result.CredentialSubjectID), SelectedModel: result.SelectedModel, SelectedBy: result.SelectedBy, BaselineModel: result.MatchedModel, BaselineBy: result.MatchedBy, Multiplier: result.Multiplier, UnavailableReason: result.UnavailableReason, SnapshotID: r.SnapshotID()}
}

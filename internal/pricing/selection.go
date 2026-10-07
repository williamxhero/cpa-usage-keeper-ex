package pricing

// CostSelection is safe request-level evidence from the very snapshot that
// calculated the cost. Override matching is distinct from baseline matching.
type CostSelection struct {
	Scope             string   `json:"scope"`
	SubjectID         string   `json:"subject_id,omitempty"`
	SubjectName       string   `json:"subject_name,omitempty"`
	SelectedModel     string   `json:"selected_model,omitempty"`
	SelectedBy        string   `json:"selected_by,omitempty"`
	BaselineModel     string   `json:"baseline_model,omitempty"`
	BaselineBy        string   `json:"baseline_by,omitempty"`
	Multiplier        *float64 `json:"multiplier,omitempty"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
	SnapshotID        string   `json:"snapshot_id"`
}

func (r Resolver) Selection(result CostResult) *CostSelection {
	if !r.HasPricingOverrides() {
		return nil
	}
	scope := result.Scope
	if scope == "" {
		scope = "legacy"
	}
	return &CostSelection{Scope: scope, SubjectID: result.CredentialSubjectID, SubjectName: r.snapshot.CredentialName(result.CredentialSubjectID), SelectedModel: result.SelectedModel, SelectedBy: result.SelectedBy, BaselineModel: result.MatchedModel, BaselineBy: result.MatchedBy, Multiplier: result.Multiplier, UnavailableReason: result.UnavailableReason, SnapshotID: r.SnapshotID()}
}

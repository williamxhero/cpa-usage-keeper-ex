package service

import (
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/service/dto"
)

// Effective cells are read-only projections, never persisted coverage facts.
type CredentialMatrixEffectiveCell struct {
	CredentialMatrixCell
	Status            string `json:"status"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	MatchedModel      string `json:"matched_model,omitempty"`
}

func matrixEffectiveCells(snapshot *pricing.Snapshot, credentials []dto.PricingCredential, models []string, allowedPairs map[credentialMatrixPair]bool, secrets []string) []CredentialMatrixEffectiveCell {
	result := make([]CredentialMatrixEffectiveCell, 0, len(credentials)*len(models))
	// Match each model once from the very same pinned snapshot as sparse overrides.
	official := snapshot.OfficialPrices()
	inherited := make(map[string]CredentialMatrixEffectiveCell, len(models))
	for _, model := range models {
		cell := CredentialMatrixEffectiveCell{CredentialMatrixCell: CredentialMatrixCell{CanonicalModelID: model, Mode: "inherit", Source: "unavailable", SnapshotID: snapshot.ID()}, Status: "unavailable"}
		match := pricing.MatchOfficialPrice(official, model, nil)
		if match.Price != nil && safeCredentialMatrixModel(match.Price.CanonicalModelID, secrets) {
			price := match.Price
			cell.PricingStyle, cell.Source, cell.MatchedModel = price.PricingStyle, price.Source, price.CanonicalModelID
			cell.PromptPricePer1M, cell.CompletionPricePer1M = price.PromptPricePer1M, price.CompletionPricePer1M
			cell.CacheReadPricePer1M, cell.CacheWritePricePer1M = price.CacheReadPricePer1M, price.CacheWritePricePer1M
			known := 0
			for _, value := range []*float64{cell.PromptPricePer1M, cell.CompletionPricePer1M, cell.CacheReadPricePer1M, cell.CacheWritePricePer1M} {
				if value != nil {
					known++
				}
			}
			switch known {
			case 4:
				cell.Status = "complete"
			case 0:
				cell.UnavailableReason = "missing_segments"
			default:
				cell.Status, cell.UnavailableReason = "partial", "missing_segments"
			}
		} else {
			cell.UnavailableReason = match.Reason
			if cell.UnavailableReason == "" {
				cell.UnavailableReason = "no_match"
			}
		}
		inherited[model] = cell
	}
	for index := range credentials {
		credential := &credentials[index]
		for _, model := range models {
			if allowedPairs != nil && !allowedPairs[credentialMatrixPair{credential.SubjectID, model}] {
				continue
			}
			cell := inherited[model]
			cell.SubjectID = credential.SubjectID
			cell.Credential = credential
			if config, ok := snapshot.CredentialMatrixConfig(credential.SubjectID, model); ok {
				cell = CredentialMatrixEffectiveCell{CredentialMatrixCell: matrixCellFromConfig(config, snapshot.ID()), Status: "complete", MatchedModel: model}
				cell.Credential = credential
			}
			result = append(result, cell)
		}
	}
	return result
}

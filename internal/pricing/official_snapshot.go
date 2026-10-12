package pricing

import "cpa-usage-keeper/internal/entities"

// SetOfficialPrices attaches a defensive copy of the persisted official
// catalog before the snapshot is published. Callers must not mutate a live
// snapshot after publication.
func (s *Snapshot) SetOfficialPrices(prices []entities.OfficialModelPrice) {
	if s == nil {
		return
	}
	s.officialPrices = cloneOfficialPrices(prices)
}

// OfficialPrices returns the official catalog captured by this immutable
// snapshot. Nested aliases and price pointers are copied for caller safety.
func (s *Snapshot) OfficialPrices() []entities.OfficialModelPrice {
	if s == nil {
		return []entities.OfficialModelPrice{}
	}
	return cloneOfficialPrices(s.officialPrices)
}

func cloneOfficialPrices(prices []entities.OfficialModelPrice) []entities.OfficialModelPrice {
	if prices == nil {
		return nil
	}
	result := make([]entities.OfficialModelPrice, len(prices))
	for index, price := range prices {
		result[index] = price
		result[index].Aliases = append([]entities.OfficialModelPriceAlias(nil), price.Aliases...)
		result[index].PromptPricePer1M = cloneOfficialPricePointer(price.PromptPricePer1M)
		result[index].CompletionPricePer1M = cloneOfficialPricePointer(price.CompletionPricePer1M)
		result[index].CacheReadPricePer1M = cloneOfficialPricePointer(price.CacheReadPricePer1M)
		result[index].CacheWritePricePer1M = cloneOfficialPricePointer(price.CacheWritePricePer1M)
	}
	return result
}

func cloneOfficialPricePointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

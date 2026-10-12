package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/pricingmetadata"
	"cpa-usage-keeper/internal/repository"
	"gorm.io/gorm"
)

const maxOfficialPricingBatch = 5000

var (
	ErrOfficialPricingPreviewNotFound = errors.New("official pricing preview not found")
	ErrOfficialPricingBatchTooLarge   = errors.New("official pricing batch exceeds limit")
)

// OfficialPricingInput is the complete mutable representation of one official
// model record. Nil prices deliberately preserve an upstream missing segment.
type OfficialPricingInput struct {
	CanonicalModelID     string
	DisplayModelID       string
	Source               string
	PricingStyle         string
	SourceURL            string
	FetchedAt            *time.Time
	ResponseHash         string
	SourceVersion        string
	PromptPricePer1M     *float64
	CompletionPricePer1M *float64
	CacheReadPricePer1M  *float64
	CacheWritePricePer1M *float64
	Aliases              []string
}

type OfficialPricingRefreshPreview struct {
	PreviewID    string                        `json:"preview_id"`
	SourceID     string                        `json:"source_id"`
	Source       string                        `json:"source"`
	SourceURL    string                        `json:"source_url"`
	FetchedAt    time.Time                     `json:"fetched_at"`
	ResponseHash string                        `json:"response_hash"`
	Prices       []entities.OfficialModelPrice `json:"prices"`
	Stale        bool                          `json:"stale"`
}

type OfficialPricingProvider interface {
	ListOfficialPricing(context.Context) ([]entities.OfficialModelPrice, error)
	UpdateOfficialPricing(context.Context, OfficialPricingInput) (*entities.OfficialModelPrice, error)
	UpdateOfficialPricingBatch(context.Context, []OfficialPricingInput) ([]entities.OfficialModelPrice, error)
	PreviewOfficialPricingRefresh(context.Context, string) (OfficialPricingRefreshPreview, error)
	CommitOfficialPricingRefresh(context.Context, string) ([]entities.OfficialModelPrice, error)
}

type officialRefreshCandidate struct {
	preview OfficialPricingRefreshPreview
}

func (s *pricingService) ListOfficialPricing(context.Context) ([]entities.OfficialModelPrice, error) {
	return repository.ListOfficialModelPrices(s.db)
}

func (s *pricingService) UpdateOfficialPricing(ctx context.Context, input OfficialPricingInput) (*entities.OfficialModelPrice, error) {
	rows, err := s.UpdateOfficialPricingBatch(ctx, []OfficialPricingInput{input})
	if err != nil {
		return nil, err
	}
	return &rows[0], nil
}

func (s *pricingService) UpdateOfficialPricingBatch(ctx context.Context, inputs []OfficialPricingInput) ([]entities.OfficialModelPrice, error) {
	if len(inputs) > maxOfficialPricingBatch {
		return nil, fmt.Errorf("%w: maximum is %d", ErrOfficialPricingBatchTooLarge, maxOfficialPricingBatch)
	}
	normalized := make([]entities.OfficialModelPrice, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for index, input := range inputs {
		row, key, err := normalizeOfficialPricingInput(input)
		if err != nil {
			return nil, fmt.Errorf("%w: official pricing at index %d: %v", ErrInvalidPricingInput, index, err)
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: official pricing at index %d: duplicate canonical_model_id", ErrInvalidPricingInput, index)
		}
		seen[key] = struct{}{}
		normalized[index] = row
	}
	if len(normalized) == 0 {
		return []entities.OfficialModelPrice{}, nil
	}

	if ctx == nil {
		ctx = context.Background()
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	var rows []entities.OfficialModelPrice
	var candidate *pricing.Snapshot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		rows, err = repository.UpsertOfficialModelPrices(tx, normalized)
		if err != nil {
			return err
		}
		candidate, err = repository.LoadPricingSnapshot(ctx, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	// The transaction has committed; publish the complete immutable snapshot
	// only after both legacy and official catalog inputs were loaded together.
	s.catalog.Replace(candidate)
	return rows, nil
}

func normalizeOfficialPricingInput(input OfficialPricingInput) (entities.OfficialModelPrice, string, error) {
	canonical := strings.TrimSpace(input.CanonicalModelID)
	if canonical == "" {
		return entities.OfficialModelPrice{}, "", fmt.Errorf("canonical_model_id is required")
	}
	style := strings.ToLower(strings.TrimSpace(input.PricingStyle))
	if style == "" {
		style = entities.ModelPricingStyleOpenAI
	}
	if style != entities.ModelPricingStyleOpenAI && style != entities.ModelPricingStyleClaude {
		return entities.OfficialModelPrice{}, "", fmt.Errorf("pricing_style must be openai or claude")
	}
	source := strings.ToLower(strings.TrimSpace(input.Source))
	if source == "" {
		source = entities.OfficialModelPriceSourceOfficial
	}
	if source != entities.OfficialModelPriceSourceOfficial && source != entities.OfficialModelPriceSourceOpenRouter && source != entities.OfficialModelPriceSourceLegacySeed {
		return entities.OfficialModelPrice{}, "", fmt.Errorf("source is not supported")
	}
	if err := validateOfficialURL(input.SourceURL); err != nil {
		return entities.OfficialModelPrice{}, "", err
	}
	for _, value := range []*float64{input.PromptPricePer1M, input.CompletionPricePer1M, input.CacheReadPricePer1M, input.CacheWritePricePer1M} {
		if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return entities.OfficialModelPrice{}, "", fmt.Errorf("prices must be finite and non-negative")
		}
	}
	aliases := make([]entities.OfficialModelPriceAlias, 0, len(input.Aliases))
	seenAliases := make(map[string]struct{}, len(input.Aliases))
	for _, alias := range input.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		key := strings.ToLower(alias)
		if _, exists := seenAliases[key]; exists || key == strings.ToLower(canonical) {
			continue
		}
		seenAliases[key] = struct{}{}
		aliases = append(aliases, entities.OfficialModelPriceAlias{Alias: alias})
	}
	return entities.OfficialModelPrice{
		CanonicalModelID: canonical, DisplayModelID: strings.TrimSpace(input.DisplayModelID), Source: source,
		PricingStyle: style, SourceURL: strings.TrimSpace(input.SourceURL), FetchedAt: input.FetchedAt,
		ResponseHash: strings.TrimSpace(input.ResponseHash), SourceVersion: strings.TrimSpace(input.SourceVersion),
		PromptPricePer1M: input.PromptPricePer1M, CompletionPricePer1M: input.CompletionPricePer1M,
		CacheReadPricePer1M: input.CacheReadPricePer1M, CacheWritePricePer1M: input.CacheWritePricePer1M,
		Aliases: aliases,
	}, strings.ToLower(canonical), nil
}

func validateOfficialURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("source_url must be a public https URL without credentials or query parameters")
	}
	return nil
}

func (s *pricingService) PreviewOfficialPricingRefresh(ctx context.Context, sourceID string) (OfficialPricingRefreshPreview, error) {
	if _, err := pricingmetadata.SourceByID(sourceID); err != nil {
		return OfficialPricingRefreshPreview{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	catalog, err := s.metadataClient.Fetch(ctx, sourceID)
	if err != nil {
		_ = repository.MarkOfficialModelPricesStale(s.db, nil)
		return OfficialPricingRefreshPreview{}, err
	}
	persisted, persistedErr := repository.ListOfficialModelPrices(s.db)
	if persistedErr != nil {
		return OfficialPricingRefreshPreview{}, persistedErr
	}
	entries := catalog.Entries
	var fallbackCatalog *pricingmetadata.Catalog
	if catalog.Source.ID != pricingmetadata.SourceOpenRouter {
		// OpenRouter is a fallback catalog only. A failed fallback does not
		// replace the last successful persisted snapshot.
		fallback, fallbackErr := s.metadataClient.Fetch(ctx, pricingmetadata.SourceOpenRouter)
		if fallbackErr != nil {
			_ = repository.MarkOfficialModelPricesStale(s.db, nil)
			return OfficialPricingRefreshPreview{}, fallbackErr
		}
		fallbackCatalog = &fallback
		entries = mergeOfficialAndOpenRouterEntries(entries, fallback.Entries, persisted)
	} else {
		entries = filterOpenRouterEntries(entries, persisted)
	}
	if err := validateRefreshEntries(catalog.Source.ID, entries); err != nil {
		_ = repository.MarkOfficialModelPricesStale(s.db, nil)
		return OfficialPricingRefreshPreview{}, err
	}
	prices := make([]entities.OfficialModelPrice, 0, len(entries))
	for _, entry := range entries {
		id := strings.TrimSpace(entry.Model.ID)
		if id == "" {
			continue
		}
		metadataCatalog := catalog
		if fallbackCatalog != nil && sourceForMetadata(catalog.Source.ID, entry) == entities.OfficialModelPriceSourceOpenRouter {
			metadataCatalog = *fallbackCatalog
		}
		price := entities.OfficialModelPrice{
			CanonicalModelID: id, DisplayModelID: displayMetadataModel(entry.Model),
			Source:       sourceForMetadata(metadataCatalog.Source.ID, entry),
			PricingStyle: metadataPricingStyle(entry.Model), SourceURL: sourceURLForMetadata(metadataCatalog, entry),
			FetchedAt: timePointer(metadataCatalog.FetchedAt), ResponseHash: metadataCatalog.ResponseHash,
			PromptPricePer1M: entry.Model.Cost.Input, CompletionPricePer1M: entry.Model.Cost.Output,
			CacheReadPricePer1M: entry.Model.Cost.CacheRead, CacheWritePricePer1M: entry.Model.Cost.CacheWrite,
		}
		price.SourceVersion = strings.TrimSpace(entry.Model.LastUpdated)
		price.MissingSegments = officialMissingSegments(price)
		prices = append(prices, price)
	}
	sort.Slice(prices, func(i, j int) bool {
		return strings.ToLower(prices[i].CanonicalModelID) < strings.ToLower(prices[j].CanonicalModelID)
	})
	preview := OfficialPricingRefreshPreview{PreviewID: newPreviewID(), SourceID: catalog.Source.ID, Source: catalog.Source.Name, SourceURL: catalog.Source.URL, FetchedAt: catalog.FetchedAt, ResponseHash: catalog.ResponseHash, Prices: prices}
	s.officialMu.Lock()
	if s.officialPreviews == nil {
		s.officialPreviews = make(map[string]officialRefreshCandidate)
	}
	s.officialPreviews[preview.PreviewID] = officialRefreshCandidate{preview: preview}
	s.officialMu.Unlock()
	return cloneOfficialPreview(preview), nil
}

func mergeOfficialAndOpenRouterEntries(official, fallback []pricingmetadata.Entry, persisted []entities.OfficialModelPrice) []pricingmetadata.Entry {
	result := append([]pricingmetadata.Entry(nil), official...)
	seen := make(map[string]struct{}, len(official)+len(persisted))
	for _, entry := range official {
		if id := strings.ToLower(strings.TrimSpace(entry.Model.ID)); id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, row := range persisted {
		if id := strings.ToLower(strings.TrimSpace(row.CanonicalModelID)); id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, entry := range fallback {
		id := strings.ToLower(strings.TrimSpace(entry.Model.ID))
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, entry)
	}
	return result
}

func filterOpenRouterEntries(entries []pricingmetadata.Entry, persisted []entities.OfficialModelPrice) []pricingmetadata.Entry {
	protected := make(map[string]struct{}, len(persisted))
	for _, row := range persisted {
		if row.Source != entities.OfficialModelPriceSourceOpenRouter {
			if id := strings.ToLower(strings.TrimSpace(row.CanonicalModelID)); id != "" {
				protected[id] = struct{}{}
			}
		}
	}
	result := make([]pricingmetadata.Entry, 0, len(entries))
	for _, entry := range entries {
		id := strings.ToLower(strings.TrimSpace(entry.Model.ID))
		if id == "" {
			continue
		}
		if _, exists := protected[id]; exists {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func officialMissingSegments(price entities.OfficialModelPrice) string {
	missing := make([]string, 0, 4)
	if price.PromptPricePer1M == nil {
		missing = append(missing, "prompt")
	}
	if price.CompletionPricePer1M == nil {
		missing = append(missing, "completion")
	}
	if price.CacheReadPricePer1M == nil {
		missing = append(missing, "cache_read")
	}
	if price.CacheWritePricePer1M == nil {
		missing = append(missing, "cache_write")
	}
	return strings.Join(missing, ",")
}

func validateRefreshEntries(sourceID string, entries []pricingmetadata.Entry) error {
	for _, entry := range entries {
		id := strings.TrimSpace(entry.Model.ID)
		if id == "" {
			return fmt.Errorf("pricing catalog contains a model without an ID")
		}
		if entry.Model.Cost.Input == nil || entry.Model.Cost.Output == nil {
			return fmt.Errorf("pricing catalog model %q is missing prompt or completion pricing", id)
		}
		for _, value := range []*float64{entry.Model.Cost.Input, entry.Model.Cost.Output, entry.Model.Cost.CacheRead, entry.Model.Cost.CacheWrite} {
			if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
				return fmt.Errorf("pricing catalog model %q contains an invalid price", id)
			}
		}
	}
	return nil
}

func (s *pricingService) CommitOfficialPricingRefresh(ctx context.Context, previewID string) ([]entities.OfficialModelPrice, error) {
	s.officialMu.Lock()
	candidate, ok := s.officialPreviews[strings.TrimSpace(previewID)]
	if ok {
		delete(s.officialPreviews, strings.TrimSpace(previewID))
	}
	s.officialMu.Unlock()
	if !ok {
		return nil, ErrOfficialPricingPreviewNotFound
	}
	inputs := make([]OfficialPricingInput, len(candidate.preview.Prices))
	for index, row := range candidate.preview.Prices {
		inputs[index] = OfficialPricingInput{CanonicalModelID: row.CanonicalModelID, DisplayModelID: row.DisplayModelID, Source: row.Source, PricingStyle: row.PricingStyle, SourceURL: row.SourceURL, FetchedAt: row.FetchedAt, ResponseHash: row.ResponseHash, SourceVersion: row.SourceVersion, PromptPricePer1M: row.PromptPricePer1M, CompletionPricePer1M: row.CompletionPricePer1M, CacheReadPricePer1M: row.CacheReadPricePer1M, CacheWritePricePer1M: row.CacheWritePricePer1M}
	}
	rows, err := s.UpdateOfficialPricingBatch(ctx, inputs)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func sourceForMetadata(sourceID string, entry pricingmetadata.Entry) string {
	if sourceID == pricingmetadata.SourceOpenRouter || strings.EqualFold(entry.ProviderID, "openrouter") {
		return entities.OfficialModelPriceSourceOpenRouter
	}
	return entities.OfficialModelPriceSourceOfficial
}

func sourceURLForMetadata(catalog pricingmetadata.Catalog, entry pricingmetadata.Entry) string {
	if sourceForMetadata(catalog.Source.ID, entry) == entities.OfficialModelPriceSourceOpenRouter {
		return "https://openrouter.ai/api/v1/models"
	}
	return catalog.Source.URL
}

func displayMetadataModel(model pricingmetadata.Model) string {
	if display := strings.TrimSpace(model.Name); display != "" {
		return display
	}
	return strings.TrimSpace(model.ID)
}

func metadataPricingStyle(model pricingmetadata.Model) string {
	value := strings.ToLower(strings.TrimSpace(model.ID + " " + model.Name + " " + model.Family))
	if strings.Contains(value, "claude") {
		return entities.ModelPricingStyleClaude
	}
	return entities.ModelPricingStyleOpenAI
}

func newPreviewID() string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(value)
}

func timePointer(value time.Time) *time.Time { return &value }

func cloneOfficialPreview(input OfficialPricingRefreshPreview) OfficialPricingRefreshPreview {
	if input.Prices == nil {
		input.Prices = nil
		return input
	}
	prices := input.Prices
	input.Prices = make([]entities.OfficialModelPrice, len(prices))
	for index, price := range prices {
		input.Prices[index] = price
		input.Prices[index].Aliases = append([]entities.OfficialModelPriceAlias(nil), price.Aliases...)
		input.Prices[index].PromptPricePer1M = cloneOfficialPricePointer(price.PromptPricePer1M)
		input.Prices[index].CompletionPricePer1M = cloneOfficialPricePointer(price.CompletionPricePer1M)
		input.Prices[index].CacheReadPricePer1M = cloneOfficialPricePointer(price.CacheReadPricePer1M)
		input.Prices[index].CacheWritePricePer1M = cloneOfficialPricePointer(price.CacheWritePricePer1M)
	}
	return input
}

func cloneOfficialPricePointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

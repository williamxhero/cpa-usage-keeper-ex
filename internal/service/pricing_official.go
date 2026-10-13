package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net"
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

const (
	maxOfficialPricingBatch   = 5000
	maxOfficialPricingPreview = 128
	officialPricingPreviewTTL = 15 * time.Minute
)

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
	RequireComplete      bool
}

type OfficialPricingRefreshPreview struct {
	PreviewID       string                        `json:"preview_id"`
	SourceID        string                        `json:"source_id"`
	Source          string                        `json:"source"`
	SourceURL       string                        `json:"source_url"`
	FetchedAt       time.Time                     `json:"fetched_at"`
	ResponseHash    string                        `json:"response_hash"`
	Prices          []entities.OfficialModelPrice `json:"prices"`
	AmbiguousModels []string                      `json:"ambiguous_models,omitempty"`
	Stale           bool                          `json:"stale"`
}

type OfficialPricingProvider interface {
	ListOfficialPricing(context.Context) ([]entities.OfficialModelPrice, error)
	UpdateOfficialPricing(context.Context, OfficialPricingInput) (*entities.OfficialModelPrice, error)
	UpdateOfficialPricingBatch(context.Context, []OfficialPricingInput) ([]entities.OfficialModelPrice, error)
	PreviewOfficialPricingRefresh(context.Context, string) (OfficialPricingRefreshPreview, error)
	CommitOfficialPricingRefresh(context.Context, string) ([]entities.OfficialModelPrice, error)
}

type officialRefreshCandidate struct {
	preview   OfficialPricingRefreshPreview
	createdAt time.Time
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
	return s.updateOfficialPricingBatch(ctx, inputs, false)
}

func (s *pricingService) updateOfficialPricingBatch(ctx context.Context, inputs []OfficialPricingInput, refresh bool) ([]entities.OfficialModelPrice, error) {
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
		if refresh {
			// A preview may predate an administrator's official-price edit.
			// Recheck fallback protection inside the commit transaction.
			protected, err := repository.ListOfficialModelPrices(tx)
			if err != nil {
				return err
			}
			protectedOfficial := make(map[string]struct{}, len(protected))
			for _, row := range protected {
				if row.Source != entities.OfficialModelPriceSourceOpenRouter {
					protectedOfficial[strings.ToLower(strings.TrimSpace(row.CanonicalModelID))] = struct{}{}
				}
			}
			filtered := normalized[:0]
			for _, row := range normalized {
				if row.Source == entities.OfficialModelPriceSourceOpenRouter {
					if _, exists := protectedOfficial[strings.ToLower(row.CanonicalModelID)]; exists {
						continue
					}
				}
				filtered = append(filtered, row)
			}
			normalized = filtered
			if len(normalized) == 0 {
				rows = []entities.OfficialModelPrice{}
				return nil
			}
		}
		var err error
		rows, err = repository.UpsertOfficialModelPrices(tx, normalized)
		if err != nil {
			return err
		}
		if err := repository.ActivateStrictPricing(tx); err != nil {
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
	if candidate != nil {
		s.catalog.Replace(candidate)
	}
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
	if source != entities.OfficialModelPriceSourceOfficial && source != entities.OfficialModelPriceSourceModelsDev && source != entities.OfficialModelPriceSourceLiteLLM && source != entities.OfficialModelPriceSourceOpenRouter && source != entities.OfficialModelPriceSourceLegacySeed {
		return entities.OfficialModelPrice{}, "", fmt.Errorf("source is not supported")
	}
	if err := validateOfficialURL(input.SourceURL); err != nil {
		return entities.OfficialModelPrice{}, "", err
	}
	values := []*float64{input.PromptPricePer1M, input.CompletionPricePer1M, input.CacheReadPricePer1M, input.CacheWritePricePer1M}
	if input.RequireComplete {
		for _, value := range values {
			if value == nil {
				return entities.OfficialModelPrice{}, "", fmt.Errorf("all four prices are required")
			}
		}
	}
	for _, value := range values {
		if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return entities.OfficialModelPrice{}, "", fmt.Errorf("prices must be finite and non-negative")
		}
	}
	var aliases []entities.OfficialModelPriceAlias
	if input.Aliases != nil {
		aliases = make([]entities.OfficialModelPriceAlias, 0, len(input.Aliases))
	}
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
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") || !strings.Contains(host, ".") {
		return fmt.Errorf("source_url must use a public host")
	}
	if address := net.ParseIP(host); address != nil && (address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsUnspecified()) {
		return fmt.Errorf("source_url must use a public host")
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
		s.markOfficialPricingStale(ctx)
		return OfficialPricingRefreshPreview{}, err
	}
	existing, err := repository.ListOfficialModelPrices(s.db)
	if err != nil {
		return OfficialPricingRefreshPreview{}, err
	}
	blockedFallback := make(map[string]struct{}, len(existing))
	blockedOpenRouter := make(map[string]struct{}, len(existing))
	for _, row := range existing {
		key := strings.ToLower(strings.TrimSpace(row.CanonicalModelID))
		if key == "" {
			continue
		}
		if row.Source != entities.OfficialModelPriceSourceOpenRouter {
			blockedFallback[key] = struct{}{}
			blockedOpenRouter[key] = struct{}{}
		}
	}
	entries, ambiguousModels := uniqueRefreshEntries(catalog.Entries)
	for _, model := range ambiguousModels {
		blockedFallback[strings.ToLower(strings.TrimSpace(model))] = struct{}{}
	}
	var fallbackCatalog *pricingmetadata.Catalog
	if catalog.Source.ID != pricingmetadata.SourceOpenRouter {
		// OpenRouter is a fallback catalog only. A failed fallback does not
		// replace the last successful persisted snapshot.
		fallback, fallbackErr := s.metadataClient.Fetch(ctx, pricingmetadata.SourceOpenRouter)
		if fallbackErr != nil {
			s.markOfficialPricingStale(ctx)
			return OfficialPricingRefreshPreview{}, fallbackErr
		}
		fallbackCatalog = &fallback
		fallbackEntries, fallbackAmbiguous := uniqueRefreshEntries(fallback.Entries)
		ambiguousModels = append(ambiguousModels, fallbackAmbiguous...)
		entries = mergeOfficialAndOpenRouterEntries(entries, fallbackEntries, blockedFallback)
	} else {
		filtered := entries[:0]
		for _, entry := range entries {
			if _, exists := blockedOpenRouter[strings.ToLower(strings.TrimSpace(entry.Model.ID))]; exists {
				continue
			}
			filtered = append(filtered, entry)
		}
		entries = filtered
	}
	if len(entries) > maxOfficialPricingBatch {
		s.markOfficialPricingStale(ctx)
		return OfficialPricingRefreshPreview{}, fmt.Errorf("%w: maximum is %d", ErrOfficialPricingBatchTooLarge, maxOfficialPricingBatch)
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
			FetchedAt: timePointer(metadataCatalog.FetchedAt), ResponseHash: metadataCatalog.ResponseHash, SourceVersion: strings.TrimSpace(entry.Model.LastUpdated),
			PromptPricePer1M: entry.Model.Cost.Input, CompletionPricePer1M: entry.Model.Cost.Output,
			CacheReadPricePer1M: entry.Model.Cost.CacheRead, CacheWritePricePer1M: entry.Model.Cost.CacheWrite,
		}
		price.MissingSegments = officialMissingSegments(price)
		prices = append(prices, price)
	}
	sort.Slice(prices, func(i, j int) bool {
		return strings.ToLower(prices[i].CanonicalModelID) < strings.ToLower(prices[j].CanonicalModelID)
	})
	preview := OfficialPricingRefreshPreview{PreviewID: newPreviewID(), SourceID: catalog.Source.ID, Source: catalog.Source.Name, SourceURL: catalog.Source.URL, FetchedAt: catalog.FetchedAt, ResponseHash: catalog.ResponseHash, Prices: prices, AmbiguousModels: uniqueSortedStrings(ambiguousModels)}
	s.officialMu.Lock()
	if s.officialPreviews == nil {
		s.officialPreviews = make(map[string]officialRefreshCandidate)
	}
	now := time.Now().UTC()
	pruneOfficialPricingPreviewsLocked(s.officialPreviews, now)
	for len(s.officialPreviews) >= maxOfficialPricingPreview {
		oldestID := ""
		var oldest time.Time
		for id, candidate := range s.officialPreviews {
			if oldestID == "" || candidate.createdAt.Before(oldest) {
				oldestID, oldest = id, candidate.createdAt
			}
		}
		if oldestID == "" {
			break
		}
		delete(s.officialPreviews, oldestID)
	}
	s.officialPreviews[preview.PreviewID] = officialRefreshCandidate{preview: preview, createdAt: now}
	s.officialMu.Unlock()
	return cloneOfficialPreview(preview), nil
}

func pruneOfficialPricingPreviewsLocked(previews map[string]officialRefreshCandidate, now time.Time) {
	for id, candidate := range previews {
		if candidate.createdAt.IsZero() || now.Sub(candidate.createdAt) >= officialPricingPreviewTTL {
			delete(previews, id)
		}
	}
}

func uniqueRefreshEntries(entries []pricingmetadata.Entry) ([]pricingmetadata.Entry, []string) {
	byID := make(map[string]pricingmetadata.Entry, len(entries))
	ambiguous := make(map[string]struct{})
	for _, entry := range entries {
		id := strings.TrimSpace(entry.Model.ID)
		key := strings.ToLower(id)
		if key == "" {
			continue
		}
		if _, exists := byID[key]; exists {
			ambiguous[key] = struct{}{}
			continue
		}
		byID[key] = entry
	}
	result := make([]pricingmetadata.Entry, 0, len(byID))
	for key, entry := range byID {
		if _, isAmbiguous := ambiguous[key]; !isAmbiguous {
			result = append(result, entry)
		}
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].Model.ID) < strings.ToLower(result[j].Model.ID) })
	return result, uniqueSortedStrings(stringMapKeys(ambiguous))
}

func mergeOfficialAndOpenRouterEntries(official, fallback []pricingmetadata.Entry, blocked map[string]struct{}) []pricingmetadata.Entry {
	result := append([]pricingmetadata.Entry(nil), official...)
	seen := make(map[string]struct{}, len(official))
	for _, entry := range official {
		if id := strings.ToLower(strings.TrimSpace(entry.Model.ID)); id != "" {
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
		if _, exists := blocked[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, entry)
	}
	return result
}

func stringMapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	result := stringMapKeys(seen)
	sort.Strings(result)
	return result
}

func (s *pricingService) markOfficialPricingStale(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Persist failure state even if the upstream request used up its deadline.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	var candidate *pricing.Snapshot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repository.MarkOfficialModelPricesStale(tx, nil); err != nil {
			return err
		}
		var err error
		candidate, err = repository.LoadPricingSnapshot(ctx, tx)
		return err
	})
	if err == nil && candidate != nil {
		s.catalog.Replace(candidate)
	}
}

func (s *pricingService) CommitOfficialPricingRefresh(ctx context.Context, previewID string) ([]entities.OfficialModelPrice, error) {
	previewID = strings.TrimSpace(previewID)
	s.officialMu.Lock()
	pruneOfficialPricingPreviewsLocked(s.officialPreviews, time.Now().UTC())
	candidate, ok := s.officialPreviews[previewID]
	s.officialMu.Unlock()
	if !ok {
		return nil, ErrOfficialPricingPreviewNotFound
	}
	inputs := make([]OfficialPricingInput, len(candidate.preview.Prices))
	for index, row := range candidate.preview.Prices {
		inputs[index] = OfficialPricingInput{CanonicalModelID: row.CanonicalModelID, DisplayModelID: row.DisplayModelID, Source: row.Source, PricingStyle: row.PricingStyle, SourceURL: row.SourceURL, FetchedAt: row.FetchedAt, ResponseHash: row.ResponseHash, SourceVersion: row.SourceVersion, PromptPricePer1M: row.PromptPricePer1M, CompletionPricePer1M: row.CompletionPricePer1M, CacheReadPricePer1M: row.CacheReadPricePer1M, CacheWritePricePer1M: row.CacheWritePricePer1M}
	}
	rows, err := s.updateOfficialPricingBatch(ctx, inputs, true)
	if err != nil {
		return nil, err
	}
	s.officialMu.Lock()
	delete(s.officialPreviews, previewID)
	s.officialMu.Unlock()
	return rows, nil
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

func sourceForMetadata(sourceID string, entry pricingmetadata.Entry) string {
	if sourceID == pricingmetadata.SourceOpenRouter || strings.EqualFold(entry.ProviderID, "openrouter") {
		return entities.OfficialModelPriceSourceOpenRouter
	}
	switch strings.ToLower(strings.TrimSpace(sourceID)) {
	case "models-dev":
		return entities.OfficialModelPriceSourceModelsDev
	case "litellm":
		return entities.OfficialModelPriceSourceLiteLLM
	default:
		return entities.OfficialModelPriceSourceOfficial
	}
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
	input.Prices = append([]entities.OfficialModelPrice(nil), input.Prices...)
	return input
}

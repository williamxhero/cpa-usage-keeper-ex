package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricingmetadata"
	"cpa-usage-keeper/internal/service"
	"github.com/gin-gonic/gin"
)

const maxOfficialPricingRequestBytes = 1 << 20

type officialPricingResponse struct {
	ID                   int64      `json:"id"`
	CanonicalModelID     string     `json:"canonical_model_id"`
	DisplayModelID       string     `json:"display_model_id"`
	Source               string     `json:"source"`
	PricingStyle         string     `json:"pricing_style"`
	SourceURL            string     `json:"source_url,omitempty"`
	FetchedAt            *time.Time `json:"fetched_at,omitempty"`
	UpdatedAt            *time.Time `json:"updated_at,omitempty"`
	ResponseHash         string     `json:"response_hash,omitempty"`
	SourceVersion        string     `json:"source_version,omitempty"`
	PromptPricePer1M     *float64   `json:"prompt_price_per_1m"`
	CompletionPricePer1M *float64   `json:"completion_price_per_1m"`
	CacheReadPricePer1M  *float64   `json:"cache_read_price_per_1m"`
	CacheWritePricePer1M *float64   `json:"cache_write_price_per_1m"`
	MissingSegments      []string   `json:"missing_segments"`
	Stale                bool       `json:"stale"`
	StaleAt              *time.Time `json:"stale_at,omitempty"`
	Aliases              []string   `json:"aliases,omitempty"`
}

type officialPricingRequest struct {
	CanonicalModelID     string   `json:"canonical_model_id"`
	DisplayModelID       string   `json:"display_model_id"`
	Source               string   `json:"source"`
	PricingStyle         string   `json:"pricing_style"`
	SourceURL            string   `json:"source_url"`
	PromptPricePer1M     *float64 `json:"prompt_price_per_1m"`
	CompletionPricePer1M *float64 `json:"completion_price_per_1m"`
	CacheReadPricePer1M  *float64 `json:"cache_read_price_per_1m"`
	CacheWritePricePer1M *float64 `json:"cache_write_price_per_1m"`
	Aliases              []string `json:"aliases"`
}

type officialPricingBatchRequest struct {
	Pricing []officialPricingRequest `json:"pricing"`
}

type officialRefreshCommitRequest struct {
	PreviewID string `json:"preview_id"`
}

type officialRefreshPreviewResponse struct {
	PreviewID       string                    `json:"preview_id"`
	SourceID        string                    `json:"source_id"`
	Source          string                    `json:"source"`
	SourceURL       string                    `json:"source_url"`
	FetchedAt       time.Time                 `json:"fetched_at"`
	ResponseHash    string                    `json:"response_hash"`
	Prices          []officialPricingResponse `json:"prices"`
	AmbiguousModels []string                  `json:"ambiguous_models,omitempty"`
	Stale           bool                      `json:"stale"`
}

func registerOfficialPricingRoutes(router gin.IRoutes, pricingProvider service.PricingProvider) {
	provider, _ := pricingProvider.(service.OfficialPricingProvider)
	available := func(c *gin.Context) bool {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "official pricing is not configured", "code": "official_pricing_unavailable"})
			return false
		}
		return true
	}

	router.GET("/pricing/official", func(c *gin.Context) {
		if !available(c) {
			return
		}
		rows, err := provider.ListOfficialPricing(c.Request.Context())
		if err != nil {
			writeInternalError(c, "list official pricing failed", err)
			return
		}
		response := make([]officialPricingResponse, len(rows))
		for index := range rows {
			response[index] = officialPricingResponseFromEntity(rows[index])
		}
		c.JSON(http.StatusOK, gin.H{"pricing": response})
	})

	save := func(c *gin.Context) {
		if !available(c) {
			return
		}
		var request officialPricingRequest
		if !decodeOfficialPricingJSON(c, &request) {
			return
		}
		if pathID := strings.TrimSpace(c.Param("canonicalModelID")); pathID != "" {
			request.CanonicalModelID = pathID
		}
		row, err := provider.UpdateOfficialPricing(c.Request.Context(), officialPricingInput(request))
		if err != nil {
			officialPricingError(c, err)
			return
		}
		c.JSON(http.StatusOK, officialPricingResponseFromEntity(*row))
	}
	router.PUT("/pricing/official", save)
	router.PUT("/pricing/official/:canonicalModelID", save)

	router.PUT("/pricing/official/batch", func(c *gin.Context) {
		if !available(c) {
			return
		}
		var request officialPricingBatchRequest
		if !decodeOfficialPricingJSON(c, &request) {
			return
		}
		inputs := make([]service.OfficialPricingInput, len(request.Pricing))
		for index := range request.Pricing {
			inputs[index] = officialPricingInput(request.Pricing[index])
		}
		rows, err := provider.UpdateOfficialPricingBatch(c.Request.Context(), inputs)
		if err != nil {
			officialPricingError(c, err)
			return
		}
		response := make([]officialPricingResponse, len(rows))
		for index := range rows {
			response[index] = officialPricingResponseFromEntity(rows[index])
		}
		c.JSON(http.StatusOK, gin.H{"pricing": response})
	})

	router.GET("/pricing/official/refresh/preview", func(c *gin.Context) {
		if !available(c) {
			return
		}
		preview, err := provider.PreviewOfficialPricingRefresh(c.Request.Context(), c.Query("source"))
		if err != nil {
			officialPricingError(c, err)
			return
		}
		c.JSON(http.StatusOK, officialRefreshPreviewResponseFromService(preview))
	})

	router.POST("/pricing/official/refresh/commit", func(c *gin.Context) {
		if !available(c) {
			return
		}
		var request officialRefreshCommitRequest
		if !decodeOfficialPricingJSON(c, &request) {
			return
		}
		rows, err := provider.CommitOfficialPricingRefresh(c.Request.Context(), request.PreviewID)
		if err != nil {
			officialPricingError(c, err)
			return
		}
		response := make([]officialPricingResponse, len(rows))
		for index := range rows {
			response[index] = officialPricingResponseFromEntity(rows[index])
		}
		c.JSON(http.StatusOK, gin.H{"pricing": response})
	})
}

func officialRefreshPreviewResponseFromService(preview service.OfficialPricingRefreshPreview) officialRefreshPreviewResponse {
	prices := make([]officialPricingResponse, len(preview.Prices))
	for index, row := range preview.Prices {
		prices[index] = officialPricingResponseFromEntity(row)
	}
	return officialRefreshPreviewResponse{PreviewID: preview.PreviewID, SourceID: preview.SourceID, Source: preview.Source, SourceURL: preview.SourceURL, FetchedAt: preview.FetchedAt, ResponseHash: preview.ResponseHash, Prices: prices, AmbiguousModels: preview.AmbiguousModels, Stale: preview.Stale}
}

func decodeOfficialPricingJSON(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxOfficialPricingRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if isRequestEntityTooLarge(err) {
			writeRequestEntityTooLarge(c)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid official pricing request", "code": "invalid_official_pricing"})
		}
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if isRequestEntityTooLarge(err) {
			writeRequestEntityTooLarge(c)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid official pricing request", "code": "invalid_official_pricing"})
		}
		return false
	}
	return true
}

func officialPricingInput(input officialPricingRequest) service.OfficialPricingInput {
	return service.OfficialPricingInput{
		CanonicalModelID: input.CanonicalModelID, DisplayModelID: input.DisplayModelID, Source: input.Source,
		PricingStyle: input.PricingStyle, SourceURL: input.SourceURL, PromptPricePer1M: input.PromptPricePer1M,
		CompletionPricePer1M: input.CompletionPricePer1M, CacheReadPricePer1M: input.CacheReadPricePer1M,
		CacheWritePricePer1M: input.CacheWritePricePer1M, Aliases: input.Aliases, RequireComplete: true,
	}
}

func officialPricingResponseFromEntity(row entities.OfficialModelPrice) officialPricingResponse {
	missing := []string{}
	if row.MissingSegments != "" {
		missing = strings.Split(row.MissingSegments, ",")
	}
	aliases := make([]string, 0, len(row.Aliases))
	for _, alias := range row.Aliases {
		aliases = append(aliases, alias.Alias)
	}
	var updatedAt *time.Time
	if !row.UpdatedAt.IsZero() {
		updated := row.UpdatedAt
		updatedAt = &updated
	}
	return officialPricingResponse{ID: row.ID, CanonicalModelID: row.CanonicalModelID, DisplayModelID: row.DisplayModelID, Source: row.Source, PricingStyle: row.PricingStyle, SourceURL: row.SourceURL, FetchedAt: row.FetchedAt, UpdatedAt: updatedAt, ResponseHash: row.ResponseHash, SourceVersion: row.SourceVersion, PromptPricePer1M: row.PromptPricePer1M, CompletionPricePer1M: row.CompletionPricePer1M, CacheReadPricePer1M: row.CacheReadPricePer1M, CacheWritePricePer1M: row.CacheWritePricePer1M, MissingSegments: missing, Stale: row.Stale, StaleAt: row.StaleAt, Aliases: aliases}
}

func officialPricingError(c *gin.Context, err error) {
	var networkError net.Error
	switch {
	case errors.Is(err, pricingmetadata.ErrInvalidSource):
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported pricing source", "code": "invalid_pricing_source"})
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()):
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "pricing source request timed out", "code": "pricing_source_timeout"})
	case errors.Is(err, service.ErrOfficialPricingBatchTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "official pricing batch exceeds limit", "code": "official_pricing_batch_too_large"})
	case errors.Is(err, service.ErrOfficialPricingPreviewNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "official pricing preview not found", "code": "official_pricing_preview_not_found"})
	case errors.Is(err, service.ErrInvalidPricingInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid official pricing", "code": "invalid_official_pricing"})
	default:
		writeInternalError(c, "official pricing operation failed", err)
	}
}

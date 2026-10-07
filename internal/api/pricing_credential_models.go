package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"bytes"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/service"
	"github.com/gin-gonic/gin"
)

func invalidCredentialFixed(c *gin.Context, field string) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid complete fixed tariff or pricing style", "field": field})
}

func decodeCredentialFixed(raw json.RawMessage) (pricing.FixedTariff, string) {
	var request struct {
		Prompt     json.RawMessage `json:"prompt_price_per_1m"`
		Completion json.RawMessage `json:"completion_price_per_1m"`
		Read       json.RawMessage `json:"cache_read_price_per_1m"`
		Write      json.RawMessage `json:"cache_write_price_per_1m"`
		Style      string          `json:"pricing_style"`
	}
	if len(raw) == 0 || string(raw) == "null" {
		return pricing.FixedTariff{}, "fixed"
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		return pricing.FixedTariff{}, "fixed"
	}
	result := pricing.FixedTariff{PricingStyle: request.Style}
	for _, item := range []struct {
		name   string
		raw    json.RawMessage
		target **float64
	}{
		{"prompt_price_per_1m", request.Prompt, &result.PromptPricePer1M},
		{"completion_price_per_1m", request.Completion, &result.CompletionPricePer1M},
		{"cache_read_price_per_1m", request.Read, &result.CacheReadPricePer1M},
		{"cache_write_price_per_1m", request.Write, &result.CacheWritePricePer1M},
	} {
		text, err := credentialMultiplierText(item.raw)
		if err != nil {
			return pricing.FixedTariff{}, item.name
		}
		value, err := pricing.ParseFixedRate(text)
		if err != nil {
			return pricing.FixedTariff{}, item.name
		}
		*item.target = &value
	}
	return result, ""
}

func registerPricingCredentialModelRoutes(router gin.IRoutes, pricingProvider service.PricingProvider) {
	provider, _ := pricingProvider.(service.PricingCredentialModelsProvider)
	writeError := func(c *gin.Context, err error) {
		switch {
		case errors.Is(err, service.ErrInvalidPricingInput):
			if c.GetString("credential_pricing_mode") == pricing.ModeFixed {
				invalidCredentialFixed(c, "fixed")
			} else {
				invalidCredentialMultiplier(c)
			}
		case errors.Is(err, service.ErrPricingCredentialNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "pricing credential not found"})
		default:
			writeCredentialBindingError(c, err)
		}
	}
	router.GET("/pricing/credentials/:subjectID/models", func(c *gin.Context) {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "credential models are not configured"})
			return
		}
		result, err := provider.ListCredentialModels(c.Request.Context(), c.Param("subjectID"))
		if err != nil {
			writeError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	handler := func(c *gin.Context) {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "credential models are not configured"})
			return
		}
		id, model := c.Param("subjectID"), strings.TrimSpace(c.Query("model"))
		if model == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model is required", "field": "model"})
			return
		}
		var result service.CredentialModelMultiplier
		var err error
		switch c.Request.Method {
		case http.MethodGet:
			result, err = provider.GetCredentialModel(c.Request.Context(), id, model)
		case http.MethodDelete:
			result, err = provider.ClearCredentialModel(c.Request.Context(), id, model)
		case http.MethodPut:
			var request struct {
				Mode       string          `json:"mode"`
				Multiplier json.RawMessage `json:"multiplier"`
				Fixed      json.RawMessage `json:"fixed"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(&struct{}{}) != io.EOF {
				invalidCredentialMultiplier(c)
				return
			}
			switch request.Mode {
			case "", pricing.ModeMultiplier:
				text, parseErr := credentialMultiplierText(request.Multiplier)
				if parseErr != nil {
					invalidCredentialMultiplier(c)
					return
				}
				result, err = provider.SetCredentialModel(c.Request.Context(), id, model, text)
			case pricing.ModeFixed:
				c.Set("credential_pricing_mode", pricing.ModeFixed)
				fixed, field := decodeCredentialFixed(request.Fixed)
				if field != "" {
					invalidCredentialFixed(c, field)
					return
				}
				fixedProvider, ok := pricingProvider.(service.PricingCredentialFixedProvider)
				if !ok {
					c.JSON(http.StatusNotImplemented, gin.H{"error": "fixed pricing is not configured"})
					return
				}
				result, err = fixedProvider.SetCredentialFixed(c.Request.Context(), id, model, fixed)
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pricing mode", "field": "mode"})
				return
			}
		}
		if err != nil {
			writeError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
	router.GET("/pricing/credentials/:subjectID/model", handler)
	router.PUT("/pricing/credentials/:subjectID/model", handler)
	router.DELETE("/pricing/credentials/:subjectID/model", handler)
}

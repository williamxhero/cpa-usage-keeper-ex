package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/service"

	"github.com/gin-gonic/gin"
)

func registerPricingChannelModelRoutes(router gin.IRoutes, pricingProvider service.PricingProvider) {
	provider, _ := pricingProvider.(service.PricingChannelModelsProvider)
	available := func(c *gin.Context) bool {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "channel models are not configured"})
			return false
		}
		return true
	}
	router.GET("/pricing/channels/:channelID/models", func(c *gin.Context) {
		if !available(c) {
			return
		}
		result, err := provider.ListChannelModels(c.Request.Context(), c.Param("channelID"))
		if err != nil {
			channelError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	handler := func(c *gin.Context) {
		if !available(c) {
			return
		}
		id, model := c.Param("channelID"), strings.TrimSpace(c.Query("model"))
		if model == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model is required", "field": "model"})
			return
		}
		var result service.ChannelModelPrice
		var err error
		switch c.Request.Method {
		case http.MethodGet:
			result, err = provider.GetChannelModel(c.Request.Context(), id, model)
		case http.MethodDelete:
			result, err = provider.ClearChannelModel(c.Request.Context(), id, model)
		case http.MethodPut:
			var request struct {
				Mode       string          `json:"mode"`
				Multiplier json.RawMessage `json:"multiplier"`
				Fixed      json.RawMessage `json:"fixed"`
			}
			if !decodeChannelRequest(c, &request) {
				return
			}
			switch request.Mode {
			case "", pricing.ModeMultiplier:
				text, parseErr := credentialMultiplierText(request.Multiplier)
				if parseErr != nil {
					invalidCredentialMultiplier(c)
					return
				}
				result, err = provider.SetChannelModel(c.Request.Context(), id, model, text)
			case pricing.ModeFixed:
				fixed, field := decodeCredentialFixed(request.Fixed)
				if field != "" {
					invalidCredentialFixed(c, field)
					return
				}
				result, err = provider.SetChannelFixed(c.Request.Context(), id, model, fixed)
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pricing mode", "field": "mode"})
				return
			}
		}
		if err != nil {
			channelError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
	router.GET("/pricing/channels/:channelID/model", handler)
	router.PUT("/pricing/channels/:channelID/model", handler)
	router.DELETE("/pricing/channels/:channelID/model", handler)
}

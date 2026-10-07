package api

import (
	"cpa-usage-keeper/internal/service"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

func decodeChannelRequest(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel request", "code": "invalid_channel"})
		return false
	}
	return true
}
func channelError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPricingChannelNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "pricing channel not found", "code": "channel_not_found"})
	case errors.Is(err, service.ErrPricingChannelConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "channel members conflict or are not uniquely selectable", "code": "channel_conflict"})
	case errors.Is(err, service.ErrPricingChannelDependencies):
		c.JSON(http.StatusConflict, gin.H{"error": "confirm removal of channel members and default", "code": "channel_dependencies"})
	case errors.Is(err, service.ErrInvalidPricingInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel or multiplier", "code": "invalid_channel"})
	default:
		writeInternalError(c, "pricing channel update failed", err)
	}
}
func registerPricingChannelRoutes(router gin.IRoutes, pricingProvider service.PricingProvider) {
	provider, _ := pricingProvider.(service.PricingChannelsProvider)
	available := func(c *gin.Context) bool {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "pricing channels are not configured"})
			return false
		}
		return true
	}
	router.GET("/pricing/channels", func(c *gin.Context) {
		if !available(c) {
			return
		}
		channels, err := provider.ListPricingChannels(c.Request.Context())
		if err != nil {
			channelError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"channels": channels})
	})
	save := func(c *gin.Context) {
		if !available(c) {
			return
		}
		var input service.PricingChannelInput
		if !decodeChannelRequest(c, &input) {
			return
		}
		var result service.PricingChannel
		var err error
		if c.Request.Method == http.MethodPost {
			result, err = provider.CreatePricingChannel(c.Request.Context(), input)
		} else {
			result, err = provider.UpdatePricingChannel(c.Request.Context(), c.Param("channelID"), input)
		}
		if err != nil {
			channelError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
	router.POST("/pricing/channels", save)
	router.PUT("/pricing/channels/:channelID", save)
	router.GET("/pricing/channels/:channelID", func(c *gin.Context) {
		if !available(c) {
			return
		}
		result, err := provider.GetPricingChannel(c.Request.Context(), c.Param("channelID"))
		if err != nil {
			channelError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	router.DELETE("/pricing/channels/:channelID", func(c *gin.Context) {
		if !available(c) {
			return
		}
		// An omitted query never confirms destructive dependency handling.
		if confirm := c.Query("confirm_dependencies"); confirm != "" && confirm != "true" && confirm != "false" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid confirmation", "code": "invalid_channel"})
			return
		}
		if err := provider.DeletePricingChannel(c.Request.Context(), c.Param("channelID"), c.Query("confirm_dependencies") == "true"); err != nil {
			channelError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	defaults := func(c *gin.Context) {
		if !available(c) {
			return
		}
		id := c.Param("channelID")
		var result service.PricingChannel
		var err error
		switch c.Request.Method {
		case http.MethodGet:
			result, err = provider.GetPricingChannel(c.Request.Context(), id)
		case http.MethodDelete:
			result, err = provider.ClearChannelDefault(c.Request.Context(), id)
		case http.MethodPut:
			var input struct {
				Multiplier json.RawMessage `json:"multiplier"`
			}
			if !decodeChannelRequest(c, &input) {
				return
			}
			text, parseErr := credentialMultiplierText(input.Multiplier)
			if parseErr != nil {
				invalidCredentialMultiplier(c)
				return
			}
			result, err = provider.SetChannelDefault(c.Request.Context(), id, text)
		}
		if err != nil {
			channelError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
	router.GET("/pricing/channels/:channelID/default", defaults)
	router.PUT("/pricing/channels/:channelID/default", defaults)
	router.DELETE("/pricing/channels/:channelID/default", defaults)
}

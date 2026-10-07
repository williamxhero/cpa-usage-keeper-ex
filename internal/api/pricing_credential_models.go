package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"cpa-usage-keeper/internal/service"
	"github.com/gin-gonic/gin"
)

func registerPricingCredentialModelRoutes(router gin.IRoutes, pricingProvider service.PricingProvider) {
	provider, _ := pricingProvider.(service.PricingCredentialModelsProvider)
	writeError := func(c *gin.Context, err error) {
		switch {
		case errors.Is(err, service.ErrInvalidPricingInput):
			invalidCredentialMultiplier(c)
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
				Multiplier json.RawMessage `json:"multiplier"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(&struct{}{}) != io.EOF {
				invalidCredentialMultiplier(c)
				return
			}
			text, parseErr := credentialMultiplierText(request.Multiplier)
			if parseErr != nil {
				invalidCredentialMultiplier(c)
				return
			}
			result, err = provider.SetCredentialModel(c.Request.Context(), id, model, text)
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

package api

import (
	"cpa-usage-keeper/internal/service"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strconv"
)

func registerPricingCredentialDefaultRoutes(router gin.IRoutes, pricingProvider service.PricingProvider) {
	provider, _ := pricingProvider.(service.PricingCredentialDefaultsProvider)
	handler := func(c *gin.Context) {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "credential defaults are not configured"})
			return
		}
		id := c.Param("subjectID")
		var result service.CredentialDefault
		var err error
		switch c.Request.Method {
		case http.MethodGet:
			result, err = provider.GetCredentialDefault(c.Request.Context(), id)
		case http.MethodDelete:
			result, err = provider.ClearCredentialDefault(c.Request.Context(), id)
		case http.MethodPut:
			var request struct {
				Multiplier json.RawMessage `json:"multiplier"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
			decoder.DisallowUnknownFields()
			if decodeErr := decoder.Decode(&request); decodeErr != nil {
				invalidCredentialMultiplier(c)
				return
			}
			if decodeErr := decoder.Decode(&struct{}{}); decodeErr != io.EOF {
				invalidCredentialMultiplier(c)
				return
			}
			var text string
			if json.Unmarshal(request.Multiplier, &text) != nil {
				var number json.Number
				if len(request.Multiplier) == 0 || string(request.Multiplier) == "null" || json.Unmarshal(request.Multiplier, &number) != nil {
					invalidCredentialMultiplier(c)
					return
				}
				value, parseErr := number.Float64()
				if parseErr != nil || value < 0 {
					invalidCredentialMultiplier(c)
					return
				}
				// Numeric JSON may use exponents; normalize into decimal text before the
				// same service validation used for string input.
				if value == 0 {
					value = 0 // Canonicalize JSON negative zero as numeric zero.
				}
				text = strconv.FormatFloat(value, 'f', -1, 64)
			}
			result, err = provider.SetCredentialDefault(c.Request.Context(), id, text)
		}
		if err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidPricingInput):
				invalidCredentialMultiplier(c)
			case errors.Is(err, service.ErrPricingCredentialNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "pricing credential not found"})
			default:
				writeCredentialBindingError(c, err)
			}
			return
		}
		c.JSON(http.StatusOK, result)
	}
	router.GET("/pricing/credentials/:subjectID/default", handler)
	router.PUT("/pricing/credentials/:subjectID/default", handler)
	router.DELETE("/pricing/credentials/:subjectID/default", handler)
}

func invalidCredentialMultiplier(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multiplier", "field": "multiplier"})
}

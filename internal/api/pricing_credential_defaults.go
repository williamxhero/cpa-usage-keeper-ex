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
			text, parseErr := credentialMultiplierText(request.Multiplier)
			if parseErr != nil {
				invalidCredentialMultiplier(c)
				return
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

// Numeric JSON may use exponents; normalize to decimal text before the same
// domain validation used by form strings. Omitted/null are never inheritance.
func credentialMultiplierText(raw json.RawMessage) (string, error) {
	var text string
	if len(raw) == 0 || string(raw) == "null" {
		return "", errors.New("multiplier required")
	}
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return "", err
	}
	value, err := number.Float64()
	if err != nil || value < 0 {
		return "", errors.New("invalid multiplier")
	}
	if value == 0 {
		value = 0
	}
	return strconv.FormatFloat(value, 'f', -1, 64), nil
}

func invalidCredentialMultiplier(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multiplier", "field": "multiplier"})
}

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"cpa-usage-keeper/internal/service"
	"github.com/gin-gonic/gin"
)

func registerPricingIdentityMigrationRoutes(router gin.IRoutes, pricingProvider service.PricingProvider) {
	provider, _ := pricingProvider.(service.PricingIdentityMigrationProvider)
	ready := func(c *gin.Context) bool {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "identity migration is not configured"})
			return false
		}
		return true
	}
	router.GET("/pricing/identity-bindings", func(c *gin.Context) {
		if !ready(c) {
			return
		}
		result, err := provider.GetPricingIdentityState(c.Request.Context())
		if err != nil {
			writePricingIdentityError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	router.POST("/pricing/identity-bindings/migrate", func(c *gin.Context) {
		if !ready(c) {
			return
		}
		var input service.PricingIdentityMigrationInput
		if !decodePricingIdentityInput(c, &input) {
			return
		}
		result, err := provider.MigratePricingIdentity(c.Request.Context(), input)
		if err != nil {
			writePricingIdentityError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	router.PUT("/pricing/identity-bindings/:bindingRef/correction", func(c *gin.Context) {
		if !ready(c) {
			return
		}
		var input service.PricingIdentityCorrectionInput
		if !decodePricingIdentityInput(c, &input) {
			return
		}
		input.BindingRef = c.Param("bindingRef")
		result, err := provider.CorrectPricingIdentity(c.Request.Context(), input)
		if err != nil {
			writePricingIdentityError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
}
func decodePricingIdentityInput(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding selection"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding selection"})
		return false
	}
	return true
}
func writePricingIdentityError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPricingBindingConfirmation):
		c.JSON(http.StatusBadRequest, gin.H{"error": "explicit binding confirmation required"})
	case errors.Is(err, service.ErrPricingBindingStale):
		c.JSON(http.StatusConflict, gin.H{"error": "binding selection changed; refresh and confirm again"})
	case errors.Is(err, service.ErrPricingCredentialNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "pricing credential not found"})
	case errors.Is(err, service.ErrInvalidPricingInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding selection"})
	default:
		writeCredentialBindingError(c, err)
	}
}

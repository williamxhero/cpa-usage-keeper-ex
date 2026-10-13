package api

import (
	"errors"
	"net/http"

	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func registerPricingCredentialRoutes(router gin.IRoutes, pricingProvider service.PricingProvider, syncProviders ...service.PricingCredentialSyncProvider) {
	provider, _ := pricingProvider.(service.PricingCredentialProvider)
	var syncProvider service.PricingCredentialSyncProvider
	if len(syncProviders) > 0 {
		syncProvider = syncProviders[0]
	}
	router.GET("/pricing/credentials/sync-status", func(c *gin.Context) {
		if syncProvider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "credential sync is not configured"})
			return
		}
		c.JSON(http.StatusOK, syncProvider.PricingCredentialSyncStatus())
	})
	router.POST("/pricing/credentials/refresh", func(c *gin.Context) {
		if syncProvider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "credential sync is not configured"})
			return
		}
		syncProvider.RequestPricingCredentialRefresh()
		c.JSON(http.StatusAccepted, syncProvider.PricingCredentialSyncStatus())
	})
	router.GET("/pricing/credentials", func(c *gin.Context) {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "credential directory is not configured"})
			return
		}
		credentials, err := provider.ListPricingCredentials(c.Request.Context())
		if err != nil {
			writeCredentialBindingError(c, err)
			return
		}
		c.JSON(http.StatusOK, struct {
			Credentials []servicedto.PricingCredential `json:"credentials"`
		}{credentials})
	})
	router.GET("/pricing/credential-subjects", func(c *gin.Context) {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "credential directory is not configured"})
			return
		}
		credentials, err := provider.ListCredentialPricingSubjects(c.Request.Context())
		if err != nil {
			writeCredentialBindingError(c, err)
			return
		}
		c.JSON(http.StatusOK, struct {
			Credentials []servicedto.PricingCredential `json:"credentials"`
		}{credentials})
	})
	router.POST("/pricing/credentials", pricingFeatureRemoved)
}

func writeCredentialBindingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrCredentialBindingConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "credential binding conflict"})
	case errors.Is(err, service.ErrCredentialNotSelectable):
		c.JSON(http.StatusConflict, gin.H{"error": "credential is not uniquely selectable"})
	default:
		// SQL, upstream metadata and decoder errors may contain secrets; never echo them.
		logrus.Error("credential binding operation failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "credential binding operation failed"})
	}
}

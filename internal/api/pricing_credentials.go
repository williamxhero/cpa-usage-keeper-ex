package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func registerPricingCredentialRoutes(router gin.IRoutes, pricingProvider service.PricingProvider) {
	provider, _ := pricingProvider.(service.PricingCredentialProvider)
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
	router.POST("/pricing/credentials", func(c *gin.Context) {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "credential directory is not configured"})
			return
		}
		var request struct {
			DirectoryID int64 `json:"directory_id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil || request.DirectoryID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential selection"})
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential selection"})
			return
		}
		credential, err := provider.BindPricingCredential(c.Request.Context(), request.DirectoryID)
		if err != nil {
			writeCredentialBindingError(c, err)
			return
		}
		c.JSON(http.StatusCreated, credential)
	})
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

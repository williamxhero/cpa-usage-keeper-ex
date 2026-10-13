package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func pricingFeatureRemoved(c *gin.Context) {
	// Do not decode or log legacy input: it may contain credential secrets.
	c.AbortWithStatusJSON(http.StatusGone, gin.H{
		"error": "pricing_feature_removed",
		"code":  "pricing_feature_removed",
	})
}

package api

import "github.com/gin-gonic/gin"

func registerPricingCredentialModelRoutes(router gin.IRoutes) {
	router.GET("/pricing/credentials/:subjectID/models", pricingFeatureRemoved)
	router.GET("/pricing/credentials/:subjectID/model", pricingFeatureRemoved)
	router.PUT("/pricing/credentials/:subjectID/model", pricingFeatureRemoved)
	router.DELETE("/pricing/credentials/:subjectID/model", pricingFeatureRemoved)
}

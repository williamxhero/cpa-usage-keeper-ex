package api

import "github.com/gin-gonic/gin"

func registerPricingCredentialDefaultRoutes(router gin.IRoutes) {
	router.GET("/pricing/credentials/:subjectID/default", pricingFeatureRemoved)
	router.PUT("/pricing/credentials/:subjectID/default", pricingFeatureRemoved)
	router.DELETE("/pricing/credentials/:subjectID/default", pricingFeatureRemoved)
}

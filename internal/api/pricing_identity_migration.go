package api

import "github.com/gin-gonic/gin"

func registerPricingIdentityMigrationRoutes(router gin.IRoutes) {
	router.GET("/pricing/identity-bindings", pricingFeatureRemoved)
	router.POST("/pricing/identity-bindings/migrate", pricingFeatureRemoved)
	router.PUT("/pricing/identity-bindings/:bindingRef/correction", pricingFeatureRemoved)
}

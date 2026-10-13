package api

import "github.com/gin-gonic/gin"

func registerPricingRuleRoutes(router gin.IRoutes) {
	router.GET("/pricing/rules", pricingFeatureRemoved)
	router.PUT("/pricing/rules", pricingFeatureRemoved)
}

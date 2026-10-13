package api

import "github.com/gin-gonic/gin"

func registerPricingChannelRoutes(router gin.IRoutes) {
	router.GET("/pricing/channels", pricingFeatureRemoved)
	router.POST("/pricing/channels", pricingFeatureRemoved)
	router.GET("/pricing/channels/:channelID", pricingFeatureRemoved)
	router.PUT("/pricing/channels/:channelID", pricingFeatureRemoved)
	router.DELETE("/pricing/channels/:channelID", pricingFeatureRemoved)
	router.GET("/pricing/channels/:channelID/default", pricingFeatureRemoved)
	router.PUT("/pricing/channels/:channelID/default", pricingFeatureRemoved)
	router.DELETE("/pricing/channels/:channelID/default", pricingFeatureRemoved)
}

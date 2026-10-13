package api

import "github.com/gin-gonic/gin"

func registerPricingChannelModelRoutes(router gin.IRoutes) {
	router.GET("/pricing/channels/:channelID/models", pricingFeatureRemoved)
	router.GET("/pricing/channels/:channelID/model", pricingFeatureRemoved)
	router.PUT("/pricing/channels/:channelID/model", pricingFeatureRemoved)
	router.DELETE("/pricing/channels/:channelID/model", pricingFeatureRemoved)
}

package api

import (
	"net/http"

	"cpa-usage-keeper/internal/service"
	"github.com/gin-gonic/gin"
)

func registerKeyCredentialMatrixRoute(router gin.IRoutes, pricingProvider service.PricingProvider) {
	provider, _ := pricingProvider.(service.ViewerCredentialModelMatrixProvider)
	router.GET("/key/pricing/credential-model-matrix", func(c *gin.Context) {
		session, _, ok := activeAPIKeyViewerContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "credential model matrix is not configured"})
			return
		}
		query := service.CredentialMatrixQuery{CredentialPage: parsePositiveQuery(c.Query("credential_page")), CredentialPageSize: parsePositiveQuery(c.Query("credential_page_size")), ModelPage: parsePositiveQuery(c.Query("model_page")), ModelPageSize: parsePositiveQuery(c.Query("model_page_size")), CredentialFilter: c.Query("credential"), ModelFilter: c.Query("model")}
		page, err := provider.ListViewerCredentialModelMatrix(c.Request.Context(), session.CPAAPIKeyID, query)
		if err != nil {
			writeCredentialMatrixError(c, err)
			return
		}
		c.JSON(http.StatusOK, page)
	})
}

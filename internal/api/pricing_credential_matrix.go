package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"cpa-usage-keeper/internal/service"
	"github.com/gin-gonic/gin"
)

const maxCredentialMatrixRequestBytes = 4 << 20

type credentialMatrixCellRequest struct {
	SubjectID            string          `json:"subject_id"`
	CanonicalModelID     string          `json:"canonical_model_id"`
	Model                string          `json:"model"`
	Mode                 string          `json:"mode"`
	Clear                bool            `json:"clear"`
	PricingStyle         string          `json:"pricing_style"`
	Fixed                json.RawMessage `json:"fixed"`
	PromptPricePer1M     *float64        `json:"prompt_price_per_1m"`
	CompletionPricePer1M *float64        `json:"completion_price_per_1m"`
	CacheReadPricePer1M  *float64        `json:"cache_read_price_per_1m"`
	CacheWritePricePer1M *float64        `json:"cache_write_price_per_1m"`
}

type credentialMatrixBatchRequest struct {
	Cells   []credentialMatrixCellRequest `json:"cells"`
	Updates []credentialMatrixCellRequest `json:"updates"`
}

type credentialMatrixFixedRequest struct {
	PricingStyle         string   `json:"pricing_style"`
	PromptPricePer1M     *float64 `json:"prompt_price_per_1m"`
	CompletionPricePer1M *float64 `json:"completion_price_per_1m"`
	CacheReadPricePer1M  *float64 `json:"cache_read_price_per_1m"`
	CacheWritePricePer1M *float64 `json:"cache_write_price_per_1m"`
}

func registerPricingCredentialMatrixRoutes(router gin.IRoutes, pricingProvider service.PricingProvider) {
	provider, _ := pricingProvider.(service.CredentialModelMatrixProvider)
	available := func(c *gin.Context) bool {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "credential model matrix is not configured", "code": "credential_matrix_unavailable"})
			return false
		}
		return true
	}
	list := func(c *gin.Context) {
		if !available(c) {
			return
		}
		query := service.CredentialMatrixQuery{CredentialPage: parsePositiveQuery(c.Query("credential_page")), CredentialPageSize: parsePositiveQuery(c.Query("credential_page_size")), ModelPage: parsePositiveQuery(c.Query("model_page")), ModelPageSize: parsePositiveQuery(c.Query("model_page_size")), CredentialFilter: c.Query("credential"), ModelFilter: c.Query("model")}
		page, err := provider.ListCredentialModelMatrix(c.Request.Context(), query)
		if err != nil {
			writeCredentialMatrixError(c, err)
			return
		}
		c.JSON(http.StatusOK, page)
	}
	write := func(c *gin.Context) {
		if !available(c) {
			return
		}
		var request credentialMatrixBatchRequest
		decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxCredentialMatrixRequestBytes))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			if isRequestEntityTooLarge(err) {
				writeRequestEntityTooLarge(c)
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential model matrix request", "code": "invalid_credential_matrix"})
			}
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			if isRequestEntityTooLarge(err) {
				writeRequestEntityTooLarge(c)
			} else {
				writeCredentialMatrixError(c, service.ErrInvalidPricingInput)
			}
			return
		}
		if request.Cells != nil && request.Updates != nil {
			writeCredentialMatrixError(c, service.ErrInvalidPricingInput)
			return
		}
		if request.Cells == nil && request.Updates == nil {
			writeCredentialMatrixError(c, service.ErrInvalidPricingInput)
			return
		}
		requests := request.Cells
		if requests == nil {
			requests = request.Updates
		}
		if len(requests) > service.CredentialMatrixMaxCells {
			writeCredentialMatrixError(c, service.ErrCredentialMatrixBatchTooLarge)
			return
		}
		inputs := make([]service.CredentialMatrixCellInput, len(requests))
		for index, cell := range requests {
			model := strings.TrimSpace(cell.CanonicalModelID)
			if model != "" && cell.Model != "" && !strings.EqualFold(model, strings.TrimSpace(cell.Model)) {
				writeCredentialMatrixError(c, service.ErrInvalidPricingInput)
				return
			}
			if model == "" {
				model = strings.TrimSpace(cell.Model)
			}
			mode := strings.ToLower(strings.TrimSpace(cell.Mode))
			clear := cell.Clear || mode == "clear" || mode == "inherit"
			if (mode != "" && mode != "fixed" && mode != "clear" && mode != "inherit") || (cell.Clear && mode == "fixed") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential model matrix request", "code": "invalid_credential_matrix"})
				return
			}
			if clear && ((len(cell.Fixed) > 0 && string(cell.Fixed) != "null") || cell.PromptPricePer1M != nil || cell.CompletionPricePer1M != nil || cell.CacheReadPricePer1M != nil || cell.CacheWritePricePer1M != nil) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential model matrix request", "code": "invalid_credential_matrix"})
				return
			}
			style := cell.PricingStyle
			prompt, completion, cacheRead, cacheWrite := cell.PromptPricePer1M, cell.CompletionPricePer1M, cell.CacheReadPricePer1M, cell.CacheWritePricePer1M
			if len(cell.Fixed) > 0 && string(cell.Fixed) != "null" {
				if prompt != nil || completion != nil || cacheRead != nil || cacheWrite != nil {
					writeCredentialMatrixError(c, service.ErrInvalidPricingInput)
					return
				}
				var fixed credentialMatrixFixedRequest
				fixedDecoder := json.NewDecoder(strings.NewReader(string(cell.Fixed)))
				fixedDecoder.DisallowUnknownFields()
				if fixedDecoder.Decode(&fixed) != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential model matrix request", "code": "invalid_credential_matrix"})
					return
				}
				var fixedExtra any
				if fixedDecoder.Decode(&fixedExtra) != io.EOF {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential model matrix request", "code": "invalid_credential_matrix"})
					return
				}
				if style != "" && fixed.PricingStyle != "" && !strings.EqualFold(strings.TrimSpace(style), strings.TrimSpace(fixed.PricingStyle)) {
					writeCredentialMatrixError(c, service.ErrInvalidPricingInput)
					return
				}
				if style == "" {
					style = fixed.PricingStyle
				}
				prompt, completion, cacheRead, cacheWrite = fixed.PromptPricePer1M, fixed.CompletionPricePer1M, fixed.CacheReadPricePer1M, fixed.CacheWritePricePer1M
			}
			inputs[index] = service.CredentialMatrixCellInput{SubjectID: cell.SubjectID, CanonicalModelID: model, Clear: clear, PricingStyle: style, PromptPricePer1M: prompt, CompletionPricePer1M: completion, CacheReadPricePer1M: cacheRead, CacheWritePricePer1M: cacheWrite}
		}
		cells, err := provider.UpdateCredentialModelMatrix(c.Request.Context(), inputs)
		if err != nil {
			writeCredentialMatrixError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"cells": cells})
	}
	for _, path := range []string{"/pricing/credential-model-matrix", "/pricing/credentials/matrix"} {
		router.GET(path, list)
		router.PUT(path, write)
	}
	router.PUT("/pricing/credential-model-matrix/batch", write)
}

func parsePositiveQuery(value string) int {
	result, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || result < 1 {
		return 0
	}
	return result
}

func writeCredentialMatrixError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrCredentialMatrixBatchTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "credential model matrix batch exceeds limit", "code": "credential_matrix_batch_too_large"})
	case errors.Is(err, service.ErrInvalidPricingInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential model matrix", "code": "invalid_credential_matrix"})
	case errors.Is(err, service.ErrPricingCredentialNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "pricing credential not found", "code": "pricing_credential_not_found"})
	default:
		writeInternalError(c, "credential model matrix operation failed", nil)
	}
}

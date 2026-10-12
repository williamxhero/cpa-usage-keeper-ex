// Package pricingmetadata 负责外部价格目录的下载和基础 token 价格归一化。
package pricingmetadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrInvalidSource = errors.New("unsupported pricing source")

const SourceOpenRouter = "openrouter"

const (
	maxCatalogResponseBytes = 64 << 20
	maxCatalogFetchDuration = 12 * time.Second
)

type Source struct{ ID, Name, URL string }

func SourceByID(id string) (Source, error) {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "", "models-dev":
		return Source{"models-dev", "Models.dev", "https://models.dev/api.json"}, nil
	case "litellm":
		return Source{"litellm", "LiteLLM", "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"}, nil
	case SourceOpenRouter:
		return Source{SourceOpenRouter, "OpenRouter", "https://openrouter.ai/api/v1/models"}, nil
	default:
		return Source{}, ErrInvalidSource
	}
}

// Cost 保留来源字段的缺失状态；服务层沿用预览中缺失缓存价按 0 处理的约定。
type Cost struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Family      string `json:"family"`
	LastUpdated string `json:"last_updated"`
	Status      string `json:"status"`
	Cost        Cost   `json:"cost"`
}

type Entry struct {
	ProviderID   string
	ProviderName string
	Model        Model
}

type Catalog struct {
	Source       Source
	FetchedAt    time.Time
	ResponseHash string
	Entries      []Entry
}

type Client struct{ httpClient *http.Client }

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: maxCatalogFetchDuration}
	} else {
		clientCopy := *httpClient
		if clientCopy.Timeout == 0 || clientCopy.Timeout > maxCatalogFetchDuration {
			clientCopy.Timeout = maxCatalogFetchDuration
		}
		httpClient = &clientCopy
	}
	return &Client{httpClient: httpClient}
}

func (c *Client) Fetch(ctx context.Context, sourceID string) (Catalog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	source, err := SourceByID(sourceID)
	if err != nil {
		return Catalog{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return Catalog{}, fmt.Errorf("build pricing catalog request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return Catalog{}, fmt.Errorf("fetch %s pricing catalog: %w", source.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Catalog{}, fmt.Errorf("fetch %s pricing catalog: unexpected status %d", source.Name, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogResponseBytes+1))
	if err != nil {
		return Catalog{}, fmt.Errorf("read %s pricing catalog: %w", source.Name, err)
	}
	if len(body) > maxCatalogResponseBytes {
		return Catalog{}, fmt.Errorf("read %s pricing catalog: response exceeds %d bytes", source.Name, maxCatalogResponseBytes)
	}
	// Preserve only a digest and fetch time in the catalog; upstream bodies may
	// contain provider metadata and are never exposed as persisted source data.
	responseHash := sha256.Sum256(body)
	decoder := json.NewDecoder(bytes.NewReader(body))
	var entries []Entry
	switch source.ID {
	case "litellm":
		entries, err = decodeLiteLLM(decoder)
	case SourceOpenRouter:
		entries, err = decodeOpenRouter(decoder)
	default:
		entries, err = decodeModelsDev(decoder)
	}
	if err != nil {
		return Catalog{}, fmt.Errorf("decode %s pricing catalog: %w", source.Name, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Catalog{}, fmt.Errorf("decode %s pricing catalog: trailing JSON data (multiple JSON documents)", source.Name)
		}
		return Catalog{}, fmt.Errorf("decode %s pricing catalog: %w", source.Name, err)
	}
	return Catalog{Source: source, FetchedAt: time.Now().UTC(), ResponseHash: hex.EncodeToString(responseHash[:]), Entries: entries}, nil
}

package pricingmetadata

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type openRouterResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Pricing struct {
		Prompt          json.RawMessage `json:"prompt"`
		Completion      json.RawMessage `json:"completion"`
		InputCacheRead  json.RawMessage `json:"input_cache_read"`
		InputCacheWrite json.RawMessage `json:"input_cache_write"`
	} `json:"pricing"`
}

func decodeOpenRouter(decoder *json.Decoder) ([]Entry, error) {
	var response openRouterResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(response.Data))
	for _, model := range response.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		prompt, err := openRouterPrice(model.Pricing.Prompt)
		if err != nil {
			return nil, fmt.Errorf("model %q prompt price: %w", id, err)
		}
		completion, err := openRouterPrice(model.Pricing.Completion)
		if err != nil {
			return nil, fmt.Errorf("model %q completion price: %w", id, err)
		}
		cacheRead, err := openRouterPrice(model.Pricing.InputCacheRead)
		if err != nil {
			return nil, fmt.Errorf("model %q cache-read price: %w", id, err)
		}
		cacheWrite, err := openRouterPrice(model.Pricing.InputCacheWrite)
		if err != nil {
			return nil, fmt.Errorf("model %q cache-write price: %w", id, err)
		}
		name := strings.TrimSpace(model.Name)
		if name == "" {
			name = id
		}
		inputPerMillion, err := openRouterPerMillion(prompt)
		if err != nil {
			return nil, fmt.Errorf("model %q prompt price: %w", id, err)
		}
		outputPerMillion, err := openRouterPerMillion(completion)
		if err != nil {
			return nil, fmt.Errorf("model %q completion price: %w", id, err)
		}
		cacheReadPerMillion, err := openRouterPerMillion(cacheRead)
		if err != nil {
			return nil, fmt.Errorf("model %q cache-read price: %w", id, err)
		}
		cacheWritePerMillion, err := openRouterPerMillion(cacheWrite)
		if err != nil {
			return nil, fmt.Errorf("model %q cache-write price: %w", id, err)
		}
		entries = append(entries, Entry{
			ProviderID: "openrouter", ProviderName: "OpenRouter",
			Model: Model{ID: id, Name: name, Cost: Cost{
				Input: inputPerMillion, Output: outputPerMillion,
				CacheRead: cacheReadPerMillion, CacheWrite: cacheWritePerMillion,
			}},
		})
	}
	return entries, nil
}

func openRouterPerMillion(price *float64) (*float64, error) {
	if price == nil {
		return nil, nil
	}
	value := *price * 1_000_000
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, fmt.Errorf("USD/token value overflows USD/1M")
	}
	return &value, nil
}

func openRouterPrice(raw json.RawMessage) (*float64, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	value := strings.TrimSpace(string(raw))
	if len(value) > 1 && value[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
	}
	price, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
		return nil, fmt.Errorf("invalid USD/token value")
	}
	return &price, nil
}

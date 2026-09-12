package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"sealchat/utils"
)

const modelsDevCacheTTL = 12 * time.Hour

var modelsDevAPIURL = "https://models.dev/api.json"
var modelsDevHTTPClient = &http.Client{Timeout: 5 * time.Second}

type modelsDevCatalog map[string]modelsDevProvider

type modelsDevProvider struct {
	Name   string                    `json:"name"`
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	Cost *modelsDevCost `json:"cost"`
}

type modelsDevCost struct {
	Input           float64         `json:"input"`
	Output          float64         `json:"output"`
	CacheRead       float64         `json:"cache_read"`
	Tiers           json.RawMessage `json:"tiers"`
	ContextOver200K json.RawMessage `json:"context_over_200k"`
}

var modelsDevPricingCache struct {
	sync.Mutex
	catalog   modelsDevCatalog
	fetchedAt time.Time
}

// FillMissingPricingFromModelsDev adds catalog pricing only for configured
// provider/model pairs that do not already have an explicit pricing entry.
func FillMissingPricingFromModelsDev(ctx context.Context, cfg utils.AIConfig) (utils.AIConfig, error) {
	cfg = utils.NormalizeAIConfig(cfg)

	pricingIndex := make(map[string]struct{}, len(cfg.Pricing))
	for _, pricing := range cfg.Pricing {
		pricingIndex[pricing.ProviderID+"::"+pricing.Model] = struct{}{}
	}

	type missingPricing struct {
		provider utils.AIProviderConfig
		model    string
	}
	missing := make([]missingPricing, 0)
	missingIndex := make(map[string]struct{})
	for _, provider := range cfg.Providers {
		for _, model := range provider.Models {
			key := provider.ID + "::" + model
			if _, exists := pricingIndex[key]; exists {
				continue
			}
			if _, exists := missingIndex[key]; exists {
				continue
			}
			missingIndex[key] = struct{}{}
			missing = append(missing, missingPricing{provider: provider, model: model})
		}
	}
	if len(missing) == 0 {
		return cfg, nil
	}

	catalog, err := getModelsDevCatalog(ctx)
	if err != nil {
		return cfg, err
	}
	for _, item := range missing {
		providerEntry, ok := findModelsDevProvider(catalog, item.provider)
		if !ok {
			continue
		}
		modelEntry, ok := providerEntry.Models[item.model]
		if !ok || modelEntry.Cost == nil {
			continue
		}
		cost := modelEntry.Cost
		if hasModelsDevTieredPricing(cost) {
			continue
		}
		if cost.Input < 0 || cost.Output < 0 || cost.CacheRead < 0 {
			continue
		}
		cfg.Pricing = append(cfg.Pricing, utils.AIModelPricingConfig{
			ProviderID:                 item.provider.ID,
			Model:                      item.model,
			PromptPricePer1MTokens:     cost.Input,
			CompletionPricePer1MTokens: cost.Output,
			CachePricePer1MTokens:      cost.CacheRead,
		})
	}

	return cfg, nil
}

func hasModelsDevTieredPricing(cost *modelsDevCost) bool {
	if cost == nil {
		return false
	}
	for _, raw := range []json.RawMessage{cost.Tiers, cost.ContextOver200K} {
		switch strings.TrimSpace(string(raw)) {
		case "", "null", "{}", "[]":
			continue
		default:
			return true
		}
	}
	return false
}

func getModelsDevCatalog(ctx context.Context) (modelsDevCatalog, error) {
	modelsDevPricingCache.Lock()
	defer modelsDevPricingCache.Unlock()

	if modelsDevPricingCache.catalog != nil && time.Since(modelsDevPricingCache.fetchedAt) < modelsDevCacheTTL {
		return modelsDevPricingCache.catalog, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsDevAPIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := modelsDevHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("models.dev pricing request failed (%d)", resp.StatusCode)
	}

	var catalog modelsDevCatalog
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, err
	}
	modelsDevPricingCache.catalog = catalog
	modelsDevPricingCache.fetchedAt = time.Now()
	return catalog, nil
}

func findModelsDevProvider(catalog modelsDevCatalog, provider utils.AIProviderConfig) (modelsDevProvider, bool) {
	normalizedID := strings.ToLower(strings.TrimSpace(provider.ID))
	if entry, ok := catalog[normalizedID]; ok {
		return entry, true
	}
	for _, suffix := range []string{"-default", "_default"} {
		if strings.HasSuffix(normalizedID, suffix) {
			if entry, ok := catalog[strings.TrimSuffix(normalizedID, suffix)]; ok {
				return entry, true
			}
			break
		}
	}

	providerName := strings.TrimSpace(provider.Name)
	var matchedKey string
	for key, entry := range catalog {
		if !strings.EqualFold(providerName, strings.TrimSpace(entry.Name)) {
			continue
		}
		if matchedKey != "" {
			return modelsDevProvider{}, false
		}
		matchedKey = key
	}
	if matchedKey == "" {
		return modelsDevProvider{}, false
	}
	return catalog[matchedKey], true
}

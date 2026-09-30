package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"sealchat/utils"
)

const aliyunBeijingHostSuffix = ".cn-beijing.maas.aliyuncs.com"

var ErrTTSProviderCredential = errors.New("Base URL 与 API Key 不匹配，或 API Key 无权访问该业务空间")

type TTSResolvedModel struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	CharacterPrice float64  `json:"characterPrice"`
	DisplayPrice   string   `json:"displayPrice"`
	DesignPrice    *float64 `json:"designPrice"`
	ClonePrice     float64  `json:"clonePrice"`
}

type TTSProviderResolution struct {
	ProviderID        string             `json:"providerId"`
	Workspace         string             `json:"workspace"`
	Region            string             `json:"region"`
	CredentialScope   string             `json:"credentialScope"`
	SynthesisEndpoint string             `json:"synthesisEndpoint"`
	VoiceEndpoint     string             `json:"voiceEndpoint"`
	Models            []TTSResolvedModel `json:"models"`
}

type aliyunModelCatalog struct {
	Success bool   `json:"success"`
	Code    any    `json:"code"`
	Message string `json:"message"`
	Output  struct {
		Models []aliyunCatalogModel `json:"models"`
	} `json:"output"`
}

type aliyunCatalogModel struct {
	Model  string             `json:"model"`
	Name   string             `json:"name"`
	Prices []aliyunPriceRange `json:"prices"`
}

type aliyunPriceRange struct {
	RangeName string            `json:"range_name"`
	Prices    []aliyunPriceItem `json:"prices"`
}

type aliyunPriceItem struct {
	Type      string          `json:"type"`
	Price     json.RawMessage `json:"price"`
	PriceUnit string          `json:"price_unit"`
	PriceName string          `json:"price_name"`
}

func ParseAliyunTTSBaseURL(baseURL string) (TTSProviderResolution, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || u.EscapedPath() != "/compatible-mode/v1" {
		return TTSProviderResolution{}, TTSValidationError("仅支持百炼北京地域的 HTTPS Compatible Mode Base URL")
	}
	host := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(host, aliyunBeijingHostSuffix) {
		return TTSProviderResolution{}, TTSValidationError("仅支持百炼北京地域的官方 Base URL")
	}
	workspace := strings.TrimSuffix(host, aliyunBeijingHostSuffix)
	if !validAliyunWorkspace(workspace) {
		return TTSProviderResolution{}, TTSValidationError("百炼 Base URL 中缺少有效的业务空间")
	}
	nativeBase := "https://" + host + "/api/v1"
	return TTSProviderResolution{
		Workspace:         workspace,
		Region:            "cn-beijing",
		CredentialScope:   "aliyun:" + workspace + ":cn-beijing",
		SynthesisEndpoint: nativeBase + "/services/audio/tts/SpeechSynthesizer",
		VoiceEndpoint:     nativeBase + "/services/audio/tts/customization",
		Models:            []TTSResolvedModel{},
	}, nil
}

func validAliyunWorkspace(value string) bool {
	if value == "" || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, r := range value {
		if r != '-' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func ResolveAliyunTTSProvider(ctx context.Context, baseURL, apiKey, providerID string) (TTSProviderResolution, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return resolveAliyunTTSProvider(ctx, &http.Client{Timeout: 12 * time.Second}, baseURL, apiKey, providerID)
}

func resolveAliyunTTSProvider(ctx context.Context, client *http.Client, baseURL, apiKey, providerID string) (TTSProviderResolution, error) {
	result, err := ParseAliyunTTSBaseURL(baseURL)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return result, TTSValidationError("请填写 API Key")
	}
	result.ProviderID = strings.TrimSpace(providerID)
	if result.ProviderID == "" {
		result.ProviderID = utils.NewID()
	}

	host := "https://" + result.Workspace + aliyunBeijingHostSuffix
	models, err := fetchAliyunModelCatalog(ctx, client, host+"/api/v1/models", apiKey, url.Values{
		"capabilities": {"TTS"},
		"language":     {"zh-CN"},
		"page_no":      {"1"},
		"page_size":    {"100"},
	})
	if err != nil {
		return result, err
	}

	designPrice := confirmedVoiceDesignPrice(models)
	if designPrice == nil {
		if enrollment, fetchErr := fetchAliyunModelCatalog(ctx, client, host+"/api/v1/models", apiKey, url.Values{
			"model":     {"voice-enrollment"},
			"language":  {"zh-CN"},
			"page_no":   {"1"},
			"page_size": {"20"},
		}); fetchErr == nil {
			designPrice = confirmedVoiceDesignPrice(enrollment)
		}
	}

	supported := map[string]bool{
		"qwen-audio-3.0-tts-flash": true,
		"qwen-audio-3.0-tts-plus":  true,
	}
	for _, model := range models {
		if !supported[model.Model] {
			continue
		}
		price, ok := characterPricePerTenThousand(model)
		if !ok {
			continue
		}
		name := strings.TrimSpace(model.Name)
		if name == "" {
			name = model.Model
		}
		result.Models = append(result.Models, TTSResolvedModel{
			ID:             model.Model,
			Name:           name,
			CharacterPrice: price / 10000,
			DisplayPrice:   strconv.FormatFloat(price, 'f', -1, 64) + " 元 / 万字符",
			DesignPrice:    designPrice,
			ClonePrice:     0,
		})
	}
	if len(result.Models) == 0 {
		return result, TTSValidationError("百炼模型目录未返回可确认配置的 Qwen Audio 3.0 TTS 模型")
	}
	return result, nil
}

func fetchAliyunModelCatalog(ctx context.Context, client *http.Client, endpoint, apiKey string, query url.Values) ([]aliyunCatalogModel, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.New("百炼模型目录地址无效")
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("无法创建百炼模型目录请求")
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	h := *client
	if h.Timeout <= 0 || h.Timeout > 15*time.Second {
		h.Timeout = 12 * time.Second
	}
	// Never forward the API key to another URL through a redirect.
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := h.Do(req)
	if err != nil {
		return nil, errors.New("无法连接百炼模型目录，请检查 Base URL 与网络连接")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrTTSProviderCredential
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("百炼模型目录请求失败（HTTP %d）", resp.StatusCode)
	}
	var payload aliyunModelCatalog
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, errors.New("百炼模型目录返回了无法解析的数据")
	}
	if !payload.Success && payload.Code != nil {
		return nil, errors.New("百炼模型目录拒绝了查询请求")
	}
	return payload.Output.Models, nil
}

func characterPricePerTenThousand(model aliyunCatalogModel) (float64, bool) {
	for _, priceRange := range model.Prices {
		for _, item := range priceRange.Prices {
			unit := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(item.PriceUnit), " ", ""))
			if unit != "每万字符" && unit != "每1万字符" && unit != "per10,000characters" && unit != "per10000characters" && unit != "per10kcharacters" {
				continue
			}
			if price, ok := parseAliyunPrice(item.Price); ok {
				return price, true
			}
		}
	}
	return 0, false
}

func confirmedVoiceDesignPrice(models []aliyunCatalogModel) *float64 {
	for _, model := range models {
		if model.Model != "voice-enrollment" {
			continue
		}
		for _, priceRange := range model.Prices {
			for _, item := range priceRange.Prices {
				unit := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(item.PriceUnit), " ", ""))
				label := strings.ToLower(item.Type + " " + item.PriceName)
				perUse := strings.HasPrefix(unit, "每次") || unit == "perrequest" || unit == "percall"
				design := strings.Contains(label, "design") || strings.Contains(label, "声音设计") || strings.Contains(label, "音色设计")
				if !perUse || !design {
					continue
				}
				if price, ok := parseAliyunPrice(item.Price); ok {
					return &price
				}
			}
		}
	}
	return nil
}

func parseAliyunPrice(raw json.RawMessage) (float64, bool) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return 0, false
	}
	var text string
	switch v := value.(type) {
	case json.Number:
		text = v.String()
	case string:
		text = v
	default:
		return 0, false
	}
	price, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	return price, err == nil && price >= 0
}

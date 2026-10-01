package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"sealchat/pkg/ttsprovider"
	aiService "sealchat/service/ai"
	"sealchat/utils"
)

const aliyunBeijingHostSuffix = ".cn-beijing.maas.aliyuncs.com"

var ErrTTSProviderCredential = errors.New("Base URL 与 API Key 不匹配，或 API Key 无权访问该业务空间")

var lookupTTSModelsDevPricing = aiService.LookupModelsDevPricing

type TTSResolvedModel struct {
	ttsprovider.TTSModelPricing
	ID            string                        `json:"id"`
	ProviderKind  string                        `json:"providerKind"`
	Runtime       string                        `json:"runtime"`
	Capabilities  ttsprovider.ModelCapabilities `json:"capabilities"`
	DefaultVoice  string                        `json:"defaultVoice"`
	Name          string                        `json:"name"`
	PricingSource string                        `json:"pricingSource"`
	DesignPrice   *float64                      `json:"designPrice"`
	ClonePrice    float64                       `json:"clonePrice"`
}

type TTSProviderResolution struct {
	ProviderID        string             `json:"providerId"`
	ProviderKind      string             `json:"providerKind"`
	Workspace         string             `json:"workspace"`
	Region            string             `json:"region"`
	CredentialScope   string             `json:"credentialScope"`
	SynthesisEndpoint string             `json:"synthesisEndpoint"`
	VoiceEndpoint     string             `json:"voiceEndpoint"`
	Models            []TTSResolvedModel `json:"models"`
}

type TTSProviderResolveRequest struct {
	ProviderKind  string
	Model         string
	BaseURL       string
	APIKey        string
	SecretID      string
	SecretKey     string
	ProviderID    string
	SavedProvider *utils.SpeechProviderConfig
}

func ResolveTTSProvider(ctx context.Context, request TTSProviderResolveRequest) (TTSProviderResolution, error) {
	kind := strings.TrimSpace(request.ProviderKind)
	if kind == "" {
		kind = ttsprovider.ProviderAliyun
	}
	var result TTSProviderResolution
	var err error
	switch kind {
	case ttsprovider.ProviderAliyun:
		if strings.TrimSpace(request.APIKey) == "" && request.SavedProvider != nil {
			if request.SavedProvider.EffectiveProviderKind() != kind {
				return result, TTSValidationError("更换服务商时必须重新填写 API Key")
			}
			parsed, parseErr := ParseAliyunTTSBaseURL(request.BaseURL)
			if parseErr != nil {
				return result, parseErr
			}
			if parsed.Workspace != request.SavedProvider.Workspace {
				return result, TTSValidationError("更换业务空间时必须重新填写 API Key")
			}
			request.APIKey = request.SavedProvider.APIKey
		}
		result, err = ResolveAliyunTTSProvider(ctx, request.BaseURL, strings.TrimSpace(request.APIKey), request.ProviderID)
	case ttsprovider.ProviderTencent:
		request.Model = strings.TrimSpace(request.Model)
		if request.Model == "" {
			request.Model = "tencent-tts-classic"
		}
		spec, ok := ttsprovider.LookupModel(kind, request.Model)
		if !ok {
			return result, TTSValidationError("请选择受支持的腾讯云 TTS 模型")
		}
		switch spec.Runtime {
		case ttsprovider.RuntimeTencentTTS:
			result, err = ResolveTencentTTSProvider(ctx, request)
		case ttsprovider.RuntimeTencentMPS:
			result, err = ResolveTencentMPSProvider(ctx, request)
		default:
			return result, TTSValidationError("请选择受支持的腾讯云 TTS 模型")
		}
	default:
		return result, TTSValidationError(fmt.Sprintf("不支持的 TTS 服务商：%s", kind))
	}
	if err != nil {
		return result, err
	}
	if modelID := strings.TrimSpace(request.Model); modelID != "" {
		for _, model := range result.Models {
			if model.ID == modelID && model.ProviderKind == kind {
				return result, nil
			}
		}
		return result, TTSValidationError(fmt.Sprintf("服务商 %s 的模型目录未返回所选 TTS 模型：%s", kind, modelID))
	}
	return result, nil
}

// Public creation choices expose configured models and prices, never credentials.
type TTSVoiceCreationModel struct {
	ID                      string                        `json:"id"`
	ProviderKind            string                        `json:"providerKind"`
	Name                    string                        `json:"name"`
	Capabilities            ttsprovider.ModelCapabilities `json:"capabilities"`
	CloneLanguages          []string                      `json:"cloneLanguages,omitempty"`
	SupportsClonePreprocess bool                          `json:"supportsClonePreprocess,omitempty"`
}

type TTSVoiceCreationProvider struct {
	ProviderID   string                  `json:"providerId"`
	ProviderKind string                  `json:"providerKind"`
	Models       []TTSVoiceCreationModel `json:"models"`
	DesignPrice  *float64                `json:"designPrice"`
	ClonePrice   *float64                `json:"clonePrice"`
}

func TTSVoiceCreationProviders() ([]TTSVoiceCreationProvider, error) {
	cfg, err := ttsConfig()
	if err != nil {
		return nil, err
	}
	providers := []TTSVoiceCreationProvider{}
	for _, p := range cfg.Providers {
		spec, supported := ttsprovider.LookupModel(p.EffectiveProviderKind(), p.Model)
		if !p.Enabled || !p.CredentialsReady() || !supported || (!spec.Capabilities.VoiceDesign && !spec.Capabilities.VoiceClone) {
			continue
		}
		model := TTSVoiceCreationModel{ID: spec.ID, ProviderKind: spec.ProviderKind, Name: spec.ID, Capabilities: spec.Capabilities, CloneLanguages: spec.CloneLanguages, SupportsClonePreprocess: spec.SupportsClonePreprocess}
		providers = append(providers, TTSVoiceCreationProvider{ProviderID: p.ID, ProviderKind: p.EffectiveProviderKind(), Models: []TTSVoiceCreationModel{model}, DesignPrice: p.DesignPrice, ClonePrice: p.ClonePrice})
	}
	return providers, nil
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
		ProviderKind:      ttsprovider.ProviderAliyun,
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

	for _, model := range models {
		spec, supported := ttsprovider.LookupModel(ttsprovider.ProviderAliyun, model.Model)
		if !supported || !spec.Capabilities.HTTPStreaming {
			continue
		}
		resolved := resolveTTSModelPricing(model, spec)
		if spec.Pricing.Mode == ttsprovider.PricingToken && resolved.PricingSource != "online" {
			if pricing, found, lookupErr := lookupTTSModelsDevPricing(ctx, "alibaba", "Alibaba", model.Model); lookupErr == nil && found {
				resolved = resolveTTSModelPricing(model, spec, pricing)
			}
		}
		resolved.DesignPrice = designPrice
		result.Models = append(result.Models, resolved)
	}
	if len(result.Models) == 0 {
		return result, TTSValidationError("百炼模型目录未返回受支持的 Qwen Audio TTS 模型")
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

type aliyunNormalizedPrice struct {
	Kind  string
	Price float64
}

func normalizeAliyunPrice(item aliyunPriceItem) (aliyunNormalizedPrice, bool) {
	price, ok := parseAliyunPrice(item.Price)
	if !ok {
		return aliyunNormalizedPrice{}, false
	}
	unit := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(item.PriceUnit), " ", ""))
	unit = strings.ReplaceAll(unit, ",", "")
	var kind string
	var divisor float64
	switch unit {
	case "每万字符", "每1万字符", "per10000characters", "per10kcharacters":
		kind, divisor = "character", 10000
	case "每千字符", "每1千字符", "per1000characters", "per1kcharacters":
		kind, divisor = "character", 1000
	case "每字符", "percharacter":
		kind, divisor = "character", 1
	case "每百万token", "每100万token", "每1百万token", "per1000000tokens", "per1mtokens", "permilliontokens":
		kind, divisor = "token", 1000000
	case "每千token", "每1千token", "per1000tokens", "per1ktokens":
		kind, divisor = "token", 1000
	case "每token", "pertoken":
		kind, divisor = "token", 1
	case "每次", "每次调用", "perrequest", "percall":
		kind, divisor = "request", 1
	default:
		return aliyunNormalizedPrice{}, false
	}
	if kind == "token" {
		label := strings.ToLower(item.Type + " " + item.PriceName)
		switch {
		case strings.Contains(label, "input") || strings.Contains(label, "prompt") || strings.Contains(label, "输入"):
			kind = "input_token"
		case strings.Contains(label, "output") || strings.Contains(label, "completion") || strings.Contains(label, "输出"):
			kind = "output_token"
		default:
			return aliyunNormalizedPrice{}, false
		}
	}
	return aliyunNormalizedPrice{Kind: kind, Price: price / divisor}, true
}

func resolveTTSModelPricing(model aliyunCatalogModel, spec ttsprovider.ModelSpec, modelsDevPricing ...utils.AIModelPricingConfig) TTSResolvedModel {
	pricing := ttsprovider.TTSModelPricing{Mode: spec.Pricing.Mode}
	for _, priceRange := range model.Prices {
		for _, item := range priceRange.Prices {
			price, ok := normalizeAliyunPrice(item)
			if !ok {
				continue
			}
			switch price.Kind {
			case "character":
				if pricing.CharacterPrice == nil {
					pricing.CharacterPrice = &price.Price
				}
			case "input_token":
				if pricing.InputTokenPrice == nil {
					pricing.InputTokenPrice = &price.Price
				}
			case "output_token":
				if pricing.OutputTokenPrice == nil {
					pricing.OutputTokenPrice = &price.Price
				}
			}
		}
	}
	source := "online"
	if pricing.Mode == ttsprovider.PricingToken && len(modelsDevPricing) == 1 && (pricing.InputTokenPrice == nil || pricing.OutputTokenPrice == nil) {
		input := modelsDevPricing[0].PromptPricePer1MTokens / 1000000
		output := modelsDevPricing[0].CompletionPricePer1MTokens / 1000000
		if input >= 0 && output >= 0 && !math.IsNaN(input) && !math.IsNaN(output) && !math.IsInf(input, 0) && !math.IsInf(output, 0) {
			if pricing.InputTokenPrice == nil && pricing.OutputTokenPrice == nil {
				source = "modelsdev"
			} else {
				source = "mixed"
			}
			if pricing.InputTokenPrice == nil {
				pricing.InputTokenPrice = &input
			}
			if pricing.OutputTokenPrice == nil {
				pricing.OutputTokenPrice = &output
			}
		}
	}
	if pricing.Mode == ttsprovider.PricingCharacter {
		pricing.InputTokenPrice, pricing.OutputTokenPrice = nil, nil
		if pricing.CharacterPrice == nil {
			pricing, source = spec.Pricing, "builtin"
		}
	} else {
		pricing.CharacterPrice = nil
		if pricing.InputTokenPrice == nil && pricing.OutputTokenPrice == nil {
			pricing, source = spec.Pricing, "builtin"
		} else {
			if pricing.InputTokenPrice == nil && spec.Pricing.InputTokenPrice != nil {
				pricing.InputTokenPrice, source = spec.Pricing.InputTokenPrice, "mixed"
			}
			if pricing.OutputTokenPrice == nil && spec.Pricing.OutputTokenPrice != nil {
				pricing.OutputTokenPrice, source = spec.Pricing.OutputTokenPrice, "mixed"
			}
		}
	}
	format := func(price float64) string { return strconv.FormatFloat(math.Round(price*1e9)/1e9, 'f', -1, 64) }
	switch {
	case pricing.Mode == ttsprovider.PricingCharacter && pricing.CharacterPrice != nil:
		pricing.DisplayPrice = format(*pricing.CharacterPrice*10000) + " 元 / 万字符"
	case pricing.Mode == ttsprovider.PricingToken && pricing.InputTokenPrice != nil && pricing.OutputTokenPrice != nil:
		pricing.DisplayPrice = "输入 " + format(*pricing.InputTokenPrice*1000000) + " 元 / 百万 Token；输出 " + format(*pricing.OutputTokenPrice*1000000) + " 元 / 百万 Token"
	default:
		pricing.DisplayPrice, source = "价格未确认", "unknown"
	}
	if source == "builtin" {
		pricing.DisplayPrice = spec.Pricing.DisplayPrice
	}
	name := strings.TrimSpace(model.Name)
	if name == "" {
		name = spec.Name
	}
	if name == "" {
		name = spec.ID
	}
	return TTSResolvedModel{ID: spec.ID, ProviderKind: spec.ProviderKind, Runtime: spec.Runtime, Capabilities: spec.Capabilities, DefaultVoice: ttsprovider.DefaultVoice(spec.ProviderKind, spec.ID), Name: name, TTSModelPricing: pricing, PricingSource: source}
}

func ResolveTencentTTSProvider(ctx context.Context, request TTSProviderResolveRequest) (TTSProviderResolution, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return resolveTencentTTSProvider(ctx, &ttsprovider.TencentClient{HTTP: &http.Client{Timeout: 12 * time.Second}}, request)
}

// A probe is charged once, only on explicit admin resolution. It never stores
// audio and never retries a possible charge.
func resolveTencentTTSProvider(ctx context.Context, client *ttsprovider.TencentClient, request TTSProviderResolveRequest) (TTSProviderResolution, error) {
	result := TTSProviderResolution{ProviderKind: ttsprovider.ProviderTencent, SynthesisEndpoint: ttsprovider.TencentEndpoint, Models: []TTSResolvedModel{}}
	secretID, secretKey, err := ttsTencentSecrets(request)
	if err != nil {
		return result, err
	}
	modelID := strings.TrimSpace(request.Model)
	spec, ok := ttsprovider.LookupModel(ttsprovider.ProviderTencent, modelID)
	if !ok || spec.Runtime != ttsprovider.RuntimeTencentTTS {
		return result, TTSValidationError("请选择受支持的腾讯传统 TTS 模型")
	}
	probe := ttsprovider.Input{Text: "测", Language: "zh", Voice: spec.DefaultVoice, Format: "wav", SampleRate: ttsprovider.TencentSampleRate(modelID), Rate: 1, Pitch: 1, Volume: 50}
	c := *client
	c.SecretID, c.SecretKey = secretID, secretKey
	_, err = c.Synthesize(ctx, modelID, probe, io.Discard)
	if err != nil {
		var providerError *ttsprovider.ProviderError
		if errors.As(err, &providerError) {
			return result, TTSValidationError(ttsprovider.TencentProviderErrorMessage(providerError))
		}
		return result, err
	}
	result.ProviderID = strings.TrimSpace(request.ProviderID)
	if result.ProviderID == "" {
		result.ProviderID = utils.NewID()
	}
	result.CredentialScope = ttsprovider.TencentCredentialScope(secretID)
	for _, model := range ttsprovider.ModelCatalog() {
		if model.ProviderKind == ttsprovider.ProviderTencent && model.Runtime == ttsprovider.RuntimeTencentTTS {
			result.Models = append(result.Models, resolveTTSModelPricing(aliyunCatalogModel{}, model))
		}
	}
	return result, nil
}

func ttsTencentSecrets(request TTSProviderResolveRequest) (string, string, error) {
	secretID, secretKey := strings.TrimSpace(request.SecretID), strings.TrimSpace(request.SecretKey)
	if (secretID == "") != (secretKey == "") {
		return "", "", TTSValidationError("SecretId 与 SecretKey 必须同时填写")
	}
	if secretID == "" {
		old := request.SavedProvider
		if old == nil || old.ID != strings.TrimSpace(request.ProviderID) || old.EffectiveProviderKind() != ttsprovider.ProviderTencent || !old.CredentialsReady() || old.CredentialScope != ttsprovider.TencentCredentialScope(old.SecretID) {
			return "", "", TTSValidationError("请同时填写腾讯云 SecretId 与 SecretKey；更换服务商时必须重新填写凭据")
		}
		secretID, secretKey = old.SecretID, old.SecretKey
	}
	return secretID, secretKey, nil
}

func ResolveTencentMPSProvider(ctx context.Context, request TTSProviderResolveRequest) (TTSProviderResolution, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return resolveTencentMPSProvider(ctx, &ttsprovider.TencentMPSClient{HTTP: &http.Client{Timeout: 12 * time.Second}}, request)
}

// Configuration verification queries voices and never issues a synthesis POST.
func resolveTencentMPSProvider(ctx context.Context, client *ttsprovider.TencentMPSClient, request TTSProviderResolveRequest) (TTSProviderResolution, error) {
	result := TTSProviderResolution{ProviderKind: ttsprovider.ProviderTencent, SynthesisEndpoint: ttsprovider.TencentMPSEndpoint, Models: []TTSResolvedModel{}}
	secretID, secretKey, err := ttsTencentSecrets(request)
	if err != nil {
		return result, err
	}
	spec, ok := ttsprovider.LookupModel(ttsprovider.ProviderTencent, strings.TrimSpace(request.Model))
	if !ok || spec.Runtime != ttsprovider.RuntimeTencentMPS {
		return result, TTSValidationError("请选择受支持的腾讯 MPS MiniMax 模型")
	}
	c := *client
	c.SecretID, c.SecretKey = secretID, secretKey
	voices, _, err := c.DescribeVoices(ctx)
	if err != nil {
		var providerError *ttsprovider.ProviderError
		if errors.As(err, &providerError) {
			return result, TTSValidationError(ttsprovider.TencentMPSProviderErrorMessage(providerError))
		}
		return result, err
	}
	dynamic := ttsprovider.TencentMPSVoiceSpecs(voices)
	if len(dynamic) == 0 {
		return result, TTSValidationError("腾讯 MPS 未返回可用的系统音色")
	}
	result.ProviderID = strings.TrimSpace(request.ProviderID)
	if result.ProviderID == "" {
		result.ProviderID = utils.NewID()
	}
	result.CredentialScope = ttsprovider.TencentCredentialScope(secretID)
	ttsMPSCatalog.prime(result.CredentialScope, dynamic)
	for _, model := range ttsprovider.ModelCatalog() {
		if model.ProviderKind == ttsprovider.ProviderTencent && model.Runtime == ttsprovider.RuntimeTencentMPS {
			resolved := resolveTTSModelPricing(aliyunCatalogModel{}, model)
			resolved.DefaultVoice = ttsMPSDefaultVoice(dynamic)
			result.Models = append(result.Models, resolved)
		}
	}
	return result, nil
}

func TTSModelCatalog() []TTSResolvedModel {
	models := []TTSResolvedModel{}
	for _, spec := range ttsprovider.ModelCatalog() {
		if spec.Capabilities.HTTPStreaming {
			models = append(models, resolveTTSModelPricing(aliyunCatalogModel{Model: spec.ID}, spec))
		}
	}
	return models
}

func confirmedVoiceDesignPrice(models []aliyunCatalogModel) *float64 {
	for _, model := range models {
		if model.Model != "voice-enrollment" {
			continue
		}
		for _, priceRange := range model.Prices {
			for _, item := range priceRange.Prices {
				label := strings.ToLower(item.Type + " " + item.PriceName)
				design := strings.Contains(label, "design") || strings.Contains(label, "声音设计") || strings.Contains(label, "音色设计")
				price, ok := normalizeAliyunPrice(item)
				if !ok || price.Kind != "request" || !design {
					continue
				}
				return &price.Price
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
	return price, err == nil && price >= 0 && !math.IsNaN(price) && !math.IsInf(price, 0)
}

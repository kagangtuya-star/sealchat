package utils

import (
	"fmt"
	"math"
	"net/url"
	"strings"

	"sealchat/pkg/ttsprovider"
)

// Nil prices are unconfirmed; an explicitly configured zero is valid.
type SpeechProviderConfig struct {
	ID                string   `json:"id" yaml:"id"`
	ProviderKind      string   `json:"providerKind" yaml:"providerKind"`
	Enabled           bool     `json:"enabled" yaml:"enabled"`
	CredentialScope   string   `json:"credentialScope" yaml:"credentialScope"`
	Region            string   `json:"region" yaml:"region"`
	Workspace         string   `json:"workspace" yaml:"workspace"`
	APIKey            string   `json:"apiKey" yaml:"apiKey"`
	HasAPIKey         bool     `json:"hasApiKey" yaml:"-"`
	SynthesisEndpoint string   `json:"synthesisEndpoint" yaml:"synthesisEndpoint"`
	VoiceEndpoint     string   `json:"voiceEndpoint" yaml:"voiceEndpoint"`
	Model             string   `json:"model" yaml:"model"`
	CharacterPrice    *float64 `json:"characterPrice" yaml:"characterPrice"`
	PricingMode       string   `json:"pricingMode,omitempty" yaml:"pricingMode,omitempty"`
	InputTokenPrice   *float64 `json:"inputTokenPrice" yaml:"inputTokenPrice"`
	OutputTokenPrice  *float64 `json:"outputTokenPrice" yaml:"outputTokenPrice"`
	DesignPrice       *float64 `json:"designPrice" yaml:"designPrice"`
	ClonePrice        *float64 `json:"clonePrice" yaml:"clonePrice"`
	AccountVoiceLimit *int     `json:"accountVoiceLimit" yaml:"accountVoiceLimit"`
	Revision          int64    `json:"revision" yaml:"revision"`
}

// Empty kinds belong to the legacy Aliyun compatible-mode configuration.
func (p SpeechProviderConfig) EffectiveProviderKind() string {
	if p.ProviderKind == "" {
		return ttsprovider.ProviderAliyun
	}
	return p.ProviderKind
}

func (p SpeechProviderConfig) EffectivePricingMode() string {
	if p.PricingMode == "" {
		return ttsprovider.PricingCharacter
	}
	return p.PricingMode
}

func (p SpeechProviderConfig) SynthesisPriceConfirmed() bool {
	switch p.EffectivePricingMode() {
	case ttsprovider.PricingCharacter:
		return p.CharacterPrice != nil
	case ttsprovider.PricingToken:
		return p.InputTokenPrice != nil && p.OutputTokenPrice != nil
	default:
		return false
	}
}

type SpeechConfig struct {
	Enabled               bool                   `json:"enabled" yaml:"enabled"`
	Providers             []SpeechProviderConfig `json:"providers" yaml:"providers"`
	DefaultProvider       string                 `json:"defaultProvider" yaml:"defaultProvider"`
	DefaultVoice          string                 `json:"defaultVoice" yaml:"defaultVoice"`
	Format                string                 `json:"format" yaml:"format"`
	QuotaDefault          AIQuotaPolicyConfig    `json:"quotaDefault" yaml:"quotaDefault"`
	DefaultSlots          int                    `json:"defaultSlots" yaml:"defaultSlots"`
	PreviewTTLMinutes     int                    `json:"previewTTLMinutes" yaml:"previewTTLMinutes"`
	PreviewLimit          int                    `json:"previewLimit" yaml:"previewLimit"`
	RequestTimeoutSeconds int                    `json:"requestTimeoutSeconds" yaml:"requestTimeoutSeconds"`
	MaxConcurrent         int                    `json:"maxConcurrent" yaml:"maxConcurrent"`
	ChannelQueueLimit     int                    `json:"channelQueueLimit" yaml:"channelQueueLimit"`
}

func NormalizeSpeechConfig(cfg *SpeechConfig) *SpeechConfig {
	return normalizeSpeechConfig(cfg, true)
}

// Explicit nonempty selections must survive normalization so validation can
// reject them. Historical reads may repair a stale selection instead.
func NormalizeSpeechConfigForWrite(cfg *SpeechConfig) *SpeechConfig {
	return normalizeSpeechConfig(cfg, false)
}

func normalizeSpeechConfig(cfg *SpeechConfig, repairVoice bool) *SpeechConfig {
	if cfg == nil {
		return nil
	}
	out := *cfg
	out.Providers = append([]SpeechProviderConfig{}, cfg.Providers...)
	if out.Format == "" || out.Format == "opus" {
		out.Format = "wav"
	}
	if out.PreviewTTLMinutes <= 0 {
		out.PreviewTTLMinutes = 30
	}
	if out.PreviewLimit <= 0 {
		out.PreviewLimit = 2
	}
	if out.RequestTimeoutSeconds <= 0 {
		out.RequestTimeoutSeconds = 90
	}
	if out.MaxConcurrent <= 0 {
		out.MaxConcurrent = 2
	}
	if out.ChannelQueueLimit <= 0 {
		out.ChannelQueueLimit = 8
	}
	for i := range out.Providers {
		p := &out.Providers[i]
		p.ProviderKind = p.EffectiveProviderKind()
		if p.Model == "" && p.ProviderKind == ttsprovider.ProviderAliyun {
			p.Model = "qwen-audio-3.0-tts-flash"
		}
		if p.Region == "" && p.ProviderKind == ttsprovider.ProviderAliyun {
			p.Region = "cn-beijing"
		}
		if p.Revision <= 0 {
			p.Revision = 1
		}
	}
	for _, p := range out.Providers {
		if p.ID == out.DefaultProvider && (out.DefaultVoice == "" || (repairVoice && !ttsprovider.VoiceSupported(p.ProviderKind, p.Model, out.DefaultVoice))) {
			out.DefaultVoice = ttsprovider.DefaultVoice(p.ProviderKind, p.Model)
			break
		}
	}
	return &out
}

func validateSpeechEndpoint(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == "" && !strings.Contains(value, "#")
}

func validateSpeechProviderRuntimeFields(p SpeechProviderConfig, spec ttsprovider.ModelSpec) error {
	switch p.EffectiveProviderKind() {
	case ttsprovider.ProviderAliyun:
		if p.CredentialScope == "" || p.Workspace == "" || p.Region != "cn-beijing" {
			return fmt.Errorf("请配置语音账号命名空间、北京业务空间与 API Key")
		}
		if (spec.Capabilities.VoiceDesign || spec.Capabilities.VoiceClone) && !validateSpeechEndpoint(p.VoiceEndpoint) {
			return fmt.Errorf("语音接口必须是管理员配置的 HTTPS 地址")
		}
	}
	return nil
}

func validateSpeechProviderConfig(p SpeechProviderConfig, spec ttsprovider.ModelSpec) error {
	if p.EffectivePricingMode() != spec.Pricing.Mode {
		return fmt.Errorf("语音定价模式与模型不匹配")
	}
	for _, price := range []*float64{p.CharacterPrice, p.InputTokenPrice, p.OutputTokenPrice, p.DesignPrice, p.ClonePrice} {
		if price != nil && (*price < 0 || math.IsNaN(*price) || math.IsInf(*price, 0)) {
			return fmt.Errorf("语音单位值无效")
		}
	}
	if p.AccountVoiceLimit != nil && *p.AccountVoiceLimit < 0 {
		return fmt.Errorf("账号音色上限无效")
	}
	if !p.Enabled {
		return nil
	}
	if strings.TrimSpace(p.APIKey) == "" {
		if p.EffectiveProviderKind() == ttsprovider.ProviderAliyun {
			return fmt.Errorf("请配置语音账号命名空间、北京业务空间与 API Key")
		}
		return fmt.Errorf("请配置语音 API Key")
	}
	if !validateSpeechEndpoint(p.SynthesisEndpoint) {
		return fmt.Errorf("语音接口必须是管理员配置的 HTTPS 地址")
	}
	return validateSpeechProviderRuntimeFields(p, spec)
}

func ValidateSpeechConfig(cfg *SpeechConfig) error {
	if cfg == nil {
		return nil
	}
	if cfg.DefaultSlots < 0 || cfg.PreviewLimit < 1 || cfg.PreviewLimit > 10 || cfg.PreviewTTLMinutes < 1 || cfg.PreviewTTLMinutes > 1440 || cfg.MaxConcurrent < 1 || cfg.MaxConcurrent > 8 || cfg.ChannelQueueLimit < 1 || cfg.ChannelQueueLimit > 100 || cfg.RequestTimeoutSeconds < 1 || cfg.RequestTimeoutSeconds > 300 {
		return fmt.Errorf("语音容量或时限配置无效")
	}
	if cfg.Format != "mp3" && cfg.Format != "wav" {
		return fmt.Errorf("语音格式不受支持")
	}
	for _, v := range []*float64{cfg.QuotaDefault.DailyLimit, cfg.QuotaDefault.MonthlyLimit, cfg.QuotaDefault.LifetimeLimit} {
		if v != nil && (*v < 0 || math.IsNaN(*v) || math.IsInf(*v, 0)) {
			return fmt.Errorf("语音限制无效")
		}
	}
	ids := map[string]bool{}
	found := false
	for _, p := range cfg.Providers {
		if strings.TrimSpace(p.ID) == "" || ids[p.ID] {
			return fmt.Errorf("语音 provider ID 为空或重复")
		}
		ids[p.ID] = true
		spec, supported := ttsprovider.LookupModel(p.EffectiveProviderKind(), p.Model)
		if !supported || !spec.Capabilities.HTTPStreaming {
			return fmt.Errorf("语音模型不受支持")
		}
		if err := validateSpeechProviderConfig(p, spec); err != nil {
			return err
		}
		if !p.Enabled {
			continue
		}
		if p.ID == cfg.DefaultProvider {
			found = true
			if !ttsprovider.VoiceSupported(p.EffectiveProviderKind(), p.Model, cfg.DefaultVoice) {
				return fmt.Errorf("默认系统音色与默认 provider 类型或模型不匹配")
			}
			if cfg.Enabled && !p.SynthesisPriceConfirmed() {
				return fmt.Errorf("请显式确认合成单价")
			}
		}
	}
	if (cfg.Enabled || cfg.DefaultProvider != "" || cfg.DefaultVoice != "") && !found {
		return fmt.Errorf("请选择已启用的默认语音 provider")
	}
	return nil
}

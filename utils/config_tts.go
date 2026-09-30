package utils

import (
	"fmt"
	"math"
	"net/url"
	"strings"
)

// Nil prices are unconfirmed; an explicitly configured zero is valid.
type SpeechProviderConfig struct {
	ID                string   `json:"id" yaml:"id"`
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
	DesignPrice       *float64 `json:"designPrice" yaml:"designPrice"`
	ClonePrice        *float64 `json:"clonePrice" yaml:"clonePrice"`
	AccountVoiceLimit *int     `json:"accountVoiceLimit" yaml:"accountVoiceLimit"`
	Revision          int64    `json:"revision" yaml:"revision"`
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
	if cfg == nil {
		return nil
	}
	out := *cfg
	out.Providers = append([]SpeechProviderConfig{}, cfg.Providers...)
	if out.Format == "" || out.Format == "opus" {
		out.Format = "wav"
	}
	if out.DefaultVoice == "" {
		out.DefaultVoice = "longanhuan_v3.6"
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
		if p.Model == "" {
			p.Model = "qwen-audio-3.0-tts-flash"
		}
		if p.Region == "" {
			p.Region = "cn-beijing"
		}
		if p.Revision <= 0 {
			p.Revision = 1
		}
	}
	return &out
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
		if p.Model != "qwen-audio-3.0-tts-flash" && p.Model != "qwen-audio-3.0-tts-plus" {
			return fmt.Errorf("语音模型不受支持")
		}
		for _, price := range []*float64{p.CharacterPrice, p.DesignPrice, p.ClonePrice} {
			if price != nil && (*price < 0 || math.IsNaN(*price) || math.IsInf(*price, 0)) {
				return fmt.Errorf("语音单位值无效")
			}
		}
		if p.AccountVoiceLimit != nil && *p.AccountVoiceLimit < 0 {
			return fmt.Errorf("账号音色上限无效")
		}
		if !p.Enabled {
			continue
		}
		if p.CredentialScope == "" || p.Workspace == "" || p.Region != "cn-beijing" || strings.TrimSpace(p.APIKey) == "" {
			return fmt.Errorf("请配置语音账号命名空间、北京业务空间与 API Key")
		}
		for _, endpoint := range []string{p.SynthesisEndpoint, p.VoiceEndpoint} {
			u, err := url.Parse(endpoint)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
				return fmt.Errorf("语音接口必须是管理员配置的 HTTPS 地址")
			}
		}
		if p.ID == cfg.DefaultProvider {
			found = true
			if cfg.Enabled && p.CharacterPrice == nil {
				return fmt.Errorf("请显式确认合成字符单位值")
			}
		}
	}
	if cfg.Enabled && !found {
		return fmt.Errorf("请选择已启用的默认语音 provider")
	}
	return nil
}

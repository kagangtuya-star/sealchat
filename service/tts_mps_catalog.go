package service

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"sealchat/pkg/ttsprovider"
	"sealchat/utils"
)

const ttsMPSCatalogTTL = 10 * time.Minute

type ttsMPSCatalogEntry struct {
	voices    []ttsprovider.TTSVoiceSpec
	updatedAt time.Time
}

type ttsMPSCatalogFlight struct {
	done chan struct{}
	err  error
}

// Process-local, credential-scoped cache. Reads used by message preparation are
// always local, including while a directory refresh is running.
type ttsMPSVoiceCache struct {
	mu      sync.Mutex
	entries map[string]ttsMPSCatalogEntry
	flights map[string]*ttsMPSCatalogFlight
}

var ttsMPSCatalog = ttsMPSVoiceCache{entries: map[string]ttsMPSCatalogEntry{}, flights: map[string]*ttsMPSCatalogFlight{}}

// TTSPrimeMPSCatalogs runs one bounded batch of catalog refreshes. Startup and
// config-save callers invoke it in a goroutine; message preparation stays local.
func TTSPrimeMPSCatalogs() {
	cfg := utils.GetConfig()
	if cfg == nil || cfg.AI.Speech == nil {
		return
	}
	speech := utils.NormalizeSpeechConfig(cfg.AI.Speech)
	seenScopes := map[string]bool{}
	var wg sync.WaitGroup
	for _, p := range speech.Providers {
		if !p.Enabled || !p.CredentialsReady() || !ttsIsMPSProvider(p, p.Model) || seenScopes[p.CredentialScope] {
			continue
		}
		seenScopes[p.CredentialScope] = true
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, _ = ttsMPSCatalog.refresh(ctx, p.CredentialScope, func(ctx context.Context) ([]ttsprovider.TTSVoiceSpec, error) {
				return ttsFetchMPSVoices(ctx, p)
			})
		}()
	}
	wg.Wait()
}

func ttsMPSCatalogKey(scope string) string { return scope + ":minimax" }

func cloneTTSMPSVoices(voices []ttsprovider.TTSVoiceSpec) []ttsprovider.TTSVoiceSpec {
	out := make([]ttsprovider.TTSVoiceSpec, len(voices))
	for i, v := range voices {
		v.Models, v.Languages, v.SpeechLanguages = slices.Clone(v.Models), slices.Clone(v.Languages), slices.Clone(v.SpeechLanguages)
		out[i] = v
	}
	return out
}

func (c *ttsMPSVoiceCache) read(scope string) []ttsprovider.TTSVoiceSpec {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneTTSMPSVoices(c.entries[ttsMPSCatalogKey(scope)].voices)
}

func (c *ttsMPSVoiceCache) prime(scope string, voices []ttsprovider.TTSVoiceSpec) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[ttsMPSCatalogKey(scope)] = ttsMPSCatalogEntry{cloneTTSMPSVoices(voices), time.Now()}
}

func (c *ttsMPSVoiceCache) refresh(ctx context.Context, scope string, fetch func(context.Context) ([]ttsprovider.TTSVoiceSpec, error)) ([]ttsprovider.TTSVoiceSpec, error) {
	key := ttsMPSCatalogKey(scope)
	c.mu.Lock()
	entry := c.entries[key]
	if !entry.updatedAt.IsZero() && time.Since(entry.updatedAt) < ttsMPSCatalogTTL {
		c.mu.Unlock()
		return cloneTTSMPSVoices(entry.voices), nil
	}
	if flight := c.flights[key]; flight != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return c.read(scope), ctx.Err()
		case <-flight.done:
			return c.read(scope), flight.err
		}
	}
	flight := &ttsMPSCatalogFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()
	voices, err := fetch(ctx)
	c.mu.Lock()
	if err == nil {
		c.entries[key] = ttsMPSCatalogEntry{cloneTTSMPSVoices(voices), time.Now()}
	}
	entry = c.entries[key]
	flight.err = err
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()
	return cloneTTSMPSVoices(entry.voices), err
}

func ttsFetchMPSVoices(ctx context.Context, provider utils.SpeechProviderConfig) ([]ttsprovider.TTSVoiceSpec, error) {
	client := ttsprovider.TencentMPSClient{SecretID: provider.SecretID, SecretKey: provider.SecretKey, Endpoint: provider.SynthesisEndpoint}
	voices, _, err := client.DescribeVoices(ctx)
	if err != nil {
		return nil, err
	}
	if len(voices) == 0 {
		return nil, errors.New("腾讯 MPS 未返回系统音色")
	}
	return ttsprovider.TencentMPSVoiceSpecs(voices), nil
}

func ttsIsMPSProvider(provider utils.SpeechProviderConfig, modelID string) bool {
	spec, ok := ttsprovider.LookupModel(provider.EffectiveProviderKind(), modelID)
	return ok && spec.Runtime == ttsprovider.RuntimeTencentMPS
}

func ttsMPSSystemVoice(provider utils.SpeechProviderConfig, modelID, voiceID string) (ttsprovider.TTSVoiceSpec, bool) {
	for _, voice := range ttsMPSCatalog.read(provider.CredentialScope) {
		if voice.ID == voiceID && voice.Supports(provider.EffectiveProviderKind(), modelID) {
			return voice, true
		}
	}
	return ttsprovider.TTSVoiceSpec{}, false
}

func ttsSystemVoiceSupported(provider utils.SpeechProviderConfig, modelID, voiceID string) bool {
	if !ttsIsMPSProvider(provider, modelID) {
		return ttsprovider.VoiceSupported(provider.EffectiveProviderKind(), modelID, voiceID)
	}
	_, ok := ttsMPSSystemVoice(provider, modelID, voiceID)
	return ok
}

func ttsSystemVoiceLanguages(provider utils.SpeechProviderConfig, modelID, voiceID string) []string {
	if !ttsIsMPSProvider(provider, modelID) {
		return ttsprovider.VoiceLanguages(provider.EffectiveProviderKind(), modelID, voiceID)
	}
	voice, _ := ttsMPSSystemVoice(provider, modelID, voiceID)
	return voice.Languages
}

func ttsSystemVoiceSpeechLanguages(provider utils.SpeechProviderConfig, modelID, voiceID string) []string {
	if !ttsIsMPSProvider(provider, modelID) {
		return ttsprovider.VoiceSpeechLanguages(provider.EffectiveProviderKind(), modelID, voiceID)
	}
	voice, _ := ttsMPSSystemVoice(provider, modelID, voiceID)
	return voice.SpeechLanguages
}

func ttsMPSDefaultVoice(voices []ttsprovider.TTSVoiceSpec) string {
	for _, v := range voices {
		if slices.Contains(v.Languages, "zh") {
			return v.ID
		}
	}
	if len(voices) > 0 {
		return voices[0].ID
	}
	return ""
}

func ttsDefaultSystemVoice(provider utils.SpeechProviderConfig, modelID string) string {
	if !ttsIsMPSProvider(provider, modelID) {
		return ttsprovider.DefaultVoice(provider.EffectiveProviderKind(), modelID)
	}
	return ttsMPSDefaultVoice(ttsMPSCatalog.read(provider.CredentialScope))
}

// ValidateTTSDynamicDefaultVoice supplements config validation at the service
// boundary; utils owns static metadata and never accesses the dynamic cache.
func ValidateTTSDynamicDefaultVoice(cfg *utils.SpeechConfig) error {
	if cfg == nil {
		return nil
	}
	for _, p := range cfg.Providers {
		if p.ID == cfg.DefaultProvider && p.Enabled && ttsIsMPSProvider(p, p.Model) && !ttsSystemVoiceSupported(p, p.Model, cfg.DefaultVoice) {
			return TTSValidationError("默认系统音色与默认 provider 类型或模型不匹配，请重新获取音色目录")
		}
	}
	return nil
}

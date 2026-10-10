package service

import (
	"context"
	"sealchat/pkg/ttsprovider"
	"time"
)

// TargetModel is a derived compatibility field for older API clients only.
type TTSSystemVoice struct {
	ttsprovider.TTSVoiceSpec
	TargetModel string `json:"targetModel,omitempty"`
}

func TTSSystemVoices(contexts ...context.Context) []TTSSystemVoice {
	voices := ttsprovider.VoiceCatalog()
	// Only this explicit directory operation refreshes expired caches.
	parent := context.Background()
	if len(contexts) > 0 {
		parent = contexts[0]
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	if cfg, err := ttsConfig(); err == nil {
		seenScopes, seenVoices := map[string]bool{}, map[string]bool{}
		for _, p := range cfg.Providers {
			if !p.Enabled || !p.CredentialsReady() || !ttsIsMPSProvider(p, p.Model) || seenScopes[p.CredentialScope] {
				continue
			}
			seenScopes[p.CredentialScope] = true
			dynamic, _ := ttsMPSCatalog.refresh(ctx, p.CredentialScope, func(ctx context.Context) ([]ttsprovider.TTSVoiceSpec, error) { return ttsFetchMPSVoices(ctx, p) })
			for _, voice := range dynamic {
				if !seenVoices[voice.ID] {
					voices = append(voices, voice)
					seenVoices[voice.ID] = true
				}
			}
		}
	}
	items := []TTSSystemVoice{}
	for _, v := range voices {
		item := TTSSystemVoice{TTSVoiceSpec: v}
		if len(v.Models) == 1 {
			item.TargetModel = v.Models[0]
		}
		items = append(items, item)
	}
	return items
}

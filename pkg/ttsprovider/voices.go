package ttsprovider

import "slices"

type TTSVoiceSpec struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ProviderKind string   `json:"providerKind"`
	Models       []string `json:"models"`
	Languages    []string `json:"languages"`
	Kind         string   `json:"kind"`
	Tags         string   `json:"tags,omitempty"`
}

func (v TTSVoiceSpec) Supports(providerKind, modelID string) bool {
	return v.ProviderKind == providerKind && slices.Contains(v.Models, modelID)
}

var systemVoiceCatalog = aliyunSystemVoices()

func copyVoice(v TTSVoiceSpec) TTSVoiceSpec {
	v.Models = slices.Clone(v.Models)
	v.Languages = slices.Clone(v.Languages)
	return v
}

func VoiceCatalog() []TTSVoiceSpec {
	items := make([]TTSVoiceSpec, 0, len(systemVoiceCatalog))
	for _, v := range systemVoiceCatalog {
		items = append(items, copyVoice(v))
	}
	return items
}

func VoicesForModel(providerKind, modelID string) []TTSVoiceSpec {
	items := []TTSVoiceSpec{}
	for _, v := range systemVoiceCatalog {
		if v.Supports(providerKind, modelID) {
			items = append(items, copyVoice(v))
		}
	}
	return items
}

func VoiceSupported(providerKind, modelID, voiceID string) bool {
	for _, v := range systemVoiceCatalog {
		if v.ID == voiceID && v.Supports(providerKind, modelID) {
			return true
		}
	}
	return false
}

func DefaultVoice(providerKind, modelID string) string {
	if spec, ok := LookupModel(providerKind, modelID); ok && VoiceSupported(providerKind, modelID, spec.DefaultVoice) {
		return spec.DefaultVoice
	}
	return ""
}

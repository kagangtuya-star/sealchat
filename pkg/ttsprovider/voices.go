package ttsprovider

import "slices"

type TTSVoiceSpec struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ProviderKind string   `json:"providerKind"`
	PresetSource string   `json:"presetSource,omitempty"`
	Models       []string `json:"models"`
	Languages    []string `json:"languages"`
	// SpeechLanguages are SealChat AI translation targets; empty falls back to Languages.
	SpeechLanguages []string `json:"speechLanguages,omitempty"`
	Kind            string   `json:"kind"`
	Tags            string   `json:"tags,omitempty"`
}

func (v TTSVoiceSpec) Supports(providerKind, modelID string) bool {
	return v.ProviderKind == providerKind && slices.Contains(v.Models, modelID)
}

var systemVoiceCatalog = append(aliyunSystemVoices(), tencentSystemVoices()...)

func copyVoice(v TTSVoiceSpec) TTSVoiceSpec {
	v.Models = slices.Clone(v.Models)
	v.Languages = slices.Clone(v.Languages)
	v.SpeechLanguages = slices.Clone(v.SpeechLanguages)
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

// Languages are provider catalog metadata used for voice labels and filtering.
func VoiceLanguages(providerKind, modelID, voiceID string) []string {
	for _, v := range systemVoiceCatalog {
		if v.ID == voiceID && v.Supports(providerKind, modelID) {
			return slices.Clone(v.Languages)
		}
	}
	return nil
}

func VoiceLanguageSupported(providerKind, modelID, voiceID, language string) bool {
	return slices.Contains(VoiceLanguages(providerKind, modelID, voiceID), language)
}

func VoiceSpeechLanguages(providerKind, modelID, voiceID string) []string {
	for _, v := range systemVoiceCatalog {
		if v.ID == voiceID && v.Supports(providerKind, modelID) {
			if len(v.SpeechLanguages) > 0 {
				return slices.Clone(v.SpeechLanguages)
			}
			return slices.Clone(v.Languages)
		}
	}
	return nil
}

func VoiceSpeechLanguageSupported(providerKind, modelID, voiceID, language string) bool {
	return slices.Contains(VoiceSpeechLanguages(providerKind, modelID, voiceID), language)
}

func DefaultVoice(providerKind, modelID string) string {
	if spec, ok := LookupModel(providerKind, modelID); ok && VoiceSupported(providerKind, modelID, spec.DefaultVoice) {
		return spec.DefaultVoice
	}
	return ""
}

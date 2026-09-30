package service

import "sealchat/pkg/ttsprovider"

// TargetModel is a derived compatibility field for older API clients only.
type TTSSystemVoice struct {
	ttsprovider.TTSVoiceSpec
	TargetModel string `json:"targetModel,omitempty"`
}

func TTSSystemVoices() []TTSSystemVoice {
	items := []TTSSystemVoice{}
	for _, v := range ttsprovider.VoiceCatalog() {
		item := TTSSystemVoice{TTSVoiceSpec: v}
		if len(v.Models) == 1 {
			item.TargetModel = v.Models[0]
		}
		items = append(items, item)
	}
	return items
}

package ttsprovider

import (
	_ "embed"
	"encoding/json"
)

// Generated offline from the official 2026-07-23 Flash and Plus XLSX catalogs
// linked by the source page below. Voice IDs are preserved verbatim.
//
//go:embed tts_catalog_basic_20260723.json
var ttsBasicCatalogJSON []byte

type aliyunVoiceCatalogItem struct {
	TTSVoiceSpec
	TargetModel string `json:"targetModel"`
}

func loadAliyunVoiceCatalog(data []byte) []TTSVoiceSpec {
	var items []aliyunVoiceCatalogItem
	if err := json.Unmarshal(data, &items); err != nil {
		panic("invalid bundled TTS voice catalog")
	}
	voices := make([]TTSVoiceSpec, 0, len(items))
	for _, item := range items {
		item.ProviderKind = ProviderAliyun
		item.Models = []string{item.TargetModel}
		voices = append(voices, item.TTSVoiceSpec)
	}
	return voices
}

var ttsBasicVoices = loadAliyunVoiceCatalog(ttsBasicCatalogJSON)

// Source: https://help.aliyun.com/zh/model-studio/qwen-audio-tts-voice-list
// System catalog version 2026-09-30. IDs are model-specific.
func aliyunSystemVoices() []TTSVoiceSpec {
	items := append([]TTSVoiceSpec{}, ttsBasicVoices...)
	for _, v := range [][3]string{
		{"longanlingxin", "龙安灵心", "plus"}, {"longanlufeng", "龙安鲁风", "plus"},
		{"longanfengyue", "龙安风悦", "flash"}, {"longanyuanfei", "龙安元妃", "flash"}, {"longanlingxi", "龙安灵希", "flash"}, {"longanxiaoxin", "龙安小昕", "flash"}, {"longanhuan_v3.6", "龙安欢", "flash"}, {"longjielidou_v3.6", "龙杰力豆", "flash"}, {"longpaopao_v3.6", "龙泡泡", "flash"}, {"longhuohuo_v3.6", "龙火火", "flash"}, {"longchuanshu_v3.6", "龙川叔", "flash"}, {"loongmary", "loongmary", "flash"}, {"loongeva_v3.6", "loongeva", "flash"}, {"loongjohn", "loongJohn", "flash"},
	} {
		languages := []string{"zh", "en"}
		if len(v[0]) >= 5 && v[0][:5] == "loong" {
			languages = []string{"en"}
		}
		items = append(items, TTSVoiceSpec{ID: v[0], Name: v[1], ProviderKind: ProviderAliyun, Models: []string{"qwen-audio-3.0-tts-" + v[2]}, Languages: languages, Kind: "system"})
	}
	items = append(items, tts31SystemVoices()...)
	for i := range items {
		if items[i].Kind == "basic" && len(items[i].Models) == 1 {
			if spec, ok := LookupModel(items[i].ProviderKind, items[i].Models[0]); ok && spec.Capabilities.VoiceClone {
				// Base voices are cloned voices. Keep catalog Languages for filtering,
				// but allow SealChat translation targets supported by the model's clone path.
				items[i].SpeechLanguages = append([]string(nil), spec.CloneLanguages...)
			}
		}
		if !items[i].Supports(ProviderAliyun, "qwen-audio-3.1-tts-flash") {
			continue
		}
		switch items[i].ID {
		case "longanhuan_v3.1", "longanlingxin_v3.1", "longanfengyue_v3.1", "xunanchuan_v3.1":
			// SealChat also permits English translation, without changing official metadata.
			items[i].SpeechLanguages = []string{"zh", "en", "ja", "ko", "fr", "de", "pt", "it", "vi", "id"}
		}
	}
	return items
}

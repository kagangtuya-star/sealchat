package service

import (
	_ "embed"
	"encoding/json"
)

// Generated offline from the official 2026-07-23 Flash and Plus XLSX catalogs
// linked by the source page below. Voice IDs are preserved verbatim.
//
//go:embed tts_catalog_basic_20260723.json
var ttsBasicCatalogJSON []byte

var ttsBasicVoices = func() []TTSSystemVoice {
	var items []TTSSystemVoice
	if err := json.Unmarshal(ttsBasicCatalogJSON, &items); err != nil {
		panic("invalid bundled TTS voice catalog")
	}
	return items
}()

// Source: https://help.aliyun.com/zh/model-studio/qwen-audio-tts-voice-list
// System catalog version 2026-09-12. IDs are model-specific.
type TTSSystemVoice struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	TargetModel string   `json:"targetModel"`
	Languages   []string `json:"languages"`
	Kind        string   `json:"kind"`
	Tags        string   `json:"tags,omitempty"`
}

func TTSSystemVoices() []TTSSystemVoice {
	items := append([]TTSSystemVoice{}, ttsBasicVoices...)
	for _, v := range [][3]string{
		{"longanlingxin", "龙安灵心", "plus"}, {"longanlufeng", "龙安鲁风", "plus"},
		{"longanfengyue", "龙安风悦", "flash"}, {"longanyuanfei", "龙安元妃", "flash"}, {"longanlingxi", "龙安灵希", "flash"}, {"longanxiaoxin", "龙安小昕", "flash"}, {"longanhuan_v3.6", "龙安欢", "flash"}, {"longjielidou_v3.6", "龙杰力豆", "flash"}, {"longpaopao_v3.6", "龙泡泡", "flash"}, {"longhuohuo_v3.6", "龙火火", "flash"}, {"longchuanshu_v3.6", "龙川叔", "flash"}, {"loongmary", "loongmary", "flash"}, {"loongeva_v3.6", "loongeva", "flash"}, {"loongjohn", "loongJohn", "flash"},
	} {
		languages := []string{"zh", "en"}
		if len(v[0]) >= 5 && v[0][:5] == "loong" {
			languages = []string{"en"}
		}
		items = append(items, TTSSystemVoice{ID: v[0], Name: v[1], TargetModel: "qwen-audio-3.0-tts-" + v[2], Languages: languages, Kind: "system"})
	}
	return items
}
func ttsSystemVoiceValid(id, model string) bool {
	for _, v := range TTSSystemVoices() {
		if v.ID == id && v.TargetModel == model {
			return true
		}
	}
	return false
}

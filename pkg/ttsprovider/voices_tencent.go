package ttsprovider

import (
	_ "embed"
	"encoding/json"
)

// Source: https://cloud.tencent.com/document/product/1073/92668
// Only the confirmed TextToVoice classic/large voices are bundled. Languages
// follow the basic synthesis table, not the realtime table on the same page.
//
//go:embed tts_catalog_tencent_20260929.json
var tencentCatalogJSON []byte

func tencentSystemVoices() []TTSVoiceSpec {
	var voices []TTSVoiceSpec
	if err := json.Unmarshal(tencentCatalogJSON, &voices); err != nil {
		panic("invalid bundled Tencent TTS voice catalog")
	}
	return voices
}

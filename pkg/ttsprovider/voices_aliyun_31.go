package ttsprovider

import _ "embed"

// Generated from the official Qwen-Audio-TTS system voice table.
// Voice IDs are preserved verbatim.
//
//go:embed tts_catalog_31_20260930.json
var tts31CatalogJSON []byte

func tts31SystemVoices() []TTSVoiceSpec {
	return loadAliyunVoiceCatalog(tts31CatalogJSON)
}

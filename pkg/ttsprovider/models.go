package ttsprovider

const (
	PricingCharacter = "character"
	PricingToken     = "token"
)

type TTSModelPricing struct {
	Mode             string   `json:"pricingMode"`
	CharacterPrice   *float64 `json:"characterPrice"`
	InputTokenPrice  *float64 `json:"inputTokenPrice"`
	OutputTokenPrice *float64 `json:"outputTokenPrice"`
	DisplayPrice     string   `json:"displayPrice"`
}

type ModelCapabilities struct {
	HTTPStreaming      bool `json:"httpStreaming"`
	WebSocketStreaming bool `json:"webSocketStreaming"`
	VoiceDesign        bool `json:"voiceDesign"`
	VoiceClone         bool `json:"voiceClone"`
}

type ModelSpec struct {
	ID                      string
	ProviderKind            string
	Pricing                 TTSModelPricing
	Capabilities            ModelCapabilities
	DefaultVoice            string
	CloneLanguages          []string
	SupportsClonePreprocess bool
}

func ModelCatalog() []ModelSpec {
	flash, plus, input, output := 0.0001, 0.00014, 0.0000015, 0.000012
	capabilities := ModelCapabilities{HTTPStreaming: true, WebSocketStreaming: true, VoiceDesign: true, VoiceClone: true}
	models := []ModelSpec{
		{ID: "qwen-audio-3.0-tts-flash", ProviderKind: ProviderAliyun, Capabilities: capabilities, DefaultVoice: "longanhuan_v3.6", Pricing: TTSModelPricing{Mode: PricingCharacter, CharacterPrice: &flash, DisplayPrice: "1 元 / 万字符"}},
		{ID: "qwen-audio-3.0-tts-plus", ProviderKind: ProviderAliyun, Capabilities: capabilities, DefaultVoice: "longanlingxin", Pricing: TTSModelPricing{Mode: PricingCharacter, CharacterPrice: &plus, DisplayPrice: "1.4 元 / 万字符"}},
		{ID: "qwen-audio-3.1-tts-flash", ProviderKind: ProviderAliyun, Capabilities: capabilities, DefaultVoice: "longanhuan_v3.1", Pricing: TTSModelPricing{Mode: PricingToken, InputTokenPrice: &input, OutputTokenPrice: &output, DisplayPrice: "输入 1.5 元 / 百万 Token；输出 12 元 / 百万 Token"}},
	}
	// These registered Qwen-Audio clone APIs share the enrollment capabilities.
	for i := range models {
		models[i].CloneLanguages = []string{"zh", "en", "fr", "de", "ja", "ko", "ru", "pt", "th", "id", "vi", "it", "es", "ms", "fil", "ar"}
		models[i].SupportsClonePreprocess = true
	}
	return models
}

func VoiceCloneLanguageSupported(spec ModelSpec, language string) bool {
	if language == "" {
		return true
	}
	if !spec.Capabilities.VoiceClone {
		return false
	}
	for _, hint := range spec.CloneLanguages {
		if language == hint {
			return true
		}
	}
	return false
}

func LookupModel(providerKind, id string) (ModelSpec, bool) {
	for _, spec := range ModelCatalog() {
		if spec.ProviderKind == providerKind && spec.ID == id {
			return spec, true
		}
	}
	return ModelSpec{}, false
}

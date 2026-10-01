package ttsprovider

import "strings"

const (
	PricingCharacter  = "character"
	PricingToken      = "token"
	RuntimeAliyunQwen = "aliyun-qwen"
	RuntimeTencentTTS = "tencent-tts"
	RuntimeTencentMPS = "tencent-mps"
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
	Name                    string
	ProviderKind            string
	Runtime                 string
	Pricing                 TTSModelPricing
	Capabilities            ModelCapabilities
	DefaultVoice            string
	CloneLanguages          []string
	SupportsClonePreprocess bool
}

func ModelCatalog() []ModelSpec {
	flash, plus, input, output := 0.0001, 0.00014, 0.0000015, 0.000012
	classic, large := 0.00003, 0.00012
	capabilities := ModelCapabilities{HTTPStreaming: true, WebSocketStreaming: true, VoiceDesign: true, VoiceClone: true}
	models := []ModelSpec{
		{ID: "qwen-audio-3.0-tts-flash", ProviderKind: ProviderAliyun, Capabilities: capabilities, DefaultVoice: "longanhuan_v3.6", Pricing: TTSModelPricing{Mode: PricingCharacter, CharacterPrice: &flash, DisplayPrice: "1 元 / 万字符"}},
		{ID: "qwen-audio-3.0-tts-plus", ProviderKind: ProviderAliyun, Capabilities: capabilities, DefaultVoice: "longanlingxin", Pricing: TTSModelPricing{Mode: PricingCharacter, CharacterPrice: &plus, DisplayPrice: "1.4 元 / 万字符"}},
		{ID: "qwen-audio-3.1-tts-flash", ProviderKind: ProviderAliyun, Capabilities: capabilities, DefaultVoice: "longanhuan_v3.1", Pricing: TTSModelPricing{Mode: PricingToken, InputTokenPrice: &input, OutputTokenPrice: &output, DisplayPrice: "输入 1.5 元 / 百万 Token；输出 12 元 / 百万 Token"}},
	}
	// These registered Qwen-Audio clone APIs share the enrollment capabilities.
	for i := range models {
		models[i].Runtime = RuntimeAliyunQwen
		models[i].CloneLanguages = []string{"zh", "en", "fr", "de", "ja", "ko", "ru", "pt", "th", "id", "vi", "it", "es", "ms", "fil", "ar"}
		models[i].SupportsClonePreprocess = true
	}
	tencentCapabilities := ModelCapabilities{HTTPStreaming: true}
	models = append(models,
		ModelSpec{ID: "tencent-tts-classic", Name: "腾讯云 TTS 精品音色", ProviderKind: ProviderTencent, Runtime: RuntimeTencentTTS, Capabilities: tencentCapabilities, DefaultVoice: "101004", Pricing: TTSModelPricing{Mode: PricingCharacter, CharacterPrice: &classic, DisplayPrice: "0.3 元 / 万字符"}},
		ModelSpec{ID: "tencent-tts-large", Name: "腾讯云 TTS 大模型音色", ProviderKind: ProviderTencent, Runtime: RuntimeTencentTTS, Capabilities: tencentCapabilities, DefaultVoice: "501004", Pricing: TTSModelPricing{Mode: PricingCharacter, CharacterPrice: &large, DisplayPrice: "1.2 元 / 万字符（后付费首档估算，可在 Provider 中调整）"}},
	)
	for _, id := range TencentMPSModels() {
		price, display := 0.00035, "0.175 元 / 500 计费字符"
		if strings.HasSuffix(id, "-turbo") {
			price, display = 0.0002, "0.1 元 / 500 计费字符"
		}
		models = append(models, ModelSpec{ID: id, Name: "腾讯 MPS MiniMax " + id, ProviderKind: ProviderTencent, Runtime: RuntimeTencentMPS, Capabilities: tencentCapabilities, Pricing: TTSModelPricing{Mode: PricingCharacter, CharacterPrice: &price, DisplayPrice: display}})
	}
	return models
}

func TencentMPSModels() []string {
	return []string{"speech-2.8-hd", "speech-2.8-turbo", "speech-2.6-hd", "speech-2.6-turbo", "speech-02-hd", "speech-02-turbo"}
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

// SupportsLivePCM reports whether the model runtime delivers incremental PCM.
func SupportsLivePCM(providerKind, model string) bool {
	spec, ok := LookupModel(providerKind, model)
	return ok && spec.Runtime == RuntimeAliyunQwen
}

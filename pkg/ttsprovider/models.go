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

type ModelSpec struct {
	ID            string
	Pricing       TTSModelPricing
	HTTPStreaming bool
}

func ModelCatalog() []ModelSpec {
	flash, plus, input, output := 0.0001, 0.00014, 0.0000015, 0.000012
	return []ModelSpec{
		{ID: "qwen-audio-3.0-tts-flash", HTTPStreaming: true, Pricing: TTSModelPricing{Mode: PricingCharacter, CharacterPrice: &flash, DisplayPrice: "1 元 / 万字符"}},
		{ID: "qwen-audio-3.0-tts-plus", HTTPStreaming: true, Pricing: TTSModelPricing{Mode: PricingCharacter, CharacterPrice: &plus, DisplayPrice: "1.4 元 / 万字符"}},
		{ID: "qwen-audio-3.1-tts-flash", HTTPStreaming: true, Pricing: TTSModelPricing{Mode: PricingToken, InputTokenPrice: &input, OutputTokenPrice: &output, DisplayPrice: "输入 1.5 元 / 百万 Token；输出 12 元 / 百万 Token"}},
	}
}

func LookupModel(id string) (ModelSpec, bool) {
	for _, spec := range ModelCatalog() {
		if spec.ID == id {
			return spec, true
		}
	}
	return ModelSpec{}, false
}

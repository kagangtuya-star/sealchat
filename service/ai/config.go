package ai

const (
	FeaturePolish        = "polish"
	FeatureBattleSummary = "battle_summary"
	FeatureTTSTranslate  = "tts_translate"
)

type FeatureDefinition struct {
	Key           string
	Label         string
	InputMaxChars int
}

func BuiltinFeatures() map[string]FeatureDefinition {
	return map[string]FeatureDefinition{
		FeatureTTSTranslate: {Key: FeatureTTSTranslate, Label: "语音翻译", InputMaxChars: 20000},
		FeaturePolish: {
			Key:           FeaturePolish,
			Label:         "润色",
			InputMaxChars: 4000,
		},
		FeatureBattleSummary: {
			Key:           FeatureBattleSummary,
			Label:         "战报总结",
			InputMaxChars: 12000,
		},
	}
}

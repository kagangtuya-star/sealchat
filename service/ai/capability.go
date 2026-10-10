package ai

import "sealchat/utils"

type FeatureCapability struct {
	Key            string              `json:"key"`
	Enabled        bool                `json:"enabled"`
	UserCustomOnly bool                `json:"userCustomOnly"`
	DefaultPrompt  string              `json:"defaultPrompt,omitempty"`
	DefaultModel   string              `json:"defaultModel,omitempty"`
	Params         utils.AIModelParams `json:"params,omitempty"`
}

func IsFeatureAvailable(cfg utils.AIConfig, featureKey string, userID string, worldID string) bool {
	cfg = utils.NormalizeAIConfig(cfg)
	if !cfg.Enabled {
		return false
	}
	feature, ok := cfg.Features[featureKey]
	if !ok || !feature.Enabled {
		return false
	}
	switch feature.Access.Mode {
	case utils.AIFeatureAccessUsers:
		return containsID(feature.Access.UserIDs, userID)
	case utils.AIFeatureAccessWorlds:
		return containsID(feature.Access.WorldIDs, worldID)
	case utils.AIFeatureAccessUsersOrWorlds:
		return containsID(feature.Access.UserIDs, userID) || containsID(feature.Access.WorldIDs, worldID)
	default:
		return true
	}
}

func AvailableFeatures(cfg utils.AIConfig, userID string, worldID string) []FeatureCapability {
	cfg = utils.NormalizeAIConfig(cfg)
	features := BuiltinFeatures()
	out := make([]FeatureCapability, 0, len(features))
	for featureKey := range features {
		if featureKey == FeatureTTSTranslate {
			continue // Backend platform workflow; never a personal AI option.
		}
		featureCfg, ok := cfg.Features[featureKey]
		if !ok {
			featureCfg = utils.AIFeatureConfig{}
		}
		out = append(out, FeatureCapability{
			Key:            featureKey,
			Enabled:        IsFeatureAvailable(cfg, featureKey, userID, worldID),
			UserCustomOnly: featureCfg.UserCustomOnly,
			DefaultPrompt:  featureCfg.DefaultPrompt,
			DefaultModel:   featureCfg.DefaultModel,
			Params:         featureCfg.Params,
		})
	}
	return out
}

func containsID(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

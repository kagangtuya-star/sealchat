package ai

import (
	"strings"

	"sealchat/model"
	"sealchat/utils"
)

func loadUserProviders(userID string) ([]utils.AIProviderConfig, error) {
	items, err := model.UserAIProviderProfileList(userID)
	if err != nil {
		return nil, err
	}
	out := make([]utils.AIProviderConfig, 0, len(items))
	for index, item := range items {
		if item == nil {
			continue
		}
		models := make([]string, 0, len(item.Models))
		for _, modelName := range item.Models {
			trimmed := strings.TrimSpace(modelName)
			if trimmed != "" {
				models = append(models, trimmed)
			}
		}
		out = append(out, utils.AIProviderConfig{
			ID:            strings.TrimSpace(item.ID),
			Name:          strings.TrimSpace(item.Name),
			Enabled:       item.Enabled,
			BaseURL:       strings.TrimSpace(item.BaseURL),
			APIKey:        item.APIKey,
			Models:        models,
			SelectedModel: strings.TrimSpace(item.SelectedModel),
			Weight:        index + 1,
		})
	}
	return out, nil
}

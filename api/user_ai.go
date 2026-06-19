package api

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
)

type userAIProviderProfilePayload struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	BaseURL       string   `json:"baseUrl"`
	APIKey        string   `json:"apiKey,omitempty"`
	Models        []string `json:"models"`
	SelectedModel string   `json:"selectedModel,omitempty"`
	Enabled       bool     `json:"enabled"`
	HasAPIKey     bool     `json:"hasApiKey,omitempty"`
}

func sanitizeUserAIProfilesForClient(items []*model.UserAIProviderProfileModel) []userAIProviderProfilePayload {
	out := make([]userAIProviderProfilePayload, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, userAIProviderProfilePayload{
			ID:            item.ID,
			Name:          item.Name,
			BaseURL:       item.BaseURL,
			APIKey:        "",
			Models:        append([]string(nil), item.Models...),
			SelectedModel: strings.TrimSpace(item.SelectedModel),
			Enabled:       item.Enabled,
			HasAPIKey:     strings.TrimSpace(item.APIKey) != "",
		})
	}
	return out
}

func mergeUserAIProfiles(userID string, current []*model.UserAIProviderProfileModel, incoming []userAIProviderProfilePayload) []*model.UserAIProviderProfileModel {
	currentKeyMap := make(map[string]string, len(current))
	for _, item := range current {
		if item == nil {
			continue
		}
		currentKeyMap[strings.TrimSpace(item.ID)] = item.APIKey
	}

	out := make([]*model.UserAIProviderProfileModel, 0, len(incoming))
	for _, item := range incoming {
		id := strings.TrimSpace(item.ID)
		apiKey := strings.TrimSpace(item.APIKey)
		if apiKey == "" && id != "" {
			apiKey = currentKeyMap[id]
		}
		out = append(out, &model.UserAIProviderProfileModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: id},
			UserID:            userID,
			Name:              strings.TrimSpace(item.Name),
			BaseURL:           strings.TrimSpace(item.BaseURL),
			APIKey:            apiKey,
			Models:            model.JSONList[string](normalizeUserAIModelList(item.Models)),
			SelectedModel:     strings.TrimSpace(item.SelectedModel),
			Enabled:           item.Enabled,
		})
	}
	return out
}

func normalizeUserAIModelList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func UserAIProfilesGet(ctx *fiber.Ctx) error {
	user := getCurUser(ctx)
	if user == nil {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	items, err := model.UserAIProviderProfileList(user.ID)
	if err != nil {
		return wrapError(ctx, err, "读取用户 AI 配置失败")
	}
	return ctx.JSON(fiber.Map{
		"items": sanitizeUserAIProfilesForClient(items),
	})
}

func UserAIProfilesUpsert(ctx *fiber.Ctx) error {
	user := getCurUser(ctx)
	if user == nil {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var body struct {
		Items []userAIProviderProfilePayload `json:"items"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	current, err := model.UserAIProviderProfileList(user.ID)
	if err != nil {
		return wrapError(ctx, err, "读取用户 AI 配置失败")
	}
	merged := mergeUserAIProfiles(user.ID, current, body.Items)
	items, err := model.UserAIProviderProfileUpsert(user.ID, merged)
	if err != nil {
		return wrapError(ctx, err, "保存用户 AI 配置失败")
	}
	return ctx.JSON(fiber.Map{
		"items": sanitizeUserAIProfilesForClient(items),
	})
}

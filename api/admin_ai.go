package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/pm"
	aiService "sealchat/service/ai"
	"sealchat/utils"
)

const (
	aiQuotaPolicySourceDefault  = "default"
	aiQuotaPolicySourceOverride = "override"
)

var adminAIHTTPClient = &http.Client{Timeout: 15 * time.Second}

func AdminAIConfigGet(ctx *fiber.Ctx) error {
	cfg := sanitizeConfigForAdmin(appConfig).AI
	return ctx.JSON(fiber.Map{
		"config": cfg,
	})
}

func AdminAIConfigUpdate(ctx *fiber.Ctx) error {
	var body struct {
		Config utils.AIConfig `json:"config"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return err
	}
	current := appConfig
	if current == nil {
		current = &utils.AppConfig{}
	}
	incoming := *current
	incoming.AI = body.Config
	merged := mergeConfigForWrite(current, &incoming)
	merged.AI = utils.NormalizeAIConfig(merged.AI)
	if err := utils.ValidateAIConfig(merged.AI); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	appConfig = merged
	utils.WriteConfig(appConfig)
	SyncConfigToDB(appConfig, "api")
	return ctx.JSON(fiber.Map{
		"config": sanitizeConfigForAdmin(appConfig).AI,
	})
}

func AdminAIProviderTest(ctx *fiber.Ctx) error {
	var body struct {
		ProviderID string `json:"providerId"`
		Model      string `json:"model"`
		Prompt     string `json:"prompt"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return err
	}
	if appConfig == nil {
		return ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"message": "AI 配置不可用"})
	}
	cfg := utils.NormalizeAIConfig(appConfig.AI)
	for _, provider := range cfg.Providers {
		if provider.ID != strings.TrimSpace(body.ProviderID) {
			continue
		}
		model := strings.TrimSpace(body.Model)
		if model == "" && len(provider.Models) > 0 {
			model = provider.Models[0]
		}
		client := aiService.NewRunner(func() *utils.AppConfig {
			return &utils.AppConfig{AI: utils.AIConfig{
				Enabled:   true,
				Providers: []utils.AIProviderConfig{provider},
				Features: map[string]utils.AIFeatureConfig{
					aiService.FeaturePolish: {
						Enabled:       true,
						DefaultPrompt: "你是连通性测试助手。按原样返回用户输入。",
						DefaultModel:  model,
						Access: utils.AIFeatureAccessConfig{
							Mode: utils.AIFeatureAccessAll,
						},
					},
				},
			}}
		}, nil)
		result, err := client.Run(context.Background(), aiService.RunRequest{
			FeatureKey: aiService.FeaturePolish,
			UserID:     "admin-test",
			WorldID:    "",
			Input:      strings.TrimSpace(body.Prompt),
			Source:     "platform",
		})
		if err != nil {
			return ctx.Status(fiber.StatusBadGateway).JSON(fiber.Map{"message": err.Error()})
		}
		return ctx.JSON(fiber.Map{
			"providerId": result.ProviderID,
			"model":      result.Model,
			"result":     result.Result,
		})
	}
	return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "AI provider 不存在"})
}

func discoverAIProviderModels(ctx context.Context, provider utils.AIProviderConfig) ([]string, error) {
	baseURL := strings.TrimSpace(provider.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("AI provider baseUrl 不能为空")
	}
	apiKey := strings.TrimSpace(provider.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("AI provider apiKey 不能为空")
	}
	target, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("AI provider baseUrl 非法")
	}
	target.Path = strings.TrimRight(target.Path, "/") + "/models"
	target.RawQuery = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := adminAIHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return nil, errors.New(strings.TrimSpace(payload.Error.Message))
		}
		if strings.TrimSpace(payload.Message) != "" {
			return nil, errors.New(strings.TrimSpace(payload.Message))
		}
		return nil, fmt.Errorf("AI provider models 请求失败(%d)", resp.StatusCode)
	}
	models := make([]string, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	return models, nil
}

func AdminAIProviderModelsDiscover(ctx *fiber.Ctx) error {
	var body struct {
		ProviderID string `json:"providerId"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return err
	}
	if appConfig == nil {
		return ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"message": "AI 配置不可用"})
	}
	providerID := strings.TrimSpace(body.ProviderID)
	cfg := utils.NormalizeAIConfig(appConfig.AI)
	for _, provider := range cfg.Providers {
		if provider.ID != providerID {
			continue
		}
		models, err := discoverAIProviderModels(ctx.Context(), provider)
		if err != nil {
			return ctx.Status(fiber.StatusBadGateway).JSON(fiber.Map{"message": err.Error()})
		}
		return ctx.JSON(fiber.Map{
			"providerId": provider.ID,
			"models":     models,
		})
	}
	return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "AI provider 不存在"})
}

func AdminAIUsageLogs(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	result, err := aiService.AdminListUsageLogs(aiService.AdminUsageLogQuery{
		Page:       c.QueryInt("page", 1),
		PageSize:   c.QueryInt("pageSize", 20),
		Query:      c.Query("query"),
		FeatureKey: c.Query("featureKey"),
		ProviderID: c.Query("providerId"),
		Model:      c.Query("model"),
		Status:     c.Query("status"),
		StartMS:    int64(c.QueryInt("start", 0)),
		EndMS:      int64(c.QueryInt("end", 0)),
	})
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取 AI 调用日志失败")
	}
	return c.JSON(result)
}

func AdminAIUsageLogsCleanup(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	var body struct {
		RetentionDays int `json:"retentionDays"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&body); err != nil {
			return wrapErrorStatus(c, fiber.StatusBadRequest, err, "AI 日志清理请求解析失败")
		}
	}
	affectedRows, err := aiService.AdminCleanupUsageLogs(body.RetentionDays, time.Now())
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "清理 AI 调用日志失败")
	}
	return c.JSON(fiber.Map{"affectedRows": affectedRows})
}

func AdminAIQuotaList(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	result, err := aiService.AdminListQuotaOverrides(
		c.QueryInt("page", 1),
		c.QueryInt("pageSize", 20),
		c.Query("query"),
		time.Now(),
	)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取 AI 用户配额失败")
	}
	return c.JSON(result)
}

func AdminAIQuotaGet(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	item, err := aiService.AdminGetQuotaDetail(c.Params("userId"), time.Now())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "用户不存在")
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取 AI 用户配额失败")
	}
	return c.JSON(item)
}

func AdminAIQuotaUpsert(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	var req utils.AIQuotaPolicyConfig
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "AI 用户配额请求解析失败")
	}
	user := getCurUser(c)
	record, err := aiService.AdminUpsertQuotaOverride(c.Params("userId"), user.ID, req)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "用户不存在")
		default:
			return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
		}
	}
	item, err := aiService.AdminGetQuotaDetail(record.UserID, time.Now())
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取 AI 用户配额失败")
	}
	return c.JSON(item)
}

func AdminAIQuotaDelete(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	if err := aiService.AdminDeleteQuotaOverride(c.Params("userId")); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	return c.JSON(fiber.Map{"message": "AI 用户配额覆盖已删除"})
}

package api

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	aiService "sealchat/service/ai"
	"sealchat/utils"
)

type aiTaskRunner interface {
	Run(ctx context.Context, req aiService.RunRequest) (aiService.RunResult, error)
}

var aiRunnerFactory = func(cfgProvider func() *utils.AppConfig) aiTaskRunner {
	return aiService.NewRunner(cfgProvider, nil)
}

func AICapabilitiesGet(ctx *fiber.Ctx) error {
	user := getCurUser(ctx)
	if user == nil {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	if appConfig == nil {
		return ctx.JSON(fiber.Map{"features": []aiService.FeatureCapability{}})
	}
	worldID := strings.TrimSpace(ctx.Query("worldId"))
	features := aiService.AvailableFeatures(appConfig.AI, user.ID, worldID)
	return ctx.JSON(fiber.Map{
		"features": features,
	})
}

func AITaskRun(ctx *fiber.Ctx) error {
	user := getCurUser(ctx)
	if user == nil {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var body struct {
		WorldID   string `json:"worldId"`
		ChannelID string `json:"channelId"`
		Input     string `json:"input"`
		Source    string `json:"source"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return err
	}
	featureKey := strings.TrimSpace(ctx.Params("featureKey"))
	source := strings.TrimSpace(body.Source)
	runner := aiRunnerFactory(func() *utils.AppConfig { return appConfig })
	cfg := utils.AIConfig{}
	if appConfig != nil {
		cfg = appConfig.AI
	}
	output, err := aiService.RunTaskWithBilling(ctx.Context(), aiService.BilledRunInput{
		Config:     cfg,
		User:       user,
		FeatureKey: featureKey,
		WorldID:    body.WorldID,
		Input:      body.Input,
		Source:     source,
		Runner:     runner,
	})
	if err != nil {
		if errors.Is(err, aiService.ErrInputTooLong) {
			maxInputChars := 0
			if appConfig != nil {
				maxInputChars = utils.NormalizeAIConfig(appConfig.AI).Features[featureKey].Params.MaxInputChars
			}
			err = aiService.FormatInputTooLongError(featureKey, len([]rune(strings.TrimSpace(body.Input))), maxInputChars)
		}
		status := fiber.StatusBadRequest
		switch err.(type) {
		case *aiService.AIQuotaExceededError:
			status = fiber.StatusForbidden
		default:
			if errors.Is(err, aiService.ErrUserCustomProviderRequired) {
				status = fiber.StatusForbidden
			} else if strings.Contains(err.Error(), "no ai provider available") {
				status = fiber.StatusServiceUnavailable
			} else if strings.Contains(err.Error(), "unavailable") {
				status = fiber.StatusForbidden
			} else if strings.Contains(err.Error(), "pricing") {
				status = fiber.StatusServiceUnavailable
			} else if strings.Contains(err.Error(), "ai usage unavailable") {
				status = fiber.StatusBadGateway
			} else if strings.Contains(err.Error(), "quota reservation missing") {
				status = fiber.StatusInternalServerError
			}
		}
		if strings.Contains(err.Error(), "no ai provider available") {
			status = fiber.StatusServiceUnavailable
		}
		return ctx.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	result := output.Result
	response := fiber.Map{
		"featureKey": result.FeatureKey,
		"result":     result.Result,
		"model":      result.Model,
		"providerId": result.ProviderID,
	}
	if warning := strings.TrimSpace(output.Warning); warning != "" {
		response["warning"] = warning
	}
	return ctx.JSON(response)
}

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
	aiService "sealchat/service/ai"
	"sealchat/utils"
)

type aiTaskRunner interface {
	Run(ctx context.Context, req aiService.RunRequest) (aiService.RunResult, error)
}

type aiTaskRequest struct {
	WorldID        string           `json:"worldId"`
	ChannelID      string           `json:"channelId"`
	Input          string           `json:"input"`
	Source         string           `json:"source"`
	ProjectID      string           `json:"projectId"`
	SourceVersions map[string]int64 `json:"sourceVersions"`
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
	if projectID := strings.TrimSpace(ctx.Query("projectId")); projectID != "" {
		var err error
		worldID, err = service.LogProofreadWorld(projectID, user.ID)
		if err != nil {
			return writeAIContextError(ctx, err)
		}
	} else if worldID != "" && !service.IsWorldMember(worldID, user.ID) {
		worldID = ""
	}
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
	var body aiTaskRequest
	if err := ctx.BodyParser(&body); err != nil {
		return err
	}
	featureKey := strings.TrimSpace(ctx.Params("featureKey"))
	if featureKey == aiService.FeatureTTSTranslate {
		return ctx.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "语音翻译仅由后端朗读流程调用"})
	}
	if featureKey == aiService.FeatureLogProofread {
		validateOnly := ctx.Query("validate") == "1"
		var worldID string
		var err error
		if validateOnly {
			worldID, err = service.ValidateLogProofreadReferences(body.ProjectID, user.ID, body.Input, body.SourceVersions)
		} else {
			worldID, err = service.ValidateLogProofreadInput(body.ProjectID, user.ID, body.Input, body.SourceVersions)
		}
		if err != nil {
			return writeAIContextError(ctx, err)
		}
		body.WorldID = worldID
		if appConfig == nil || !aiService.IsFeatureAvailable(appConfig.AI, featureKey, user.ID, worldID) {
			return ctx.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "LOG AI 校对未启用或无权使用"})
		}
		if body.Source != "platform" && body.Source != "user" {
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请选择平台 AI 或个人 API"})
		}
		if appConfig.AI.Features[featureKey].UserCustomOnly && body.Source != "user" {
			return writeAITaskHTTPError(ctx, aiService.FormatUserCustomProviderRequiredError(featureKey), featureKey, body.Input, appConfig.AI)
		}
		limit := utils.NormalizeAIConfig(appConfig.AI).Features[featureKey].Params.MaxInputChars
		if len([]rune(body.Input)) > limit {
			return writeAITaskHTTPError(ctx, aiService.ErrInputTooLong, featureKey, body.Input, appConfig.AI)
		}
		if validateOnly {
			return ctx.JSON(fiber.Map{"valid": true})
		}
	} else if body.WorldID != "" && !service.IsWorldMember(body.WorldID, user.ID) {
		body.WorldID = ""
	}
	runner := aiRunnerFactory(func() *utils.AppConfig { return appConfig })
	cfg := utils.AIConfig{}
	if appConfig != nil {
		cfg = appConfig.AI
	}
	if ctx.Query("stream") == "1" {
		return streamAITask(ctx, user, featureKey, body, cfg, runner)
	}
	output, err := executeAITask(ctx.Context(), user, featureKey, body, cfg, runner)
	if err != nil {
		return writeAITaskHTTPError(ctx, err, featureKey, body.Input, cfg)
	}
	return ctx.JSON(aiTaskResponse(output))
}

func writeAIContextError(ctx *fiber.Ctx, err error) error {
	var domain *service.LogError
	if errors.As(err, &domain) {
		status := fiber.StatusForbidden
		if domain.Code == "INVALID_INPUT" {
			status = fiber.StatusBadRequest
		}
		if domain.Code == "SOURCE_CHANGED" {
			status = fiber.StatusConflict
		}
		if domain.Code == "NOT_FOUND" {
			status = fiber.StatusNotFound
		}
		return ctx.Status(status).JSON(fiber.Map{"message": domain.Message, "code": domain.Code})
	}
	return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "校验 AI 工程权限失败"})
}

func executeAITask(ctx context.Context, user *model.UserModel, featureKey string, body aiTaskRequest, cfg utils.AIConfig, runner aiTaskRunner) (aiService.BilledRunOutput, error) {
	return aiService.RunTaskWithBilling(ctx, aiService.BilledRunInput{
		Config:     cfg,
		User:       user,
		FeatureKey: featureKey,
		WorldID:    body.WorldID,
		Input:      body.Input,
		Source:     strings.TrimSpace(body.Source),
		Runner:     runner,
	})
}

func aiTaskResponse(output aiService.BilledRunOutput) fiber.Map {
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
	return response
}

func normalizeAITaskError(err error, featureKey string, input string, cfg utils.AIConfig) error {
	if !errors.Is(err, aiService.ErrInputTooLong) {
		return err
	}
	maxInputChars := utils.NormalizeAIConfig(cfg).Features[featureKey].Params.MaxInputChars
	return aiService.FormatInputTooLongError(featureKey, len([]rune(strings.TrimSpace(input))), maxInputChars)
}

func aiTaskErrorStatus(err error) int {
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
	return status
}

func writeAITaskHTTPError(ctx *fiber.Ctx, err error, featureKey string, input string, cfg utils.AIConfig) error {
	err = normalizeAITaskError(err, featureKey, input, cfg)
	return ctx.Status(aiTaskErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
}

func streamAITask(ctx *fiber.Ctx, user *model.UserModel, featureKey string, body aiTaskRequest, cfg utils.AIConfig, runner aiTaskRunner) error {
	streamCtx, cancel := context.WithCancel(context.Background())
	ctx.Set(fiber.HeaderContentType, "text/event-stream; charset=utf-8")
	ctx.Set(fiber.HeaderCacheControl, "no-cache, no-transform")
	ctx.Set("X-Accel-Buffering", "no")
	ctx.Context().SetBodyStreamWriter(func(writer *bufio.Writer) {
		defer cancel()
		resultCh := make(chan struct {
			output aiService.BilledRunOutput
			err    error
		}, 1)
		go func() {
			runCtx := streamCtx
			if featureKey == aiService.FeatureLogProofread {
				// Disconnect stops delivery, not accounting for a provider run already in flight.
				// Runner deadlines still bound the request; successful usage is settled normally.
				runCtx = context.WithoutCancel(streamCtx)
			}
			output, err := executeAITask(runCtx, user, featureKey, body, cfg, runner)
			resultCh <- struct {
				output aiService.BilledRunOutput
				err    error
			}{output: output, err: err}
		}()
		if !writeAITaskComment(writer, "ready") {
			return
		}
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case result := <-resultCh:
				if result.err != nil {
					err := normalizeAITaskError(result.err, featureKey, body.Input, cfg)
					_ = writeAITaskSSE(writer, "error", fiber.Map{"message": err.Error()})
					return
				}
				_ = writeAITaskSSE(writer, "result", aiTaskResponse(result.output))
				return
			case <-ticker.C:
				if !writeAITaskComment(writer, "ping") {
					return
				}
			}
		}
	})
	return nil
}

func writeAITaskComment(writer *bufio.Writer, value string) bool {
	if _, err := fmt.Fprintf(writer, ": %s\n\n", value); err != nil {
		return false
	}
	return writer.Flush() == nil
}

func writeAITaskSSE(writer *bufio.Writer, event string, payload any) bool {
	data, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return false
	}
	return writer.Flush() == nil
}

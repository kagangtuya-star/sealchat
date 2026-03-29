package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
)

func getAuthorizationToken(c *fiber.Ctx) string {
	token := strings.TrimSpace(c.Get("Authorization"))
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}
	if token == "" {
		return ""
	}
	lower := strings.ToLower(token)
	if strings.HasPrefix(lower, "bearer ") {
		return strings.TrimSpace(token[len("bearer "):])
	}
	return token
}

func getWebhookIntegration(c *fiber.Ctx) *model.ChannelWebhookIntegrationModel {
	v := c.Locals("webhookIntegration")
	if v == nil {
		return nil
	}
	if i, ok := v.(*model.ChannelWebhookIntegrationModel); ok {
		return i
	}
	return nil
}

func requireWebhookCapability(c *fiber.Ctx, cap string) (*model.ChannelWebhookIntegrationModel, error) {
	integration := getWebhookIntegration(c)
	if integration == nil {
		return nil, c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "missing integration context",
		})
	}
	if !integration.HasCapability(cap) {
		return nil, c.Status(http.StatusForbidden).JSON(fiber.Map{
			"error":   "forbidden",
			"message": "capability required: " + cap,
		})
	}
	return integration, nil
}

func WebhookAuthMiddleware(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error":   "bad_request",
			"message": "missing channelId",
		})
	}

	token := getAuthorizationToken(c)
	if len(token) != 32 {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "token invalid",
		})
	}

	user, err := model.BotVerifyAccessToken(token)
	if err != nil || user == nil {
		msg := "token invalid"
		if err != nil && strings.TrimSpace(err.Error()) != "" {
			msg = err.Error()
		}
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": msg,
		})
	}

	integration, err := model.ChannelWebhookIntegrationGetByChannelAndBot(channelID, user.ID)
	if err != nil || integration == nil {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{
			"error":   "forbidden",
			"message": "integration not found or revoked",
		})
	}

	now := time.Now()
	_ = model.ChannelWebhookIntegrationTouchUsage(channelID, user.ID, now)
	_ = model.GetDB().Model(&model.BotTokenModel{}).
		Where("id = ?", user.ID).
		Update("recent_used_at", now.UnixMilli()).Error

	c.Locals("user", user)
	c.Locals("webhookIntegration", integration)
	c.Locals("webhookToken", token)
	return c.Next()
}

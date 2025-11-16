package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

// ChannelConfigSync 同步频道配置（角色等）
func ChannelConfigSync(c *fiber.Ctx) error {
	user := getCurUser(c)
	var payload struct {
		SourceChannelID  string   `json:"sourceChannelId"`
		TargetChannelIDs []string `json:"targetChannelIds"`
		Scopes           []string `json:"scopes"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	scopes := make([]service.ChannelConfigSyncScope, 0, len(payload.Scopes))
	for _, s := range payload.Scopes {
		scopes = append(scopes, service.ChannelConfigSyncScope(s))
	}
	result, err := service.ChannelConfigSync(&service.ChannelConfigSyncOptions{
		UserID:           user.ID,
		SourceChannelID:  payload.SourceChannelID,
		TargetChannelIDs: payload.TargetChannelIDs,
		Scopes:           scopes,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

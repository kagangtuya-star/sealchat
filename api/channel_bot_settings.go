package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/pm"
	"sealchat/service"
)

func ChannelBotSettingsGet(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	if !pm.CanWithChannelRole(user.ID, channelID, pm.PermFuncChannelManageInfo, pm.PermFuncChannelRoleLink) &&
		!pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return fiber.ErrForbidden
	}
	settings, err := service.GetChannelBotSettings(c.Context(), channelID)
	if err != nil {
		return err
	}
	return c.JSON(settings)
}

func ChannelBotSettingsUpdate(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	if !pm.CanWithChannelRole(user.ID, channelID, pm.PermFuncChannelManageInfo, pm.PermFuncChannelRoleLink) &&
		!pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return fiber.ErrForbidden
	}
	var payload service.ChannelBotSettingsUpdate
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "请求参数错误",
		})
	}
	settings, err := service.SaveChannelBotSettings(c.Context(), channelID, user.ID, &payload)
	if err != nil {
		return err
	}
	return c.JSON(settings)
}

package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func WorldCharacterCardTemplateShareHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := strings.TrimSpace(c.Params("worldId"))
	templateID := strings.TrimSpace(c.Params("templateId"))
	if worldID == "" || templateID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	if err := service.WorldCharacterCardTemplateShare(worldID, templateID, user.ID); err != nil {
		switch {
		case err == service.ErrWorldPermission:
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权共享模板"})
		default:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	}
	return c.JSON(fiber.Map{"success": true})
}

func WorldCharacterCardTemplateUnshareHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := strings.TrimSpace(c.Params("worldId"))
	templateID := strings.TrimSpace(c.Params("templateId"))
	if worldID == "" || templateID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	if err := service.WorldCharacterCardTemplateUnshare(worldID, templateID, user.ID); err != nil {
		switch {
		case err == service.ErrWorldPermission:
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权取消共享模板"})
		default:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	}
	return c.JSON(fiber.Map{"success": true})
}

package api

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func parseEffectiveWorldKeywordListOptions(c *fiber.Ctx) service.EffectiveWorldKeywordListOptions {
	return service.EffectiveWorldKeywordListOptions{
		Query:             strings.TrimSpace(c.Query("q")),
		Category:          strings.TrimSpace(c.Query("category")),
		IncludeDisabled:   c.QueryBool("includeDisabled"),
		IncludeAllMatches: c.QueryBool("includeAllMatches"),
	}
}

func EffectiveWorldKeywordListHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	items, err := service.EffectiveWorldKeywordList(worldID, user.ID, parseEffectiveWorldKeywordListOptions(c))
	if err != nil {
		status := fiber.StatusInternalServerError
		switch err {
		case service.ErrWorldPermission:
			status = fiber.StatusForbidden
		case service.ErrWorldNotFound:
			status = fiber.StatusNotFound
		default:
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{
		"items": items,
		"total": len(items),
	})
}

func EffectiveWorldKeywordPublicListHandler(c *fiber.Ctx) error {
	worldID := c.Params("worldId")
	items, err := service.EffectiveWorldKeywordListPublic(worldID, parseEffectiveWorldKeywordListOptions(c))
	if err != nil {
		status := fiber.StatusInternalServerError
		switch err {
		case service.ErrWorldPermission:
			status = fiber.StatusForbidden
		case service.ErrWorldNotFound:
			status = fiber.StatusNotFound
		default:
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{
		"items": items,
		"total": len(items),
	})
}

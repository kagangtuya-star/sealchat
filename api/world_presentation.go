package api

import (
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"

	"sealchat/protocol"
	"sealchat/service"
)

func worldPresentationGet(c *fiber.Ctx) error {
	worldID := c.Params("worldId")
	user := getCurUser(c)
	if user == nil || !service.IsWorldMember(worldID, user.ID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权访问世界演出"})
	}
	settings, err := service.ReadWorldPresentation(worldID)
	if err != nil {
		return wrapError(c, err, "获取世界演出配置失败")
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(settings)
}

func worldPresentationPut(c *fiber.Ctx) error {
	worldID := c.Params("worldId")
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权编辑世界演出"})
	}
	var input struct {
		Config   json.RawMessage `json:"config"`
		Revision string          `json:"revision"`
	}
	if len(c.Body()) > service.WorldPresentationMaxBytes+1024 || c.BodyParser(&input) != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": service.ErrWorldPresentationInvalid.Error()})
	}
	settings, err := service.SaveWorldPresentation(worldID, user.ID, input.Revision, input.Config)
	switch {
	case errors.Is(err, service.ErrWorldPermission):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权编辑世界演出"})
	case errors.Is(err, service.ErrWorldPresentationInvalid):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	case errors.Is(err, service.ErrWorldPresentationConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": err.Error()})
	case err != nil:
		return wrapError(c, err, "保存世界演出配置失败")
	}
	broadcastEventToWorld(worldID, &protocol.Event{
		Type: protocol.EventWorldPresentationUpdated,
		Argv: &protocol.Argv{Options: map[string]any{"worldId": worldID, "revision": settings.Revision}},
	})
	return c.JSON(settings)
}

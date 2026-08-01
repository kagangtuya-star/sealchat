package api

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/protocol"
	"sealchat/service"
)

func WorldDice3DConfigGet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	if !service.IsWorldMember(worldID, user.ID) {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权访问世界"})
	}
	config, err := service.ResolveDice3DWorldConfig(worldID)
	if err != nil {
		return wrapError(c, err, "获取 3D 骰子配置失败")
	}
	return c.JSON(fiber.Map{"config": config})
}

func WorldDice3DConfigPut(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var body protocol.Dice3DWorldConfig
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	config, err := service.SaveDice3DWorldConfig(worldID, user.ID, body)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorldPermission):
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权编辑世界"})
		case errors.Is(err, service.ErrDice3DConfigInvalid):
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		default:
			return wrapError(c, err, "保存 3D 骰子配置失败")
		}
	}
	broadcastEventToWorld(worldID, &protocol.Event{
		Type: protocol.EventWorldDice3DUpdated,
		Argv: &protocol.Argv{Options: map[string]interface{}{"worldId": worldID, "config": config}},
	})
	return c.JSON(fiber.Map{"config": config})
}

func WorldDice3DProfileGet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	profile, revision, err := service.ResolveDice3DMemberProfile(c.Params("worldId"), user.ID)
	if err != nil {
		if errors.Is(err, service.ErrWorldPermission) {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权访问世界"})
		}
		return wrapError(c, err, "获取个人骰子配置失败")
	}
	return c.JSON(fiber.Map{"profile": profile, "revision": revision})
}

func WorldDice3DProfilePut(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var body protocol.Dice3DMemberProfile
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	profile, revision, err := service.SaveDice3DMemberProfile(worldID, user.ID, body)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorldPermission):
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权编辑个人骰子"})
		case errors.Is(err, service.ErrDice3DProfileInvalid):
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		default:
			return wrapError(c, err, "保存个人骰子配置失败")
		}
	}
	broadcastEventToWorld(worldID, &protocol.Event{
		Type: protocol.EventWorldMemberDice3DUpdated,
		Argv: &protocol.Argv{Options: map[string]interface{}{"worldId": worldID, "userId": user.ID, "revision": revision}},
	})
	return c.JSON(fiber.Map{"profile": profile, "revision": revision})
}

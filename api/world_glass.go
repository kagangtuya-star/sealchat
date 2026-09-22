package api

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/protocol"
	"sealchat/service"
)

func BindWorldGlassRoutes(group fiber.Router) {
	group.Get("/worlds/:worldId/glass-background", worldGlassEffectiveHandler)
	group.Get("/worlds/:worldId/glass-presets", worldGlassListHandler)
	group.Post("/worlds/:worldId/glass-presets", worldGlassPresetSaveHandler)
	group.Patch("/worlds/:worldId/glass-presets/:presetId", worldGlassPresetSaveHandler)
	group.Delete("/worlds/:worldId/glass-presets/:presetId", worldGlassMutationHandler("preset-delete"))
	group.Put("/worlds/:worldId/glass-presets/:presetId/channel-triggers", worldGlassMutationHandler("channel-triggers-replace"))
	group.Post("/worlds/:worldId/glass-background/activate", worldGlassMutationHandler("activate"))
	group.Post("/worlds/:worldId/glass-background/disable", worldGlassMutationHandler("disable"))
	group.Post("/worlds/:worldId/glass-triggers", worldGlassMutationHandler("trigger-save"))
	group.Patch("/worlds/:worldId/glass-triggers/:triggerId", worldGlassMutationHandler("trigger-save"))
	group.Delete("/worlds/:worldId/glass-triggers/:triggerId", worldGlassMutationHandler("trigger-delete"))
}

func worldGlassError(c *fiber.Ctx, err error) error {
	status, message := fiber.StatusInternalServerError, "世界玻璃背景操作失败"
	switch {
	case errors.Is(err, service.ErrWorldGlassDenied):
		status, message = fiber.StatusForbidden, "需要世界 owner/admin 权限或世界访问权限"
	case errors.Is(err, service.ErrWorldNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		status, message = fiber.StatusNotFound, "世界、预设或规则不存在"
	case errors.Is(err, service.ErrWorldGlassLimit):
		status, message = fiber.StatusConflict, "每个世界最多 20 个玻璃预设"
	case errors.Is(err, service.ErrWorldGlassInvalid):
		status, message = fiber.StatusBadRequest, "预设参数、正式图片附件或频道规则无效"
	}
	return c.Status(status).JSON(fiber.Map{"message": message})
}

func worldGlassEffectiveHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	worldID := c.Params("worldId")
	canManage, err := service.WorldGlassAccess(worldID, user.ID, false)
	if err != nil {
		return worldGlassError(c, err)
	}
	state, err := service.ResolveWorldGlassEffectivePreset(service.WorldGlassTriggerContext{WorldID: worldID, ChannelID: c.Query("channelId")})
	if err != nil {
		return worldGlassError(c, err)
	}
	state.CanManage = canManage
	c.Set("Cache-Control", "no-store")
	return c.JSON(state)
}

func worldGlassListHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	result, err := service.WorldGlassPresetList(c.Params("worldId"), user.ID)
	if err != nil {
		return worldGlassError(c, err)
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(result)
}

func worldGlassBroadcast(worldID string, revision uint64) {
	ctx := &ChatContext{UserId2ConnInfo: userId2ConnInfoGlobal}
	ctx.BroadcastEventInWorld(worldID, &protocol.Event{Type: protocol.EventWorldGlassBackgroundUpdated, WorldGlass: &protocol.WorldGlassEventPayload{WorldID: worldID, Revision: revision}})
}

func worldGlassPresetSaveHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	var body service.WorldGlassPresetInput
	if err := c.BodyParser(&body); err != nil {
		return worldGlassError(c, service.ErrWorldGlassInvalid)
	}
	preset, revision, err := service.WorldGlassPresetSave(c.Params("worldId"), user.ID, c.Params("presetId"), body)
	if err != nil {
		return worldGlassError(c, err)
	}
	worldGlassBroadcast(c.Params("worldId"), revision)
	return c.JSON(fiber.Map{"preset": preset, "revision": revision})
}

func worldGlassMutationHandler(action string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user := getCurUser(c)
		if user == nil {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		worldID := c.Params("worldId")
		var revision uint64
		var err error
		switch action {
		case "preset-delete":
			revision, err = service.WorldGlassPresetDelete(worldID, user.ID, c.Params("presetId"))
		case "disable":
			revision, err = service.WorldGlassDisable(worldID, user.ID)
		case "activate":
			var body struct {
				PresetID string `json:"presetId"`
			}
			if err := c.BodyParser(&body); err != nil || body.PresetID == "" {
				return worldGlassError(c, service.ErrWorldGlassInvalid)
			}
			revision, err = service.WorldGlassActivatePreset(worldID, user.ID, body.PresetID)
		case "trigger-save":
			var body service.WorldGlassTriggerInput
			if err := c.BodyParser(&body); err != nil {
				return worldGlassError(c, service.ErrWorldGlassInvalid)
			}
			revision, err = service.WorldGlassTriggerUpsert(worldID, user.ID, c.Params("triggerId"), body)
		case "trigger-delete":
			revision, err = service.WorldGlassTriggerDelete(worldID, user.ID, c.Params("triggerId"))
		case "channel-triggers-replace":
			var body struct {
				ChannelIDs []string `json:"channelIds"`
				Enabled    *bool    `json:"enabled"`
			}
			if err := c.BodyParser(&body); err != nil || body.ChannelIDs == nil || body.Enabled == nil {
				return worldGlassError(c, service.ErrWorldGlassInvalid)
			}
			revision, err = service.WorldGlassChannelTriggersReplace(worldID, user.ID, c.Params("presetId"), body.ChannelIDs, *body.Enabled)
		}
		if err != nil {
			return worldGlassError(c, err)
		}
		worldGlassBroadcast(worldID, revision)
		return c.JSON(fiber.Map{"revision": revision})
	}
}

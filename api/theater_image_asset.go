package api

import (
	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func TheaterImageAssetList(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	items, err := service.ListTheaterImageAssets(c.Context(), user.ID, c.Params("worldId"), c.Params("channelId"))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "items": items})
}

func TheaterImageAssetCreate(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	var body struct {
		ResourceID string `json:"resourceId"`
		Name       string `json:"name"`
	}
	if err := decodeTheaterBody(c, &body, 16<<10); err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	item, err := service.CreateTheaterImageAsset(c.Context(), user.ID, c.Params("worldId"), c.Params("channelId"), body.ResourceID, body.Name)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true, "requestId": requestID, "item": item})
}

func TheaterImageAssetUpdate(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeTheaterBody(c, &body, 16<<10); err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	item, err := service.UpdateTheaterImageAsset(c.Context(), user.ID, c.Params("worldId"), c.Params("channelId"), c.Params("assetId"), body.Name)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "item": item})
}

func TheaterImageAssetDelete(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	if err := service.DeleteTheaterImageAsset(c.Context(), user.ID, c.Params("worldId"), c.Params("channelId"), c.Params("assetId")); err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.Status(fiber.StatusNoContent).Send(nil)
}

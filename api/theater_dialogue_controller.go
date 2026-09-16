package api

import (
	"github.com/gofiber/fiber/v2"
	"sealchat/service"
)

func TheaterDialogueControllerGet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "未登录")
	}
	result, err := service.GetTheaterDialogueController(user.ID, c.Params("worldId"))
	if err != nil {
		return theaterErrorResponse(c, theaterRequestID(c), err)
	}
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	return c.JSON(result)
}
func TheaterCharacterOptionsGet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "未登录")
	}
	result, err := service.ListTheaterCharacterOptions(user.ID, c.Params("worldId"), c.Query("keyword"), c.QueryInt("page", 1), c.QueryInt("pageSize", 30))
	if err != nil {
		return theaterErrorResponse(c, theaterRequestID(c), err)
	}
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	return c.JSON(result)
}

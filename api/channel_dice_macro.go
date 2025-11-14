package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

type diceMacroPayload struct {
	Digits   string `json:"digits"`
	Label    string `json:"label"`
	Expr     string `json:"expr"`
	Note     string `json:"note"`
	Favorite bool   `json:"favorite"`
}

func ChannelDiceMacroList(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	user := getCurUser(c)
	items, err := service.DiceMacroList(user.ID, channelID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"items": items})
}

func ChannelDiceMacroCreate(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	payload := diceMacroPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	user := getCurUser(c)
	item, err := service.DiceMacroCreate(user.ID, &service.DiceMacroInput{
		ChannelID: channelID,
		Digits:    payload.Digits,
		Label:     payload.Label,
		Expr:      payload.Expr,
		Note:      payload.Note,
		Favorite:  payload.Favorite,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item})
}

func ChannelDiceMacroUpdate(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	macroID := c.Params("macroId")
	if channelID == "" || macroID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "参数无效"})
	}
	payload := diceMacroPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	user := getCurUser(c)
	item, err := service.DiceMacroUpdate(user.ID, macroID, &service.DiceMacroInput{
		ChannelID: channelID,
		Digits:    payload.Digits,
		Label:     payload.Label,
		Expr:      payload.Expr,
		Note:      payload.Note,
		Favorite:  payload.Favorite,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"item": item})
}

func ChannelDiceMacroDelete(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	macroID := c.Params("macroId")
	if channelID == "" || macroID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "参数无效"})
	}
	user := getCurUser(c)
	if err := service.DiceMacroDelete(user.ID, channelID, macroID); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

type diceMacroImportPayload struct {
	Macros []diceMacroPayload `json:"macros"`
}

func ChannelDiceMacroImport(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	payload := diceMacroImportPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	user := getCurUser(c)
	inputs := make([]*service.DiceMacroInput, 0, len(payload.Macros))
	for _, item := range payload.Macros {
		inputs = append(inputs, &service.DiceMacroInput{
			ChannelID: channelID,
			Digits:    item.Digits,
			Label:     item.Label,
			Expr:      item.Expr,
			Note:      item.Note,
			Favorite:  item.Favorite,
		})
	}
	items, err := service.DiceMacroImport(user.ID, channelID, inputs)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"items": items})
}

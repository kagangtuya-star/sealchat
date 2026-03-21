package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

type characterCardAvatarBindingPayload struct {
	ChannelID          string `json:"channelId"`
	ExternalCardID     string `json:"externalCardId"`
	CardName           string `json:"cardName"`
	SheetType          string `json:"sheetType"`
	AvatarAttachmentID string `json:"avatarAttachmentId"`
}

type characterCardAvatarBindingMigratePayload struct {
	ChannelID string                              `json:"channelId"`
	Items     []characterCardAvatarBindingPayload `json:"items"`
}

func mapCharacterCardAvatarBindingError(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "不存在"):
		return http.StatusNotFound, msg
	case strings.Contains(msg, "无权"), strings.Contains(msg, "成员"), strings.Contains(msg, "无法使用"):
		return http.StatusForbidden, msg
	case strings.Contains(msg, "不能为空"), strings.Contains(msg, "无效"), strings.Contains(msg, "缺少"), strings.Contains(msg, "长度"):
		return http.StatusBadRequest, msg
	default:
		return http.StatusInternalServerError, "操作失败"
	}
}

func CharacterCardAvatarBindingList(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Query("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	user := getCurUser(c)
	items, err := service.CharacterCardAvatarBindingList(user.ID, channelID)
	if err != nil {
		status, msg := mapCharacterCardAvatarBindingError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"items": items})
}

func CharacterCardAvatarBindingUpsert(c *fiber.Ctx) error {
	payload := characterCardAvatarBindingPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	user := getCurUser(c)
	item, err := service.CharacterCardAvatarBindingUpsert(user.ID, &service.CharacterCardAvatarBindingInput{
		ChannelID:          payload.ChannelID,
		ExternalCardID:     payload.ExternalCardID,
		CardName:           payload.CardName,
		SheetType:          payload.SheetType,
		AvatarAttachmentID: payload.AvatarAttachmentID,
	})
	if err != nil {
		status, msg := mapCharacterCardAvatarBindingError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"item": item})
}

func CharacterCardAvatarBindingDelete(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Query("channelId"))
	externalCardID := strings.TrimSpace(c.Query("externalCardId"))
	user := getCurUser(c)
	if err := service.CharacterCardAvatarBindingDelete(user.ID, channelID, externalCardID); err != nil {
		status, msg := mapCharacterCardAvatarBindingError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"success": true})
}

func CharacterCardAvatarBindingMigrateLegacy(c *fiber.Ctx) error {
	payload := characterCardAvatarBindingMigratePayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	inputs := make([]*service.CharacterCardAvatarBindingInput, 0, len(payload.Items))
	for _, item := range payload.Items {
		inputs = append(inputs, &service.CharacterCardAvatarBindingInput{
			ChannelID:          payload.ChannelID,
			ExternalCardID:     item.ExternalCardID,
			CardName:           item.CardName,
			SheetType:          item.SheetType,
			AvatarAttachmentID: item.AvatarAttachmentID,
		})
	}
	user := getCurUser(c)
	items, err := service.CharacterCardAvatarBindingMigrateLegacy(user.ID, payload.ChannelID, inputs)
	if err != nil {
		status, msg := mapCharacterCardAvatarBindingError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"items": items})
}

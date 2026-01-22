package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

type characterCardPayload struct {
	ChannelID string         `json:"channelId"`
	Name      string         `json:"name"`
	SheetType string         `json:"sheetType"`
	Attrs     map[string]any `json:"attrs"`
}

func mapCharacterCardError(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "不存在"):
		return http.StatusNotFound, msg
	case strings.Contains(msg, "无权"), strings.Contains(msg, "所有权"):
		return http.StatusForbidden, msg
	case strings.Contains(msg, "名称"), strings.Contains(msg, "类型"), strings.Contains(msg, "数据"):
		return http.StatusBadRequest, msg
	default:
		return http.StatusInternalServerError, "操作失败"
	}
}

func broadcastCharacterCardEvent(channelID string, card *model.CharacterCardModel, eventType protocol.EventName, action string) {
	if userId2ConnInfoGlobal == nil || channelID == "" || card == nil {
		return
	}
	// Convert model.JSONMap to map[string]any for protocol
	attrs := map[string]any(card.Attrs)
	if attrs == nil {
		attrs = map[string]any{}
	}
	event := &protocol.Event{
		Type:    eventType,
		Channel: &protocol.Channel{ID: channelID},
		CharacterCard: &protocol.CharacterCardEventPayload{
			Card: &protocol.CharacterCard{
				ID:        card.ID,
				UserID:    card.UserID,
				ChannelID: card.ChannelID,
				Name:      card.Name,
				SheetType: card.SheetType,
				Attrs:     attrs,
				UpdatedAt: card.UpdatedAt.Unix(),
			},
			Action: action,
		},
	}
	ctx := &ChatContext{
		UserId2ConnInfo: userId2ConnInfoGlobal,
	}
	ctx.BroadcastEventInChannel(channelID, event)
}

func CharacterCardList(c *fiber.Ctx) error {
	channelID := c.Query("channelId")
	user := getCurUser(c)
	items, err := service.CharacterCardList(user.ID, channelID)
	if err != nil {
		status, msg := mapCharacterCardError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"items": items})
}

func CharacterCardCreate(c *fiber.Ctx) error {
	payload := characterCardPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	user := getCurUser(c)
	item, err := service.CharacterCardCreate(user.ID, &service.CharacterCardInput{
		ChannelID: payload.ChannelID,
		Name:      payload.Name,
		SheetType: payload.SheetType,
		Attrs:     payload.Attrs,
	})
	if err != nil {
		status, msg := mapCharacterCardError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	broadcastCharacterCardEvent(item.ChannelID, item, protocol.EventCharacterCardCreated, "create")
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item})
}

func CharacterCardGet(c *fiber.Ctx) error {
	cardID := c.Params("id")
	if cardID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的角色卡ID"})
	}
	user := getCurUser(c)
	item, err := service.CharacterCardGet(user.ID, cardID)
	if err != nil {
		status, msg := mapCharacterCardError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"item": item})
}

func CharacterCardUpdate(c *fiber.Ctx) error {
	cardID := c.Params("id")
	if cardID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的角色卡ID"})
	}
	payload := characterCardPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	user := getCurUser(c)
	item, err := service.CharacterCardUpdate(user.ID, cardID, &service.CharacterCardInput{
		ChannelID: payload.ChannelID,
		Name:      payload.Name,
		SheetType: payload.SheetType,
		Attrs:     payload.Attrs,
	})
	if err != nil {
		status, msg := mapCharacterCardError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	broadcastCharacterCardEvent(item.ChannelID, item, protocol.EventCharacterCardUpdated, "update")
	return c.JSON(fiber.Map{"item": item})
}

func CharacterCardDelete(c *fiber.Ctx) error {
	cardID := c.Params("id")
	if cardID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的角色卡ID"})
	}
	user := getCurUser(c)
	item, err := service.CharacterCardGet(user.ID, cardID)
	if err != nil {
		status, msg := mapCharacterCardError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	channelID := item.ChannelID
	if err := service.CharacterCardDelete(user.ID, cardID); err != nil {
		status, msg := mapCharacterCardError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	broadcastCharacterCardEvent(channelID, item, protocol.EventCharacterCardDeleted, "delete")
	return c.JSON(fiber.Map{"success": true})
}

type characterCardBindPayload struct {
	CharacterCardID string `json:"characterCardId"`
}

func ChannelIdentityBindCharacterCard(c *fiber.Ctx) error {
	identityID := c.Params("id")
	if identityID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的身份ID"})
	}
	payload := characterCardBindPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if payload.CharacterCardID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少角色卡ID"})
	}
	user := getCurUser(c)
	item, err := service.CharacterCardBindToIdentity(user.ID, identityID, payload.CharacterCardID)
	if err != nil {
		status, msg := mapCharacterCardError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"item": item})
}

func ChannelIdentityUnbindCharacterCard(c *fiber.Ctx) error {
	identityID := c.Params("id")
	if identityID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的身份ID"})
	}
	user := getCurUser(c)
	item, err := service.CharacterCardUnbindFromIdentity(user.ID, identityID)
	if err != nil {
		status, msg := mapCharacterCardError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"item": item})
}

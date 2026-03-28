package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

type channelIdentityPayload struct {
	ChannelID          string   `json:"channelId"`
	DisplayName        string   `json:"displayName"`
	Color              string   `json:"color"`
	AvatarAttachmentID string   `json:"avatarAttachmentId"`
	AvatarDecoration   *protocol.AvatarDecoration `json:"avatarDecoration"`
	AvatarDecorations  protocol.AvatarDecorationList `json:"avatarDecorations"`
	IsDefault          bool     `json:"isDefault"`
	IsTemporary        bool     `json:"isTemporary"`
	ICOOCOnActivate    string   `json:"icOocOnActivate"`
	FolderIDs          []string `json:"folderIds"`
}

func ChannelIdentityList(c *fiber.Ctx) error {
	channelID := c.Query("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "缺少频道ID",
		})
	}
	user := getCurUser(c)
	result, err := service.ChannelIdentityListByUser(channelID, user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	if err := service.ApplyTemporaryIdentityActivateModes(user.ID, result.Items); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	config, err := model.ChannelIdentityModeConfigGet(user.ID, channelID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"items":       result.Items,
		"folders":     result.Folders,
		"favorites":   result.Favorites,
		"membership":  result.Membership,
		"icOocConfig": serializeChannelIdentityModeConfig(config),
	})
}

func ChannelIdentityCreate(c *fiber.Ctx) error {
	payload := channelIdentityPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "请求参数解析失败",
		})
	}
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "缺少频道ID",
		})
	}
	user := getCurUser(c)
	item, err := service.ChannelIdentityCreate(user.ID, &service.ChannelIdentityInput{
		ChannelID:          payload.ChannelID,
		DisplayName:        payload.DisplayName,
		Color:              payload.Color,
		AvatarAttachmentID: payload.AvatarAttachmentID,
		AvatarDecorations:  resolveChannelIdentityPayloadDecorations(payload),
		IsDefault:          payload.IsDefault,
		IsTemporary:        payload.IsTemporary,
		ICOOCOnActivate:    payload.ICOOCOnActivate,
		FolderIDs:          payload.FolderIDs,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"item": item,
	})
}

func ChannelIdentityUpdate(c *fiber.Ctx) error {
	identityID := c.Params("id")
	if identityID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "无效的身份ID",
		})
	}
	payload := channelIdentityPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "请求参数解析失败",
		})
	}
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "缺少频道ID",
		})
	}
	user := getCurUser(c)
	item, err := service.ChannelIdentityUpdate(user.ID, identityID, &service.ChannelIdentityInput{
		ChannelID:          payload.ChannelID,
		DisplayName:        payload.DisplayName,
		Color:              payload.Color,
		AvatarAttachmentID: payload.AvatarAttachmentID,
		AvatarDecorations:  resolveChannelIdentityPayloadDecorations(payload),
		IsDefault:          payload.IsDefault,
		IsTemporary:        payload.IsTemporary,
		ICOOCOnActivate:    payload.ICOOCOnActivate,
		FolderIDs:          payload.FolderIDs,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"item": item,
	})
}

func ChannelIdentityDelete(c *fiber.Ctx) error {
	identityID := c.Params("id")
	if identityID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "无效的身份ID",
		})
	}
	channelID := c.Query("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "缺少频道ID",
		})
	}
	user := getCurUser(c)
	if err := service.ChannelIdentityDelete(user.ID, channelID, identityID); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"success": true,
	})
}

func ChannelIdentityReplaceTemporary(c *fiber.Ctx) error {
	identityID := c.Params("id")
	if identityID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "无效的身份ID",
		})
	}
	payload := channelIdentityPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "请求参数解析失败",
		})
	}
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "缺少频道ID",
		})
	}
	user := getCurUser(c)
	result, err := service.ChannelIdentityReplaceTemporary(user.ID, identityID, &service.ChannelIdentityInput{
		ChannelID:          payload.ChannelID,
		DisplayName:        payload.DisplayName,
		Color:              payload.Color,
		AvatarAttachmentID: payload.AvatarAttachmentID,
		AvatarDecorations:  resolveChannelIdentityPayloadDecorations(payload),
		IsDefault:          payload.IsDefault,
		ICOOCOnActivate:    payload.ICOOCOnActivate,
		FolderIDs:          payload.FolderIDs,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"item":          result.Item,
		"oldIdentityId": result.OldIdentityID,
		"removedId":     result.RemovedID,
	})
}

func resolveChannelIdentityPayloadDecorations(payload channelIdentityPayload) protocol.AvatarDecorationList {
	if len(payload.AvatarDecorations) > 0 {
		return payload.AvatarDecorations
	}
	if payload.AvatarDecoration != nil {
		return protocol.AvatarDecorationList{*payload.AvatarDecoration}
	}
	return nil
}

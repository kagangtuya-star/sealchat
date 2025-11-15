package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

type channelIdentityPayload struct {
	ChannelID          string   `json:"channelId"`
	DisplayName        string   `json:"displayName"`
	Color              string   `json:"color"`
	AvatarAttachmentID string   `json:"avatarAttachmentId"`
	IsDefault          bool     `json:"isDefault"`
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

	return c.JSON(fiber.Map{
		"items":      result.Items,
		"folders":    result.Folders,
		"favorites":  result.Favorites,
		"membership": result.Membership,
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
		IsDefault:          payload.IsDefault,
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
		IsDefault:          payload.IsDefault,
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

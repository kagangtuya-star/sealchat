package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
)

type channelIdentityVariantPayload struct {
	ChannelID          string         `json:"channelId"`
	TargetUserID       string         `json:"targetUserId"`
	IdentityID         string         `json:"identityId"`
	SelectorEmoji      string         `json:"selectorEmoji"`
	Keyword            string         `json:"keyword"`
	Note               string         `json:"note"`
	AvatarAttachmentID string         `json:"avatarAttachmentId"`
	DisplayName        string         `json:"displayName"`
	Color              string         `json:"color"`
	Appearance         map[string]any `json:"appearance"`
	Enabled            bool           `json:"enabled"`
}

func serializeChannelIdentityVariant(item *model.ChannelIdentityVariantModel) fiber.Map {
	if item == nil {
		return fiber.Map{}
	}
	return fiber.Map{
		"id":                 item.ID,
		"identityId":         item.IdentityID,
		"channelId":          item.ChannelID,
		"userId":             item.UserID,
		"selectorEmoji":      item.SelectorEmoji,
		"keyword":            item.Keyword,
		"note":               item.Note,
		"avatarAttachmentId": item.AvatarAttachmentID,
		"displayName":        item.DisplayName,
		"color":              item.Color,
		"appearance":         item.Appearance(),
		"sortOrder":          item.SortOrder,
		"enabled":            item.Enabled,
		"createdAt":          item.CreatedAt,
		"updatedAt":          item.UpdatedAt,
	}
}

func ChannelIdentityVariantList(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Query("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	identityID := strings.TrimSpace(c.Query("identityId"))
	ctx, err := resolveChannelIdentityActorFromRequest(c, channelID, strings.TrimSpace(c.Query("targetUserId")))
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	var (
		items []*model.ChannelIdentityVariantModel
	)
	if identityID != "" {
		if _, err = model.ChannelIdentityValidateOwnership(identityID, ctx.TargetUserID, channelID); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		items, err = model.ChannelIdentityVariantListByIdentityID(channelID, ctx.TargetUserID, identityID)
	} else {
		items, err = service.ChannelIdentityVariantListByUser(channelID, ctx.TargetUserID)
	}
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	result := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		result = append(result, serializeChannelIdentityVariant(item))
	}
	return c.JSON(fiber.Map{"items": result})
}

func ChannelIdentityVariantCreate(c *fiber.Ctx) error {
	payload := channelIdentityVariantPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	item, err := service.ChannelIdentityVariantCreateWithAccess(ctx.TargetUserID, ctx.OperatorUserID, &service.ChannelIdentityVariantInput{
		ChannelID:          payload.ChannelID,
		IdentityID:         payload.IdentityID,
		SelectorEmoji:      payload.SelectorEmoji,
		Keyword:            payload.Keyword,
		Note:               payload.Note,
		AvatarAttachmentID: payload.AvatarAttachmentID,
		DisplayName:        payload.DisplayName,
		Color:              payload.Color,
		Appearance:         payload.Appearance,
		Enabled:            payload.Enabled,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": serializeChannelIdentityVariant(item)})
}

func ChannelIdentityVariantUpdate(c *fiber.Ctx) error {
	variantID := strings.TrimSpace(c.Params("id"))
	if variantID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的差分ID"})
	}
	payload := channelIdentityVariantPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	item, err := service.ChannelIdentityVariantUpdateWithAccess(ctx.TargetUserID, ctx.OperatorUserID, variantID, &service.ChannelIdentityVariantInput{
		ChannelID:          payload.ChannelID,
		IdentityID:         payload.IdentityID,
		SelectorEmoji:      payload.SelectorEmoji,
		Keyword:            payload.Keyword,
		Note:               payload.Note,
		AvatarAttachmentID: payload.AvatarAttachmentID,
		DisplayName:        payload.DisplayName,
		Color:              payload.Color,
		Appearance:         payload.Appearance,
		Enabled:            payload.Enabled,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"item": serializeChannelIdentityVariant(item)})
}

func ChannelIdentityVariantDelete(c *fiber.Ctx) error {
	variantID := strings.TrimSpace(c.Params("id"))
	channelID := strings.TrimSpace(c.Query("channelId"))
	if variantID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的差分ID"})
	}
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, channelID, strings.TrimSpace(c.Query("targetUserId")))
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	if err := service.ChannelIdentityVariantDeleteWithAccess(ctx.TargetUserID, ctx.OperatorUserID, channelID, variantID); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

func ChannelIdentityVariantReorder(c *fiber.Ctx) error {
	var payload struct {
		ChannelID    string   `json:"channelId"`
		TargetUserID string   `json:"targetUserId"`
		IdentityID   string   `json:"identityId"`
		IDs          []string `json:"ids"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	if err := service.ChannelIdentityVariantReorderWithAccess(ctx.TargetUserID, ctx.OperatorUserID, payload.ChannelID, payload.IdentityID, payload.IDs); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	items, err := model.ChannelIdentityVariantListByIdentityID(payload.ChannelID, ctx.TargetUserID, payload.IdentityID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	result := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		result = append(result, serializeChannelIdentityVariant(item))
	}
	return c.JSON(fiber.Map{"items": result})
}

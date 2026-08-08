package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

type channelIdentityVariantPayload struct {
	ChannelID                  string                                    `json:"channelId"`
	TargetUserID               string                                    `json:"targetUserId"`
	IdentityID                 string                                    `json:"identityId"`
	SelectorEmoji              string                                    `json:"selectorEmoji"`
	Keyword                    string                                    `json:"keyword"`
	Note                       string                                    `json:"note"`
	AvatarAttachmentID         string                                    `json:"avatarAttachmentId"`
	DisplayName                string                                    `json:"displayName"`
	Color                      string                                    `json:"color"`
	Appearance                 map[string]any                            `json:"appearance"`
	Enabled                    bool                                      `json:"enabled"`
	TheaterPresentation        protocol.OptionalTheaterPresentationPatch `json:"theaterPresentation"`
	SkipTheaterAssetValidation bool                                      `json:"skipTheaterAssetValidation"`
	ExpectedRevision           int64                                     `json:"expectedRevision"`
}

func serializeChannelIdentityVariant(item *model.ChannelIdentityVariantModel) fiber.Map {
	if item == nil {
		return fiber.Map{}
	}
	appearance := item.Appearance()
	result := fiber.Map{
		"id":                 item.ID,
		"identityId":         item.IdentityID,
		"channelId":          item.ChannelID,
		"userId":             item.UserID,
		"sharedVariantId":    item.SharedVariantID,
		"sharedRevision":     item.SharedRevision,
		"selectorEmoji":      item.SelectorEmoji,
		"keyword":            item.Keyword,
		"note":               item.Note,
		"avatarAttachmentId": item.AvatarAttachmentID,
		"displayName":        item.DisplayName,
		"color":              item.Color,
		"appearance":         appearance,
		"sortOrder":          item.SortOrder,
		"enabled":            item.Enabled,
		"createdAt":          item.CreatedAt,
		"updatedAt":          item.UpdatedAt,
	}
	if theaterPresentation, exists := appearance["theaterPresentation"]; exists {
		result["theaterPresentation"] = theaterPresentation
	}
	return result
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
		if _, err = service.ValidateChannelIdentityActorIdentity(ctx, channelID, identityID); err != nil {
			return handleChannelIdentityActorErr(c, err)
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

func ChannelIdentityVariantGet(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Query("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, channelID, strings.TrimSpace(c.Query("targetUserId")))
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	item, err := service.ChannelIdentityVariantGetForUser(ctx.TargetUserID, channelID, strings.TrimSpace(c.Params("id")))
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"item": serializeChannelIdentityVariant(item)})
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
	if _, err := service.ValidateChannelIdentityActorIdentity(ctx, payload.ChannelID, payload.IdentityID); err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	item, err := service.ChannelIdentityVariantCreateWithAccess(ctx.TargetUserID, ctx.OperatorUserID, &service.ChannelIdentityVariantInput{
		ChannelID:                  payload.ChannelID,
		IdentityID:                 payload.IdentityID,
		SelectorEmoji:              payload.SelectorEmoji,
		Keyword:                    payload.Keyword,
		Note:                       payload.Note,
		AvatarAttachmentID:         payload.AvatarAttachmentID,
		DisplayName:                payload.DisplayName,
		Color:                      payload.Color,
		Appearance:                 payload.Appearance,
		Enabled:                    payload.Enabled,
		TheaterPresentation:        payload.TheaterPresentation.Value,
		TheaterPresentationSet:     payload.TheaterPresentation.Set,
		SkipTheaterAssetValidation: payload.SkipTheaterAssetValidation,
		ExpectedRevision:           payload.ExpectedRevision,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastSharedChannelIdentityVariantRefresh(item, ctx.TargetUserID, ctx.OperatorUserID, "identity-variant-create")
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
	if _, err := service.ValidateChannelIdentityActorIdentity(ctx, payload.ChannelID, payload.IdentityID); err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	item, err := service.ChannelIdentityVariantUpdateWithAccess(ctx.TargetUserID, ctx.OperatorUserID, variantID, &service.ChannelIdentityVariantInput{
		ChannelID:                  payload.ChannelID,
		IdentityID:                 payload.IdentityID,
		SelectorEmoji:              payload.SelectorEmoji,
		Keyword:                    payload.Keyword,
		Note:                       payload.Note,
		AvatarAttachmentID:         payload.AvatarAttachmentID,
		DisplayName:                payload.DisplayName,
		Color:                      payload.Color,
		Appearance:                 payload.Appearance,
		Enabled:                    payload.Enabled,
		TheaterPresentation:        payload.TheaterPresentation.Value,
		TheaterPresentationSet:     payload.TheaterPresentation.Set,
		SkipTheaterAssetValidation: payload.SkipTheaterAssetValidation,
		ExpectedRevision:           payload.ExpectedRevision,
	})
	if err != nil {
		if errors.Is(err, service.ErrSharedChannelIdentityVariantRevisionConflict) {
			copies, _ := model.SharedChannelIdentityVariantCopies(itemSharedVariantID(variantID))
			var revision int64
			for _, copy := range copies {
				if copy.SharedRevision > revision {
					revision = copy.SharedRevision
				}
			}
			return c.Status(http.StatusConflict).JSON(fiber.Map{"error": err.Error(), "revision": revision})
		}
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastSharedChannelIdentityVariantRefresh(item, ctx.TargetUserID, ctx.OperatorUserID, "identity-variant-update")
	return c.JSON(fiber.Map{"item": serializeChannelIdentityVariant(item)})
}

func itemSharedVariantID(variantID string) string {
	item, err := model.ChannelIdentityVariantGetByID(strings.TrimSpace(variantID))
	if err != nil {
		return ""
	}
	return item.SharedVariantID
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
	if ctx.IsBotTarget {
		variant, variantErr := service.ChannelIdentityVariantGetForUser(ctx.TargetUserID, channelID, variantID)
		if variantErr != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": variantErr.Error()})
		}
		if _, err := service.ValidateChannelIdentityActorIdentity(ctx, channelID, variant.IdentityID); err != nil {
			return handleChannelIdentityActorErr(c, err)
		}
	}
	item, _ := service.ChannelIdentityVariantGetForUser(ctx.TargetUserID, channelID, variantID)
	if err := service.ChannelIdentityVariantDeleteWithAccess(ctx.TargetUserID, ctx.OperatorUserID, channelID, variantID); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastSharedChannelIdentityVariantRefresh(item, ctx.TargetUserID, ctx.OperatorUserID, "identity-variant-delete")
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
	if _, err := service.ValidateChannelIdentityActorIdentity(ctx, payload.ChannelID, payload.IdentityID); err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	if err := service.ChannelIdentityVariantReorderWithAccess(ctx.TargetUserID, ctx.OperatorUserID, payload.ChannelID, payload.IdentityID, payload.IDs); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	identity, _ := model.ChannelIdentityGetByID(payload.IdentityID)
	broadcastSharedChannelIdentityVariantRefreshForIdentity(identity, ctx.TargetUserID, ctx.OperatorUserID, "identity-variant-reorder")
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

func broadcastSharedChannelIdentityVariantRefresh(item *model.ChannelIdentityVariantModel, targetUserID, operatorUserID, reason string) {
	if item == nil {
		return
	}
	identity, err := model.ChannelIdentityGetByID(item.IdentityID)
	if err != nil {
		broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{ChannelID: item.ChannelID, TargetUserID: targetUserID, OperatorUserID: operatorUserID, Reason: reason})
		return
	}
	broadcastSharedChannelIdentityVariantRefreshForIdentity(identity, targetUserID, operatorUserID, reason)
}

func broadcastSharedChannelIdentityVariantRefreshForIdentity(identity *model.ChannelIdentityModel, targetUserID, operatorUserID, reason string) {
	if identity == nil || identity.SharedIdentityID == "" {
		if identity != nil {
			broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{ChannelID: identity.ChannelID, TargetUserID: targetUserID, OperatorUserID: operatorUserID, Reason: reason})
		}
		return
	}
	copies, err := model.SharedChannelIdentityCopies(identity.SharedIdentityID)
	if err != nil {
		broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{ChannelID: identity.ChannelID, TargetUserID: targetUserID, OperatorUserID: operatorUserID, Reason: reason})
		return
	}
	for _, copy := range copies {
		broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{ChannelID: copy.ChannelID, TargetUserID: targetUserID, OperatorUserID: operatorUserID, Reason: reason})
	}
}

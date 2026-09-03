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

type channelIdentityPayload struct {
	ChannelID                  string                               `json:"channelId"`
	TargetUserID               string                               `json:"targetUserId"`
	DisplayName                string                               `json:"displayName"`
	Color                      string                               `json:"color"`
	AvatarAttachmentID         string                               `json:"avatarAttachmentId"`
	AvatarDecoration           *protocol.AvatarDecoration           `json:"avatarDecoration"`
	AvatarDecorations          protocol.AvatarDecorationList        `json:"avatarDecorations"`
	IsDefault                  bool                                 `json:"isDefault"`
	IsTemporary                bool                                 `json:"isTemporary"`
	BotAppearanceMode          string                               `json:"botAppearanceMode"`
	VariantResetMatchMode      *string                              `json:"variantResetMatchMode"`
	VariantResetMatchConfig    *string                              `json:"variantResetMatchConfig"`
	VariantResetMatchContent   *string                              `json:"variantResetMatchContent"`
	ICOOCOnActivate            string                               `json:"icOocOnActivate"`
	FolderIDs                  []string                             `json:"folderIds"`
	TheaterPresentation        protocol.OptionalTheaterPresentation `json:"theaterPresentation"`
	SkipTheaterAssetValidation bool                                 `json:"skipTheaterAssetValidation"`
	PromoteToShared            bool                                 `json:"promoteToShared"`
}

type sharedChannelIdentityTheaterPresentationPayload struct {
	ChannelID           string                        `json:"channelId"`
	TheaterPresentation *protocol.TheaterPresentation `json:"theaterPresentation"`
	ExpectedRevision    int64                         `json:"expectedRevision"`
}

func ChannelIdentityList(c *fiber.Ctx) error {
	channelID := c.Query("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "缺少频道ID",
		})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, channelID, strings.TrimSpace(c.Query("targetUserId")))
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	if ctx.IsBotTarget {
		if err := service.EnsureBotChannelIdentity(ctx.TargetUserID, channelID); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}
	result, err := service.ChannelIdentityListByUser(channelID, ctx.TargetUserID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	if err := service.ApplyTemporaryIdentityActivateModes(ctx.TargetUserID, result.Items); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	if result.Repaired {
		broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{
			ChannelID: channelID, TargetUserID: ctx.TargetUserID, OperatorUserID: ctx.OperatorUserID, Reason: "identity-repair",
		})
	}
	config, err := model.ChannelIdentityModeConfigGet(ctx.TargetUserID, channelID)
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

func ChannelIdentityGet(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Query("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, channelID, strings.TrimSpace(c.Query("targetUserId")))
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	item, err := model.ChannelIdentityValidateOwnership(strings.TrimSpace(c.Params("id")), ctx.TargetUserID, channelID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"item": item})
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
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	if ctx.IsBotTarget {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "BOT 仅允许编辑默认频道外观"})
	}
	item, err := service.ChannelIdentityCreateWithAccess(ctx.TargetUserID, ctx.OperatorUserID, &service.ChannelIdentityInput{
		ChannelID:                  payload.ChannelID,
		DisplayName:                payload.DisplayName,
		Color:                      payload.Color,
		AvatarAttachmentID:         payload.AvatarAttachmentID,
		AvatarDecorations:          resolveChannelIdentityPayloadDecorations(payload),
		IsDefault:                  payload.IsDefault,
		IsTemporary:                payload.IsTemporary,
		VariantResetMatchMode:      channelIdentityOptionalString(payload.VariantResetMatchMode),
		VariantResetMatchConfig:    channelIdentityOptionalString(payload.VariantResetMatchConfig),
		VariantResetMatchContent:   channelIdentityOptionalString(payload.VariantResetMatchContent),
		VariantResetMatchSet:       payload.VariantResetMatchMode != nil || payload.VariantResetMatchConfig != nil || payload.VariantResetMatchContent != nil,
		ICOOCOnActivate:            payload.ICOOCOnActivate,
		FolderIDs:                  payload.FolderIDs,
		TheaterPresentation:        payload.TheaterPresentation.Value,
		TheaterPresentationSet:     payload.TheaterPresentation.Set,
		SkipTheaterAssetValidation: payload.SkipTheaterAssetValidation,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{
		ChannelID:      payload.ChannelID,
		TargetUserID:   ctx.TargetUserID,
		OperatorUserID: ctx.OperatorUserID,
		Reason:         "identity-create",
	})

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
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	if ctx.IsBotTarget {
		payload.IsDefault = true
		payload.IsTemporary = false
		payload.FolderIDs = nil
	}
	if payload.PromoteToShared && ctx.OperatorUserID != ctx.TargetUserID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": service.ErrSharedChannelIdentityOwnerOnly.Error()})
	}
	updateInput := &service.ChannelIdentityInput{
		ChannelID:                  payload.ChannelID,
		DisplayName:                payload.DisplayName,
		Color:                      payload.Color,
		AvatarAttachmentID:         payload.AvatarAttachmentID,
		AvatarDecorations:          resolveChannelIdentityPayloadDecorations(payload),
		IsDefault:                  payload.IsDefault,
		IsTemporary:                payload.IsTemporary,
		BotAppearanceMode:          payload.BotAppearanceMode,
		VariantResetMatchMode:      channelIdentityOptionalString(payload.VariantResetMatchMode),
		VariantResetMatchConfig:    channelIdentityOptionalString(payload.VariantResetMatchConfig),
		VariantResetMatchContent:   channelIdentityOptionalString(payload.VariantResetMatchContent),
		VariantResetMatchSet:       payload.VariantResetMatchMode != nil || payload.VariantResetMatchConfig != nil || payload.VariantResetMatchContent != nil,
		ICOOCOnActivate:            payload.ICOOCOnActivate,
		FolderIDs:                  payload.FolderIDs,
		TheaterPresentation:        payload.TheaterPresentation.Value,
		TheaterPresentationSet:     payload.TheaterPresentation.Set,
		SkipTheaterAssetValidation: payload.SkipTheaterAssetValidation,
	}
	var updateResult *service.ChannelIdentityUpdateResult
	if ctx.IsBotTarget {
		updateResult, err = service.BotManagedChannelIdentityUpdate(ctx, identityID, updateInput)
	} else {
		updateResult, err = service.ChannelIdentityUpdateDetailedWithAccess(ctx.TargetUserID, ctx.OperatorUserID, identityID, updateInput, payload.PromoteToShared)
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrSharedChannelIdentityOwnerOnly) || errors.Is(err, service.ErrSharedChannelIdentitySynchronizedFieldsReadOnly) {
			status = http.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	item := updateResult.Item
	broadcastUpdatedSharedChannelIdentityCopies(item, ctx.TargetUserID, ctx.OperatorUserID, "identity-update", ctx.IsBotTarget)
	return c.JSON(fiber.Map{
		"item":       item,
		"sharedSync": updateResult.SharedSync,
	})
}

func SharedChannelIdentityTheaterPresentationSet(c *fiber.Ctx) error {
	identityID := strings.TrimSpace(c.Params("id"))
	if identityID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的身份ID"})
	}
	payload := sharedChannelIdentityTheaterPresentationPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	payload.ChannelID = strings.TrimSpace(payload.ChannelID)
	if payload.ChannelID == "" || payload.TheaterPresentation == nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID或演出外观"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, "")
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	result, err := service.SharedChannelIdentityTheaterPresentationSet(
		ctx.TargetUserID,
		ctx.OperatorUserID,
		identityID,
		payload.ChannelID,
		payload.TheaterPresentation,
		payload.ExpectedRevision,
	)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, service.ErrSharedChannelIdentityOwnerOnly):
			status = http.StatusForbidden
		case errors.Is(err, service.ErrSharedChannelIdentityRevisionConflict):
			if identity, identityErr := model.ChannelIdentityValidateOwnership(identityID, ctx.TargetUserID, payload.ChannelID); identityErr == nil && identity.SharedIdentityID != "" {
				if template, templateErr := model.SharedChannelIdentityGetByID(identity.SharedIdentityID); templateErr == nil {
					return c.Status(http.StatusConflict).JSON(fiber.Map{
						"error":        err.Error(),
						"revision":     template.Revision,
						"presentation": template.TheaterPresentation,
					})
				}
			}
			status = http.StatusConflict
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}
	var item *model.ChannelIdentityModel
	for _, copy := range result.Copies {
		if copy.ID == identityID {
			item = copy
			break
		}
	}
	if item == nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "共享角色当前频道投影不存在"})
	}
	broadcastUpdatedSharedChannelIdentityCopies(item, ctx.TargetUserID, ctx.OperatorUserID, "shared-theater-presentation-update", false)
	return c.JSON(fiber.Map{
		"item":         item,
		"presentation": result.Template.TheaterPresentation,
		"revision":     result.Template.Revision,
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
	ctx, err := resolveChannelIdentityActorFromRequest(c, channelID, strings.TrimSpace(c.Query("targetUserId")))
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	if ctx.IsBotTarget {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "不能删除 BOT 默认频道外观"})
	}
	var sharedCopies []*model.ChannelIdentityModel
	if identity, identityErr := model.ChannelIdentityValidateOwnership(identityID, ctx.TargetUserID, channelID); identityErr == nil && identity.SharedIdentityID != "" {
		sharedCopies, _ = model.SharedChannelIdentityCopies(identity.SharedIdentityID)
	}
	if err := service.ChannelIdentityDeleteWithAccess(ctx.TargetUserID, ctx.OperatorUserID, channelID, identityID); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrSharedChannelIdentityOwnerOnly) {
			status = http.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	if len(sharedCopies) == 0 {
		sharedCopies = []*model.ChannelIdentityModel{{ChannelID: channelID}}
	}
	for _, copy := range sharedCopies {
		broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{ChannelID: copy.ChannelID, TargetUserID: ctx.TargetUserID, OperatorUserID: ctx.OperatorUserID, Reason: "identity-delete"})
	}
	return c.JSON(fiber.Map{
		"success": true,
	})
}

func broadcastUpdatedSharedChannelIdentityCopies(item *model.ChannelIdentityModel, targetUserID, operatorUserID, reason string, botManaged bool) {
	if item == nil || item.SharedIdentityID == "" || (targetUserID != operatorUserID && !botManaged) {
		if item != nil {
			broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{ChannelID: item.ChannelID, TargetUserID: targetUserID, OperatorUserID: operatorUserID, Reason: reason})
		}
		return
	}
	copies, err := model.SharedChannelIdentityCopies(item.SharedIdentityID)
	if err != nil {
		broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{ChannelID: item.ChannelID, TargetUserID: targetUserID, OperatorUserID: operatorUserID, Reason: reason})
		return
	}
	for _, copy := range copies {
		if copy.SharedRevision != item.SharedRevision {
			continue
		}
		broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{ChannelID: copy.ChannelID, TargetUserID: targetUserID, OperatorUserID: operatorUserID, Reason: reason})
	}
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
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	if ctx.IsBotTarget {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "BOT 不支持临时频道角色"})
	}
	result, err := service.ChannelIdentityReplaceTemporaryWithAccess(ctx.TargetUserID, ctx.OperatorUserID, identityID, &service.ChannelIdentityInput{
		ChannelID:              payload.ChannelID,
		DisplayName:            payload.DisplayName,
		Color:                  payload.Color,
		AvatarAttachmentID:     payload.AvatarAttachmentID,
		AvatarDecorations:      resolveChannelIdentityPayloadDecorations(payload),
		IsDefault:              payload.IsDefault,
		ICOOCOnActivate:        payload.ICOOCOnActivate,
		FolderIDs:              payload.FolderIDs,
		TheaterPresentation:    payload.TheaterPresentation.Value,
		TheaterPresentationSet: payload.TheaterPresentation.Set,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{
		ChannelID:      payload.ChannelID,
		TargetUserID:   ctx.TargetUserID,
		OperatorUserID: ctx.OperatorUserID,
		Reason:         "identity-replace-temporary",
	})
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

func channelIdentityOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

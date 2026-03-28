package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
)

type channelIdentityModeConfigPayload struct {
	TargetUserID string  `json:"targetUserId"`
	ICRoleID     *string `json:"icRoleId"`
	OOCRoleID    *string `json:"oocRoleId"`
}

func serializeChannelIdentityModeConfig(config *model.ChannelIdentityModeConfigModel) fiber.Map {
	if config == nil {
		return fiber.Map{
			"icRoleId":  nil,
			"oocRoleId": nil,
		}
	}
	return fiber.Map{
		"icRoleId":  nullableJSONID(config.ICIdentityID),
		"oocRoleId": nullableJSONID(config.OOCIdentityID),
	}
}

func nullableJSONID(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func normalizeOptionalID(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func ChannelIdentityModeConfigUpsert(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}

	var body channelIdentityModeConfigPayload
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, channelID, body.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}

	icRoleID := normalizeOptionalID(body.ICRoleID)
	oocRoleID := normalizeOptionalID(body.OOCRoleID)
	if icRoleID != "" {
		if _, err := model.ChannelIdentityValidateOwnership(icRoleID, ctx.TargetUserID, channelID); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "场内角色不存在或不属于当前用户"})
		}
	}
	if oocRoleID != "" {
		if _, err := model.ChannelIdentityValidateOwnership(oocRoleID, ctx.TargetUserID, channelID); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "场外角色不存在或不属于当前用户"})
		}
	}

	if icRoleID == "" && oocRoleID == "" {
		if err := model.ChannelIdentityModeConfigDelete(ctx.TargetUserID, channelID); err != nil {
			return wrapError(c, err, "保存场内场外角色映射失败")
		}
		broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{
			ChannelID:      channelID,
			TargetUserID:   ctx.TargetUserID,
			OperatorUserID: ctx.OperatorUserID,
			Reason:         "identity-mode-config-delete",
		})
		return c.JSON(fiber.Map{
			"channelId": channelID,
			"exists":    false,
			"config": fiber.Map{
				"icRoleId":  nil,
				"oocRoleId": nil,
			},
		})
	}

	record, err := model.ChannelIdentityModeConfigUpsert(ctx.TargetUserID, channelID, icRoleID, oocRoleID)
	if err != nil {
		return wrapError(c, err, "保存场内场外角色映射失败")
	}
	broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{
		ChannelID:      channelID,
		TargetUserID:   ctx.TargetUserID,
		OperatorUserID: ctx.OperatorUserID,
		Reason:         "identity-mode-config-upsert",
	})
	return c.JSON(fiber.Map{
		"channelId": channelID,
		"exists":    record != nil,
		"config":    serializeChannelIdentityModeConfig(record),
	})
}

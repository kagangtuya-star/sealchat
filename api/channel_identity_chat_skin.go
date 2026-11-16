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

type channelIdentityChatSkinPayload struct {
	ChannelID    string            `json:"channelId"`
	TargetUserID string            `json:"targetUserId"`
	ChatSkin     protocol.ChatSkin `json:"chatSkin"`
}

type channelIdentityVariantChatSkinPayload struct {
	ChannelID    string                  `json:"channelId"`
	TargetUserID string                  `json:"targetUserId"`
	ChatSkin     *protocol.ChatSkinPatch `json:"chatSkin"`
}

func chatSkinErrorStatus(err error) int {
	if errors.Is(err, service.ErrSharedChannelIdentityOwnerOnly) || errors.Is(err, service.ErrSharedChannelIdentitySynchronizedFieldsReadOnly) {
		return http.StatusForbidden
	}
	if errors.Is(err, protocol.ErrChatSkinTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

// ChannelIdentityChatSkinSet 保存频道角色的基础聊天装扮；chatSkin 为 null 时清除。
func ChannelIdentityChatSkinSet(c *fiber.Ctx) error {
	identityID := strings.TrimSpace(c.Params("id"))
	if identityID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的身份ID"})
	}
	payload := channelIdentityChatSkinPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	payload.ChannelID = strings.TrimSpace(payload.ChannelID)
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	result, err := service.ChannelIdentityChatSkinSet(ctx, payload.ChannelID, identityID, payload.ChatSkin)
	if err != nil {
		return c.Status(chatSkinErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastUpdatedSharedChannelIdentityCopies(result.Item, ctx.TargetUserID, ctx.OperatorUserID, "identity-chat-skin-update", ctx.IsBotTarget)
	return c.JSON(fiber.Map{"item": result.Item})
}

// ChannelIdentityVariantChatSkinSet 保存差分的聊天装扮覆写；chatSkin 为 null 时回到基础装扮。
func ChannelIdentityVariantChatSkinSet(c *fiber.Ctx) error {
	variantID := strings.TrimSpace(c.Params("id"))
	if variantID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的差分ID"})
	}
	payload := channelIdentityVariantChatSkinPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	payload.ChannelID = strings.TrimSpace(payload.ChannelID)
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	item, err := service.ChannelIdentityVariantChatSkinSet(ctx, payload.ChannelID, variantID, payload.ChatSkin)
	if err != nil {
		if errors.Is(err, service.ErrSharedChannelIdentityVariantRevisionConflict) {
			return c.Status(http.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(chatSkinErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastSharedChannelIdentityVariantRefresh(item, ctx.TargetUserID, ctx.OperatorUserID, "identity-variant-chat-skin-update")
	return c.JSON(fiber.Map{"item": serializeChannelIdentityVariant(item)})
}

// typingPreviewChatSkin 返回输入预览应展示的装扮：编辑消息且身份、差分都未改变时，
// 服务端保存时会保留原消息快照，因此预览也使用该快照，避免预览与保存结果不一致。
func typingPreviewChatSkin(channelID, userID, mode, messageID, identityID string, appearance *service.ResolvedIdentityAppearance) protocol.ChatSkin {
	if appearance == nil {
		return nil
	}
	messageID = strings.TrimSpace(messageID)
	if mode != "editing" || messageID == "" {
		return appearance.ChatSkin
	}
	var msg model.MessageModel
	err := model.GetDB().Select("id", "channel_id", "user_id", "sender_identity_id", "sender_identity_variant_id", "sender_chat_skin").
		Where("id = ? AND channel_id = ?", messageID, channelID).Limit(1).Find(&msg).Error
	if err != nil || msg.ID == "" || msg.UserID != userID {
		return appearance.ChatSkin
	}
	if msg.SenderIdentityID == identityID && msg.SenderIdentityVariantID == appearance.VariantID {
		return msg.SenderChatSkin
	}
	return appearance.ChatSkin
}

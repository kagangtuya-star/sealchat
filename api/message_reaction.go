package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

type messageReactionRequest struct {
	Emoji      string `json:"emoji"`
	IdentityID string `json:"identity_id"`
}

func MessageReactionAdd(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	msg, channel, status, errMsg, err := resolveReactionMessage(user, c.Params("messageId"))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": errMsg})
	}

	var req messageReactionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	emoji := strings.TrimSpace(req.Emoji)
	if emoji == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "emoji 不能为空"})
	}

	identityID := strings.TrimSpace(req.IdentityID)
	identity, err := service.ChannelIdentityValidateMessageIdentity(user.ID, channel.ID, identityID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "身份校验失败"})
	}
	if identity == nil && identityID == "" {
		identity, _ = service.EnsureHiddenDefaultIdentity(user.ID, channel.ID)
	}
	if identity != nil {
		identityID = identity.ID
	}

	summary, err := service.AddMessageReaction(msg.ID, user.ID, emoji, identityID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	broadcastMessageReaction(channel, msg, user, summary, "add")

	return c.JSON(fiber.Map{
		"ok":        true,
		"messageId": summary.MessageID,
		"emoji":     summary.Emoji,
		"count":     summary.Count,
		"meReacted": summary.MeReacted,
	})
}

func MessageReactionRemove(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	msg, channel, status, errMsg, err := resolveReactionMessage(user, c.Params("messageId"))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": errMsg})
	}

	var req messageReactionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	emoji := strings.TrimSpace(req.Emoji)
	if emoji == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "emoji 不能为空"})
	}

	summary, err := service.RemoveMessageReaction(msg.ID, user.ID, emoji)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	broadcastMessageReaction(channel, msg, user, summary, "remove")

	return c.JSON(fiber.Map{
		"ok":        true,
		"messageId": summary.MessageID,
		"emoji":     summary.Emoji,
		"count":     summary.Count,
		"meReacted": summary.MeReacted,
	})
}

func MessageReactionList(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	msg, _, status, errMsg, err := resolveReactionMessage(user, c.Params("messageId"))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": errMsg})
	}

	items, err := service.ListMessageReactions(msg.ID, user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"items": items,
	})
}

func MessageReactionUsers(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	msg, channel, status, errMsg, err := resolveReactionMessage(user, c.Params("messageId"))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": errMsg})
	}

	emoji := strings.TrimSpace(c.Query("emoji"))
	if emoji == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "emoji 不能为空"})
	}

	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)
	items, total, err := service.ListMessageReactionUsers(msg.ID, channel.ID, emoji, limit, offset)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"items": items,
		"total": total,
	})
}

func resolveReactionMessage(user *model.UserModel, messageID string) (*model.MessageModel, *model.ChannelModel, int, string, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, nil, http.StatusBadRequest, "缺少消息ID", nil
	}

	var msg model.MessageModel
	if err := model.GetDB().
		Select("id, channel_id, user_id, whisper_to, is_whisper, is_deleted, is_revoked").
		Where("id = ?", messageID).
		Limit(1).
		Find(&msg).Error; err != nil {
		return nil, nil, 0, "", err
	}
	if msg.ID == "" || msg.IsDeleted || msg.IsRevoked {
		return nil, nil, http.StatusNotFound, "消息不存在", nil
	}

	if _, err := resolveChannelAccess(user.ID, msg.ChannelID); err != nil {
		if err == fiber.ErrForbidden {
			return nil, nil, http.StatusForbidden, "没有访问该频道的权限", nil
		}
		if err == fiber.ErrNotFound {
			return nil, nil, http.StatusNotFound, "频道不存在", nil
		}
		return nil, nil, 0, "", err
	}

	if msg.IsWhisper && !canUserAccessWhisperMessage(user.ID, msg.ChannelID, &msg) {
		return nil, nil, http.StatusForbidden, "没有访问该悄悄话的权限", nil
	}

	channel, err := model.ChannelGet(msg.ChannelID)
	if err != nil {
		return nil, nil, 0, "", err
	}
	if channel.ID == "" {
		return nil, nil, http.StatusNotFound, "频道不存在", nil
	}

	return &msg, channel, 0, "", nil
}

func broadcastMessageReaction(channel *model.ChannelModel, msg *model.MessageModel, user *model.UserModel, summary *service.MessageReactionSummary, action string) {
	if channel == nil || msg == nil || user == nil || summary == nil {
		return
	}

	channelData := channel.ToProtocolType()
	reaction := &protocol.MessageReactionEvent{
		MessageID: msg.ID,
		Emoji:     summary.Emoji,
		Count:     summary.Count,
		Action:    action,
		UserID:    user.ID,
		Timestamp: time.Now().UnixMilli(),
	}
	event := &protocol.Event{
		Type:            protocol.EventMessageReaction,
		Channel:         channelData,
		User:            user.ToProtocolType(),
		MessageReaction: reaction,
	}

	broadcast := &ChatContext{
		User:            user,
		ChannelUsersMap: getChannelUsersMap(),
		UserId2ConnInfo: getUserConnInfoMap(),
	}

	if msg.IsWhisper {
		recipients := make([]string, 0, 4)
		seen := map[string]struct{}{}
		addRecipient := func(id string) {
			if id == "" {
				return
			}
			if _, ok := seen[id]; ok {
				return
			}
			seen[id] = struct{}{}
			recipients = append(recipients, id)
		}
		addRecipient(msg.UserID)
		addRecipient(msg.WhisperTo)
		for _, id := range model.GetWhisperRecipientIDs(msg.ID) {
			addRecipient(id)
		}
		if broadcast.ChannelUsersMap != nil {
			if userSet, ok := broadcast.ChannelUsersMap.Load(msg.ChannelID); ok && userSet != nil {
				userSet.Range(func(id string) bool {
					if canUserReadAllWhispersInChannel(id, msg.ChannelID) {
						addRecipient(id)
					}
					return true
				})
			}
		}
		broadcast.BroadcastEventInChannelToUsers(msg.ChannelID, recipients, event)
		broadcast.BroadcastEventInChannelForBot(msg.ChannelID, event)
		return
	}

	broadcast.BroadcastEventInChannel(msg.ChannelID, event)
	broadcast.BroadcastEventInChannelForBot(msg.ChannelID, event)
}

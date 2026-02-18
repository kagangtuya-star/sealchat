package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

const (
	channelImageLayoutMinSize = 48
	channelImageLayoutMaxSize = 4096
)

type channelImageLayoutDTO struct {
	AttachmentID string `json:"attachmentId"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	UpdatedAt    int64  `json:"updatedAt,omitempty"`
}

type channelImageLayoutSaveRequest struct {
	Items []channelImageLayoutSaveItem `json:"items"`
}

type channelImageLayoutSaveItem struct {
	AttachmentID string `json:"attachmentId"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

func normalizeLayoutAttachmentID(raw string) string {
	id := strings.TrimSpace(raw)
	if strings.HasPrefix(id, "id:") {
		id = strings.TrimPrefix(id, "id:")
	}
	return id
}

func ChannelImageLayoutsGet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}

	if _, err := resolveChannelAccess(user.ID, channelID); err != nil {
		if err == fiber.ErrForbidden {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "没有访问该频道的权限"})
		}
		if err == fiber.ErrNotFound {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	rawAttachmentIDs := parseQueryStringSlice(c, "attachmentIds")
	attachmentIDs := make([]string, 0, len(rawAttachmentIDs))
	seen := map[string]struct{}{}
	for _, raw := range rawAttachmentIDs {
		normalized := normalizeLayoutAttachmentID(raw)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		attachmentIDs = append(attachmentIDs, normalized)
	}

	if len(attachmentIDs) == 0 {
		return c.JSON(fiber.Map{"message": "ok", "items": []channelImageLayoutDTO{}})
	}

	layouts, err := model.ChannelAttachmentImageLayoutBatchGet(channelID, attachmentIDs)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "查询图片布局失败"})
	}

	items := make([]channelImageLayoutDTO, 0, len(layouts))
	for _, layout := range layouts {
		if layout == nil {
			continue
		}
		items = append(items, channelImageLayoutDTO{
			AttachmentID: layout.AttachmentID,
			Width:        layout.Width,
			Height:       layout.Height,
			UpdatedAt:    layout.UpdatedAt.UnixMilli(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].AttachmentID < items[j].AttachmentID
	})

	return c.JSON(fiber.Map{
		"message": "ok",
		"items":   items,
	})
}

func channelMessageCanEditImageLayout(userID string, channel *model.ChannelModel, msg *model.MessageModel) bool {
	if channel == nil || msg == nil {
		return false
	}
	if strings.TrimSpace(userID) == "" {
		return false
	}
	if msg.UserID == userID {
		return true
	}
	if channel.WorldID == "" {
		return false
	}
	world, err := service.GetWorldByID(channel.WorldID)
	if err != nil || world == nil || !world.AllowAdminEditMessages {
		return false
	}
	operatorRank := getChannelMemberRoleRank(channel, channel.ID, userID)
	if operatorRank < channelMemberRoleRankAdmin {
		return false
	}
	if !canModerateTargetByRank(channel, channel.ID, userID, msg.UserID) {
		return false
	}
	return true
}

func ChannelMessageImageLayoutsSave(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	channelID := strings.TrimSpace(c.Params("channelId"))
	messageID := strings.TrimSpace(c.Params("messageId"))
	if channelID == "" || messageID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道或消息ID"})
	}

	if _, err := resolveChannelAccess(user.ID, channelID); err != nil {
		if err == fiber.ErrForbidden {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "没有访问该频道的权限"})
		}
		if err == fiber.ErrNotFound {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	var req channelImageLayoutSaveRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数格式错误"})
	}
	if len(req.Items) == 0 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "至少提交一项图片尺寸"})
	}

	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "读取频道失败"})
	}
	if channel.ID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
	}

	var msg model.MessageModel
	if err := model.GetDB().Where("id = ? AND channel_id = ?", messageID, channelID).Limit(1).Find(&msg).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "读取消息失败"})
	}
	if msg.ID == "" || msg.IsDeleted || msg.IsRevoked {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "消息不存在"})
	}

	if !channelMessageCanEditImageLayout(user.ID, channel, &msg) {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "没有权限调整该消息的图片尺寸"})
	}

	availableAttachmentIDs := extractImageAttachmentIDs(msg.Content)
	availableSet := map[string]struct{}{}
	for _, id := range availableAttachmentIDs {
		if normalized := normalizeLayoutAttachmentID(id); normalized != "" {
			availableSet[normalized] = struct{}{}
		}
	}
	if len(availableSet) == 0 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "目标消息不包含可调整图片"})
	}

	normalizedByAttachment := map[string]channelImageLayoutSaveItem{}
	for _, item := range req.Items {
		attachmentID := normalizeLayoutAttachmentID(item.AttachmentID)
		if attachmentID == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "存在无效附件ID"})
		}
		if _, ok := availableSet[attachmentID]; !ok {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "存在不属于该消息的附件"})
		}
		if item.Width < channelImageLayoutMinSize || item.Width > channelImageLayoutMaxSize {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "图片宽度超出允许范围"})
		}
		if item.Height < channelImageLayoutMinSize || item.Height > channelImageLayoutMaxSize {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "图片高度超出允许范围"})
		}
		normalizedByAttachment[attachmentID] = channelImageLayoutSaveItem{
			AttachmentID: attachmentID,
			Width:        item.Width,
			Height:       item.Height,
		}
	}

	upsertItems := make([]model.ChannelAttachmentImageLayoutUpsertItem, 0, len(normalizedByAttachment))
	attachmentIDs := make([]string, 0, len(normalizedByAttachment))
	for attachmentID, item := range normalizedByAttachment {
		attachmentIDs = append(attachmentIDs, attachmentID)
		upsertItems = append(upsertItems, model.ChannelAttachmentImageLayoutUpsertItem{
			AttachmentID: attachmentID,
			Width:        item.Width,
			Height:       item.Height,
		})
	}
	sort.Strings(attachmentIDs)

	if err := model.ChannelAttachmentImageLayoutUpsertBatch(channelID, user.ID, upsertItems); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "保存图片尺寸失败"})
	}

	layouts, err := model.ChannelAttachmentImageLayoutBatchGet(channelID, attachmentIDs)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "读取已保存图片尺寸失败"})
	}

	respItems := make([]channelImageLayoutDTO, 0, len(layouts))
	eventItems := make([]protocol.ChannelImageLayoutItem, 0, len(layouts))
	for _, layout := range layouts {
		if layout == nil {
			continue
		}
		updatedAt := layout.UpdatedAt.UnixMilli()
		respItems = append(respItems, channelImageLayoutDTO{
			AttachmentID: layout.AttachmentID,
			Width:        layout.Width,
			Height:       layout.Height,
			UpdatedAt:    updatedAt,
		})
		eventItems = append(eventItems, protocol.ChannelImageLayoutItem{
			AttachmentID: layout.AttachmentID,
			Width:        layout.Width,
			Height:       layout.Height,
			UpdatedAt:    updatedAt,
		})
	}

	event := &protocol.Event{
		Type:    protocol.EventChannelImageLayoutUpdated,
		Channel: channel.ToProtocolType(),
		User:    user.ToProtocolType(),
		ChannelImageLayout: &protocol.ChannelImageLayoutEventPayload{
			ChannelID:  channelID,
			MessageID:  msg.ID,
			Items:      eventItems,
			OperatorID: user.ID,
		},
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
			id = strings.TrimSpace(id)
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
		broadcast.BroadcastEventInChannelToUsers(channelID, recipients, event)
		broadcast.BroadcastEventInChannelForBot(channelID, event)
	} else {
		broadcast.BroadcastEventInChannel(channelID, event)
		broadcast.BroadcastEventInChannelForBot(channelID, event)
	}

	return c.JSON(fiber.Map{
		"message":   "ok",
		"channelId": channelID,
		"messageId": msg.ID,
		"items":     respItems,
	})
}

package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

type webhookExternalRef struct {
	Source     string `json:"source"`
	ExternalID string `json:"externalId"`
}

type webhookIdentityPayload struct {
	ExternalActorID    string `json:"externalActorId"`
	DisplayName        string `json:"displayName"`
	Color              string `json:"color"`
	AvatarAttachmentID string `json:"avatarAttachmentId"`
}

type webhookMessagePayload struct {
	MessageID       string   `json:"messageId"`
	Content         string   `json:"content"`
	ICMode          string   `json:"icMode"`
	DisplayOrder    *float64 `json:"displayOrder"`
	QuoteExternalID string   `json:"quoteExternalId"`
	QuoteMessageID  string   `json:"quoteMessageId"`
}

type webhookWriteRequest struct {
	Op             string                  `json:"op"`
	IdempotencyKey string                  `json:"idempotencyKey"`
	ExternalRef    *webhookExternalRef     `json:"externalRef"`
	Identity       *webhookIdentityPayload `json:"identity"`
	Message        *webhookMessagePayload  `json:"message"`
}

type webhookChangeEvent struct {
	Seq     int64             `json:"seq"`
	Type    string            `json:"type"`
	Channel *protocol.Channel `json:"channel,omitempty"`
	Message *protocol.Message `json:"message,omitempty"`
	Origin  *struct {
		IntegrationID string `json:"integrationId,omitempty"`
		Source        string `json:"source,omitempty"`
		ExternalID    string `json:"externalId,omitempty"`
	} `json:"origin,omitempty"`
}

func WebhookChanges(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	integration, err := requireWebhookCapability(c, "read_changes")
	if err != nil {
		return nil
	}

	channel, err2 := model.ChannelGet(channelID)
	if err2 != nil {
		return wrapError(c, err2, "读取频道失败")
	}
	if channel == nil || channel.ID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "not_found", "message": "频道不存在"})
	}
	if strings.EqualFold(channel.PermType, "private") {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "forbidden", "message": "私聊频道不支持 webhook 变更流"})
	}

	cursor := int64(0)
	hasCursor := false
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		if v, e := strconv.ParseInt(raw, 10, 64); e == nil && v >= 0 {
			cursor = v
			hasCursor = true
		} else {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "bad_request", "message": "cursor 解析失败"})
		}
	}
	limit := c.QueryInt("limit", 100)
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	excludeSource := strings.TrimSpace(c.Query("excludeSource"))

	db := model.GetDB()
	if !hasCursor {
		var maxSeq int64
		_ = db.Model(&model.WebhookEventLogModel{}).Where("channel_id = ?", channelID).Select("MAX(seq)").Scan(&maxSeq).Error
		if maxSeq > int64(limit) {
			cursor = maxSeq - int64(limit)
		}
	}
	q := db.Where("channel_id = ? AND seq > ?", channelID, cursor)
	if excludeSource != "" {
		q = q.Where("(source IS NULL OR source = '' OR source <> ?)", excludeSource)
	}

	var logs []model.WebhookEventLogModel
	if err := q.Order("seq ASC").Limit(limit).Find(&logs).Error; err != nil {
		return wrapError(c, err, "读取变更流失败")
	}

	messageIDs := []string{}
	for _, l := range logs {
		if strings.TrimSpace(l.MessageID) != "" {
			messageIDs = append(messageIDs, l.MessageID)
		}
	}

	msgByID := map[string]*model.MessageModel{}
	if len(messageIDs) > 0 {
		var messages []*model.MessageModel
		if err := db.Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, username, nickname, avatar, is_bot")
		}).Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, channel_id, user_id")
		}).Where("id IN ?", messageIDs).Find(&messages).Error; err != nil {
			return wrapError(c, err, "读取消息失败")
		}
		for _, m := range messages {
			msgByID[m.ID] = m
		}
	}

	channelData := channel.ToProtocolType()
	events := make([]*webhookChangeEvent, 0, len(logs))
	nextCursor := cursor

	for _, l := range logs {
		if l.Seq > nextCursor {
			nextCursor = l.Seq
		}

		ev := &webhookChangeEvent{
			Seq:  l.Seq,
			Type: l.Type,
			Origin: func() *struct {
				IntegrationID string `json:"integrationId,omitempty"`
				Source        string `json:"source,omitempty"`
				ExternalID    string `json:"externalId,omitempty"`
			} {
				if strings.TrimSpace(l.IntegrationID) == "" && strings.TrimSpace(l.Source) == "" && strings.TrimSpace(l.ExternalID) == "" {
					return nil
				}
				return &struct {
					IntegrationID string `json:"integrationId,omitempty"`
					Source        string `json:"source,omitempty"`
					ExternalID    string `json:"externalId,omitempty"`
				}{
					IntegrationID: strings.TrimSpace(l.IntegrationID),
					Source:        strings.TrimSpace(l.Source),
					ExternalID:    strings.TrimSpace(l.ExternalID),
				}
			}(),
		}

		msg := msgByID[strings.TrimSpace(l.MessageID)]
		if msg != nil {
			// 默认排除 whisper（未来通过 capability 放开）
			if msg.IsWhisper {
				continue
			}
			if msg.IsRevoked || msg.IsDeleted {
				msg.Content = ""
			}
			msg.EnsureWhisperMeta()
			ev.Channel = channelData
			ev.Message = buildProtocolMessage(msg, channelData)
			// BOT 出站：Satori XML 转换为 CQ 码
			if ev.Message != nil {
				ev.Message.Content = service.ConvertSatoriToCQ(ev.Message.Content)
			}
		}

		events = append(events, ev)
	}

	return c.JSON(fiber.Map{
		"channelId":  channelID,
		"cursor":     strconv.FormatInt(cursor, 10),
		"nextCursor": strconv.FormatInt(nextCursor, 10),
		"serverTime": time.Now().UnixMilli(),
		"events":     events,
		"integration": fiber.Map{
			"id":     integration.ID,
			"source": integration.Source,
		},
	})
}

func WebhookMessages(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	integration := getWebhookIntegration(c)
	if integration == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized", "message": "missing integration"})
	}
	userAny := c.Locals("user")
	botUser, _ := userAny.(*model.UserModel)
	if botUser == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized", "message": "missing bot user"})
	}

	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return wrapError(c, err, "读取频道失败")
	}
	if channel == nil || channel.ID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "not_found", "message": "频道不存在"})
	}
	if strings.EqualFold(channel.PermType, "private") {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "forbidden", "message": "私聊频道不支持 webhook 写入"})
	}

	var req webhookWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "请求参数错误"})
	}
	op := strings.TrimSpace(req.Op)
	if op == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "op 不能为空"})
	}

	switch op {
	case "message.upsert":
		if _, err := requireWebhookCapability(c, "write_create"); err != nil {
			return nil
		}
		if _, err := requireWebhookCapability(c, "write_update_own"); err != nil {
			return nil
		}
		return webhookMessageUpsert(c, integration, botUser, channel, &req)
	case "message.create":
		if _, err := requireWebhookCapability(c, "write_create"); err != nil {
			return nil
		}
		return webhookMessageCreate(c, integration, botUser, channel, &req)
	case "message.update":
		if _, err := requireWebhookCapability(c, "write_update_own"); err != nil {
			return nil
		}
		return webhookMessageUpdate(c, integration, botUser, channel, &req)
	case "message.delete":
		if _, err := requireWebhookCapability(c, "write_delete_own"); err != nil {
			return nil
		}
		return webhookMessageDelete(c, integration, botUser, channel, &req)
	default:
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "未知 op"})
	}
}

func webhookResolveExternalRef(req *webhookWriteRequest) (source, externalID string) {
	if req == nil || req.ExternalRef == nil {
		return "", ""
	}
	source = strings.TrimSpace(req.ExternalRef.Source)
	externalID = strings.TrimSpace(req.ExternalRef.ExternalID)
	return source, externalID
}

func webhookResolveIdentityID(integration *model.ChannelWebhookIntegrationModel, botUser *model.UserModel, channelID string, idp *webhookIdentityPayload) (string, string, error) {
	if idp == nil {
		return "", "", nil
	}
	if integration == nil || botUser == nil {
		return "", "", nil
	}
	if !integration.HasCapability("identity_upsert") {
		return "", "", fiber.NewError(http.StatusForbidden, "capability required: identity_upsert")
	}
	source := strings.TrimSpace(integration.Source)
	if source == "" {
		source = "external"
	}
	externalActorID := strings.TrimSpace(idp.ExternalActorID)
	if externalActorID == "" {
		return "", "", nil
	}

	displayName := strings.TrimSpace(idp.DisplayName)
	if displayName == "" {
		return "", "", fiber.NewError(http.StatusBadRequest, "identity.displayName 不能为空")
	}

	input := &service.ChannelIdentityInput{
		ChannelID:          channelID,
		DisplayName:        displayName,
		Color:              strings.TrimSpace(idp.Color),
		AvatarAttachmentID: strings.TrimSpace(idp.AvatarAttachmentID),
		IsDefault:          false,
		FolderIDs:          nil,
	}

	existing, err := model.WebhookIdentityBindingGet(integration.ID, source, externalActorID)
	if err != nil {
		return "", "", err
	}

	if existing != nil && strings.TrimSpace(existing.IdentityID) != "" {
		updated, err := service.ChannelIdentityUpdate(botUser.ID, existing.IdentityID, input)
		if err != nil {
			return "", "", err
		}
		_, _ = model.WebhookIdentityBindingUpsert(channelID, integration.ID, botUser.ID, source, externalActorID, updated.ID)
		return updated.ID, externalActorID, nil
	}

	created, err := service.ChannelIdentityCreate(botUser.ID, input)
	if err != nil {
		return "", "", err
	}
	_, _ = model.WebhookIdentityBindingUpsert(channelID, integration.ID, botUser.ID, source, externalActorID, created.ID)
	return created.ID, externalActorID, nil
}

func webhookResolveQuoteID(channelID, source, quoteExternalID, quoteMessageID string) (string, error) {
	quoteMessageID = strings.TrimSpace(quoteMessageID)
	if quoteMessageID != "" {
		return quoteMessageID, nil
	}
	quoteExternalID = strings.TrimSpace(quoteExternalID)
	if quoteExternalID == "" || strings.TrimSpace(source) == "" {
		return "", nil
	}
	ref, err := model.MessageExternalRefGet(channelID, source, quoteExternalID)
	if err != nil {
		return "", err
	}
	if ref == nil {
		return "", nil
	}
	return strings.TrimSpace(ref.MessageID), nil
}

func webhookMessageUpsert(c *fiber.Ctx, integration *model.ChannelWebhookIntegrationModel, botUser *model.UserModel, channel *model.ChannelModel, req *webhookWriteRequest) error {
	source, externalID := webhookResolveExternalRef(req)
	if source == "" || externalID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "externalRef.source/externalRef.externalId 不能为空"})
	}
	if req.Message == nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "message 不能为空"})
	}

	existing, err := model.MessageExternalRefGet(channel.ID, source, externalID)
	if err != nil {
		return wrapError(c, err, "读取 externalRef 失败")
	}
	if existing != nil {
		// 只允许操作本 integration 创建的消息（默认策略）
		if strings.TrimSpace(existing.IntegrationID) != "" && existing.IntegrationID != integration.ID {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"ok": false, "error": "forbidden", "message": "externalRef 属于其他授权"})
		}
		req.Message.MessageID = existing.MessageID
		return webhookMessageUpdate(c, integration, botUser, channel, req)
	}
	return webhookMessageCreate(c, integration, botUser, channel, req)
}

func webhookMessageCreate(c *fiber.Ctx, integration *model.ChannelWebhookIntegrationModel, botUser *model.UserModel, channel *model.ChannelModel, req *webhookWriteRequest) error {
	if req == nil || req.Message == nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "message 不能为空"})
	}
	content := strings.TrimSpace(req.Message.Content)
	if content == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "message.content 不能为空"})
	}

	// BOT 入站：CQ 码转换为 Satori XML
	content = service.ConvertCQToSatori(content)
	content = protocol.EscapeSatoriText(content)

	icMode := strings.ToLower(strings.TrimSpace(req.Message.ICMode))
	if icMode == "" {
		icMode = "ic"
	}
	if icMode != "ic" && icMode != "ooc" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "message.icMode 仅支持 ic/ooc"})
	}

	source, externalID := webhookResolveExternalRef(req)

	identityID := ""
	externalActorID := ""
	if req.Identity != nil {
		id, actorID, err := webhookResolveIdentityID(integration, botUser, channel.ID, req.Identity)
		if err != nil {
			if fe, ok := err.(*fiber.Error); ok {
				errType := "bad_request"
				if fe.Code == http.StatusForbidden {
					errType = "forbidden"
				} else if fe.Code == http.StatusNotFound {
					errType = "not_found"
				}
				return c.Status(fe.Code).JSON(fiber.Map{"ok": false, "error": errType, "message": fe.Message})
			}
			return wrapError(c, err, "处理身份失败")
		}
		identityID = id
		externalActorID = actorID
	}

	quoteID, err := webhookResolveQuoteID(channel.ID, source, req.Message.QuoteExternalID, req.Message.QuoteMessageID)
	if err != nil {
		return wrapError(c, err, "解析引用消息失败")
	}

	member, err := model.MemberGetByUserIDAndChannelIDBase(botUser.ID, channel.ID, botUser.Nickname, true)
	if err != nil {
		return wrapError(c, err, "创建频道成员失败")
	}

	now := time.Now()
	displayOrder := float64(now.UnixMilli())
	if req.Message.DisplayOrder != nil && *req.Message.DisplayOrder > 0 {
		displayOrder = *req.Message.DisplayOrder
	}

	var renderResult *service.DiceRenderResult
	if channel.BuiltInDiceEnabled {
		renderResult, err = service.RenderDiceContent(content, channel.DefaultDiceExpr, nil)
		if err != nil {
			return wrapError(c, err, "渲染骰点失败")
		}
		if renderResult != nil {
			content = renderResult.Content
		}
	}

	msg := &model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
		UserID:            botUser.ID,
		ChannelID:         channel.ID,
		MemberID:          member.ID,
		QuoteID:           quoteID,
		Content:           content,
		DisplayOrder:      displayOrder,
		ICMode:            icMode,
		SenderMemberName:  member.Nickname,
	}

	if identityID != "" {
		identity, err := service.ChannelIdentityValidateMessageIdentity(botUser.ID, channel.ID, identityID)
		if err != nil {
			return wrapError(c, err, "身份校验失败")
		}
		if identity != nil {
			msg.SenderRoleID = identity.ID
			msg.SenderIdentityID = identity.ID
			msg.SenderIdentityName = identity.DisplayName
			msg.SenderIdentityColor = identity.Color
			msg.SenderIdentityAvatarID = identity.AvatarAttachmentID
			if identity.DisplayName != "" {
				msg.SenderMemberName = identity.DisplayName
			}
		}
	}
	if identityID == "" && botUser.IsBot && strings.TrimSpace(botUser.NickColor) != "" {
		msg.SenderIdentityColor = strings.TrimSpace(botUser.NickColor)
	}

	db := model.GetDB()
	if err := db.Create(msg).Error; err != nil {
		return wrapError(c, err, "创建消息失败")
	}
	if renderResult != nil {
		_ = model.MessageDiceRollReplace(msg.ID, renderResult.Rolls)
	}

	if source != "" && externalID != "" {
		_, _ = model.MessageExternalRefUpsert(channel.ID, source, externalID, msg.ID, integration.ID, externalActorID)
	}
	_ = model.WebhookEventLogAppendForMessage(channel.ID, "message-created", msg.ID)

	// 广播（复用现有 WS 广播通道）
	channelData := channel.ToProtocolType()
	msg.User = botUser
	msg.Member = member
	if msg.QuoteID != "" {
		var quote model.MessageModel
		_ = db.Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, username, nickname, avatar, is_bot")
		}).Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, channel_id, user_id")
		}).Where("id = ?", msg.QuoteID).Limit(1).Find(&quote).Error
		if quote.ID != "" {
			if quote.IsRevoked || quote.IsDeleted {
				quote.Content = ""
			}
			msg.Quote = &quote
		}
	}
	messageData := buildProtocolMessage(msg, channelData)
	ev := &protocol.Event{
		Type:    protocol.EventMessageCreated,
		Message: messageData,
		Channel: channelData,
		User:    botUser.ToProtocolType(),
	}
	broadcast := &ChatContext{
		User:            botUser,
		ChannelUsersMap: getChannelUsersMap(),
		UserId2ConnInfo: getUserConnInfoMap(),
	}
	broadcast.BroadcastEventInChannel(channel.ID, ev)
	broadcast.BroadcastEventInChannelForBot(channel.ID, ev)

	channel.UpdateRecentSent()
	member.UpdateRecentSent()

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"ok": true,
		"result": fiber.Map{
			"messageId": msg.ID,
			"created":   true,
			"updated":   false,
		},
	})
}

func webhookMessageUpdate(c *fiber.Ctx, integration *model.ChannelWebhookIntegrationModel, botUser *model.UserModel, channel *model.ChannelModel, req *webhookWriteRequest) error {
	if req == nil || req.Message == nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "message 不能为空"})
	}

	messageID := strings.TrimSpace(req.Message.MessageID)
	source, externalID := webhookResolveExternalRef(req)
	if messageID == "" && source != "" && externalID != "" {
		ref, err := model.MessageExternalRefGet(channel.ID, source, externalID)
		if err != nil {
			return wrapError(c, err, "读取 externalRef 失败")
		}
		if ref == nil || strings.TrimSpace(ref.MessageID) == "" {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"ok": false, "error": "not_found", "message": "消息不存在"})
		}
		if strings.TrimSpace(ref.IntegrationID) != "" && ref.IntegrationID != integration.ID {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"ok": false, "error": "forbidden", "message": "externalRef 属于其他授权"})
		}
		messageID = ref.MessageID
	}
	if messageID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "messageId 或 externalRef 必填其一"})
	}

	// 预校验：仅允许操作自己的消息（默认策略）
	var msg model.MessageModel
	if err := model.GetDB().Select("id, user_id, is_deleted, is_revoked").Where("id = ? AND channel_id = ?", messageID, channel.ID).Limit(1).Find(&msg).Error; err != nil {
		return wrapError(c, err, "读取消息失败")
	}
	if msg.ID == "" || msg.IsDeleted || msg.IsRevoked {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"ok": false, "error": "not_found", "message": "消息不存在或已删除"})
	}
	if msg.UserID != botUser.ID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"ok": false, "error": "forbidden", "message": "仅允许编辑自身消息"})
	}

	identityID := (*string)(nil)
	if req.Identity != nil {
		id, _, err := webhookResolveIdentityID(integration, botUser, channel.ID, req.Identity)
		if err != nil {
			if fe, ok := err.(*fiber.Error); ok {
				errType := "bad_request"
				if fe.Code == http.StatusForbidden {
					errType = "forbidden"
				} else if fe.Code == http.StatusNotFound {
					errType = "not_found"
				}
				return c.Status(fe.Code).JSON(fiber.Map{"ok": false, "error": errType, "message": fe.Message})
			}
			return wrapError(c, err, "处理身份失败")
		}
		trimmed := strings.TrimSpace(id)
		identityID = &trimmed
	}

	ctx := &ChatContext{
		User:            botUser,
		ChannelUsersMap: getChannelUsersMap(),
		UserId2ConnInfo: getUserConnInfoMap(),
	}
	data := &struct {
		ChannelID  string  `json:"channel_id"`
		MessageID  string  `json:"message_id"`
		Content    string  `json:"content"`
		ICMode     string  `json:"ic_mode"`
		IdentityID *string `json:"identity_id"`
	}{
		ChannelID:  channel.ID,
		MessageID:  messageID,
		Content:    req.Message.Content,
		ICMode:     req.Message.ICMode,
		IdentityID: identityID,
	}
	_, err := apiMessageUpdate(ctx, data)
	if err != nil {
		return wrapError(c, err, "更新消息失败")
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"ok": true,
		"result": fiber.Map{
			"messageId": messageID,
			"created":   false,
			"updated":   true,
		},
	})
}

func webhookMessageDelete(c *fiber.Ctx, integration *model.ChannelWebhookIntegrationModel, botUser *model.UserModel, channel *model.ChannelModel, req *webhookWriteRequest) error {
	messageID := ""
	if req != nil && req.Message != nil {
		messageID = strings.TrimSpace(req.Message.MessageID)
	}
	source, externalID := webhookResolveExternalRef(req)
	if messageID == "" && source != "" && externalID != "" {
		ref, err := model.MessageExternalRefGet(channel.ID, source, externalID)
		if err != nil {
			return wrapError(c, err, "读取 externalRef 失败")
		}
		if ref == nil || strings.TrimSpace(ref.MessageID) == "" {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"ok": false, "error": "not_found", "message": "消息不存在"})
		}
		if strings.TrimSpace(ref.IntegrationID) != "" && ref.IntegrationID != integration.ID {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"ok": false, "error": "forbidden", "message": "externalRef 属于其他授权"})
		}
		messageID = ref.MessageID
	}
	if messageID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "bad_request", "message": "messageId 或 externalRef 必填其一"})
	}

	var msg model.MessageModel
	if err := model.GetDB().Select("id, user_id, is_deleted, is_revoked").Where("id = ? AND channel_id = ?", messageID, channel.ID).Limit(1).Find(&msg).Error; err != nil {
		return wrapError(c, err, "读取消息失败")
	}
	if msg.ID == "" || msg.IsDeleted {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"ok": false, "error": "not_found", "message": "消息不存在或已删除"})
	}
	if msg.UserID != botUser.ID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"ok": false, "error": "forbidden", "message": "仅允许删除自身消息"})
	}

	ctx := &ChatContext{
		User:            botUser,
		ChannelUsersMap: getChannelUsersMap(),
		UserId2ConnInfo: getUserConnInfoMap(),
	}
	data := &struct {
		ChannelID string `json:"channel_id"`
		MessageID string `json:"message_id"`
	}{
		ChannelID: channel.ID,
		MessageID: messageID,
	}
	_, err := apiMessageRemove(ctx, data)
	if err != nil {
		return wrapError(c, err, "删除消息失败")
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"ok": true,
		"result": fiber.Map{
			"messageId": messageID,
			"deleted":   true,
		},
	})
}

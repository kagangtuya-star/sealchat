package api

import (
	"fmt"
	"math"
	"net/http"
	"sealchat/service"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	ds "github.com/sealdice/dicescript"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/utils"
)

const (
	displayOrderGap     = 1024.0
	displayOrderEpsilon = 1e-6
)

func canReorderAllMessages(userID string, channel *model.ChannelModel) bool {
	if channel == nil {
		return false
	}
	if channel.UserID == userID {
		return true
	}
	if pm.CanWithSystemRole(userID, pm.PermModAdmin) {
		return true
	}
	if pm.CanWithChannelRole(userID, channel.ID,
		pm.PermFuncChannelManageInfo,
		pm.PermFuncChannelManageRole,
		pm.PermFuncChannelRoleLinkRoot,
		pm.PermFuncChannelRoleUnlinkRoot,
		pm.PermFuncChannelMemberRemove,
	) {
		return true
	}
	return false
}

func apiMessageDelete(ctx *ChatContext, data *struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
}) (any, error) {
	db := model.GetDB()
	item := model.MessageModel{}
	db.Where("channel_id = ? and id = ?", data.ChannelID, data.MessageID).Limit(1).Find(&item)
	if item.ID != "" {
		if item.UserID != ctx.User.ID {
			return nil, nil // 失败了
		}

		item.IsRevoked = true
		db.Model(&item).Update("is_revoked", true)

		var channel model.ChannelModel
		db.Where("id = ?", data.ChannelID).Limit(1).Find(&channel)
		if channel.ID == "" {
			return nil, nil
		}
		channelData := channel.ToProtocolType()

		ctx.BroadcastEvent(&protocol.Event{
			// 协议规定: 事件中必须含有 channel，message，user
			Type:    protocol.EventMessageDeleted,
			Message: item.ToProtocolType2(channelData),
			Channel: channelData,
			User:    ctx.User.ToProtocolType(),
		})

		return &struct {
			Success bool `json:"success"`
		}{Success: true}, nil
	}

	return nil, nil
}

func apiMessageCreate(ctx *ChatContext, data *struct {
	ChannelID  string `json:"channel_id"`
	QuoteID    string `json:"quote_id"`
	Content    string `json:"content"`
	WhisperTo  string `json:"whisper_to"`
	ClientID   string `json:"client_id"`
	IdentityID string `json:"identity_id"`
}) (any, error) {
	echo := ctx.Echo
	db := model.GetDB()
	channelId := data.ChannelID

	var privateOtherUser string

	// 权限检查
	if len(channelId) < 30 { // 注意，这不是一个好的区分方式
		// 群内
		if !pm.CanWithChannelRole(ctx.User.ID, channelId, pm.PermFuncChannelTextSend, pm.PermFuncChannelTextSendAll) {
			return nil, nil
		}
	} else {
		// 好友/陌生人
		fr, _ := model.FriendRelationGetByID(channelId)
		if fr.ID == "" {
			return nil, nil
		}

		privateOtherUser = fr.UserID1
		if fr.UserID1 == ctx.User.ID {
			privateOtherUser = fr.UserID2
		}
	}

	content := data.Content
	member, err := model.MemberGetByUserIDAndChannelID(ctx.User.ID, data.ChannelID, ctx.User.Nickname)
	if err != nil {
		return nil, err
	}

	identity, err := service.ChannelIdentityValidateMessageIdentity(ctx.User.ID, data.ChannelID, data.IdentityID)
	if err != nil {
		return nil, err
	}

	channel, _ := model.ChannelGet(channelId)
	if channel.ID == "" {
		return nil, nil
	}
	channelData := channel.ToProtocolType()

	var whisperUser *model.UserModel
	if data.WhisperTo != "" {
		if data.WhisperTo == ctx.User.ID {
			return nil, nil
		}
		if len(channelId) < 30 {
			targetMember, _ := model.MemberGetByUserIDAndChannelIDBase(data.WhisperTo, channelId, "", false)
			if targetMember == nil {
				return nil, nil
			}
		} else {
			if data.WhisperTo != privateOtherUser {
				return nil, nil
			}
		}
		whisperUser = model.UserGet(data.WhisperTo)
		if whisperUser == nil {
			return nil, nil
		}
	}

	var quote model.MessageModel
	if data.QuoteID != "" {
		db.Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, username, avatar, is_bot")
		}).Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, channel_id, user_id")
		}).Where("id = ?", data.QuoteID).Limit(1).Find(&quote)
		if quote.ID == "" {
			return nil, nil
		}
		if quote.WhisperTo != "" {
			quote.WhisperTarget = model.UserGet(quote.WhisperTo)
		}
	}

	m := model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID: utils.NewID(),
		},
		UserID:       ctx.User.ID,
		ChannelID:    data.ChannelID,
		MemberID:     member.ID,
		QuoteID:      data.QuoteID,
		Content:      content,
		DisplayOrder: float64(time.Now().UnixMilli()),

		SenderMemberName: member.Nickname,
		IsWhisper:        whisperUser != nil,
		WhisperTo:        data.WhisperTo,
	}
	if identity != nil {
		m.SenderIdentityID = identity.ID
		m.SenderIdentityName = identity.DisplayName
		m.SenderIdentityColor = identity.Color
		m.SenderIdentityAvatarID = identity.AvatarAttachmentID
		if identity.DisplayName != "" {
			m.SenderMemberName = identity.DisplayName
		}
	}
	if whisperUser != nil {
		m.WhisperTarget = whisperUser
	}
	rows := db.Create(&m).RowsAffected

	if rows > 0 {
		ctx.TagCheck(data.ChannelID, m.ID, content)
		member.UpdateRecentSent()

		userData := ctx.User.ToProtocolType()

		messageData := m.ToProtocolType2(channelData)
		messageData.Content = content
		messageData.User = userData
		messageData.Member = member.ToProtocolType()
		messageData.ClientID = data.ClientID
		if quote.ID != "" {
			qData := quote.ToProtocolType2(channelData)
			qData.Content = quote.Content
			if quote.User != nil {
				qData.User = quote.User.ToProtocolType()
			}
			if quote.Member != nil {
				qData.Member = quote.Member.ToProtocolType()
			}
			if quote.WhisperTarget != nil {
				qData.WhisperTo = quote.WhisperTarget.ToProtocolType()
			}
			messageData.Quote = qData
		} else {
			messageData.Quote = nil
		}
		if whisperUser != nil {
			messageData.WhisperTo = whisperUser.ToProtocolType()
		}

		// 发出广播事件
		ev := &protocol.Event{
			// 协议规定: 事件中必须含有 channel，message，user
			Type:    protocol.EventMessageCreated,
			Message: messageData,
			Channel: channelData,
			User:    userData,
		}

		if whisperUser != nil {
			recipients := lo.Uniq([]string{ctx.User.ID, whisperUser.ID})
			ctx.BroadcastEventInChannelToUsers(data.ChannelID, recipients, ev)
		} else {
			ctx.BroadcastEventInChannel(data.ChannelID, ev)
			ctx.BroadcastEventInChannelForBot(data.ChannelID, ev)
		}

		if appConfig.BuiltInSealBotEnable && whisperUser == nil {
			botReq := &struct {
				ChannelID string `json:"channel_id"`
				QuoteID   string `json:"quote_id"`
				Content   string `json:"content"`
				WhisperTo string `json:"whisper_to"`
				ClientID  string `json:"client_id"`
			}{
				ChannelID: data.ChannelID,
				QuoteID:   data.QuoteID,
				Content:   data.Content,
				WhisperTo: data.WhisperTo,
				ClientID:  data.ClientID,
			}
			builtinSealBotSolve(ctx, botReq, channelData)
		}

		if channel.PermType == "private" {
			model.FriendRelationSetVisibleById(channel.ID)
		}

		noticePayload := map[string]any{
			"op":        0,
			"type":      "message-created-notice",
			"channelId": data.ChannelID,
		}

		if whisperUser != nil {
			targets := lo.Uniq([]string{data.WhisperTo})
			for _, uid := range targets {
				if uid == "" || uid == ctx.User.ID {
					continue
				}
				_ = model.ChannelReadInit(data.ChannelID, uid)
				ctx.BroadcastToUserJSON(uid, noticePayload)
			}
		} else if channel.PermType == "private" {
			if privateOtherUser != "" {
				_ = model.ChannelReadInit(data.ChannelID, privateOtherUser)
				ctx.BroadcastToUserJSON(privateOtherUser, noticePayload)
			}
		} else {
			// 给当前在线人都通知一遍
			var uids []string
			ctx.UserId2ConnInfo.Range(func(key string, value *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
				uids = append(uids, key)
				return true
			})

			// 找出当前频道在线的人
			var uidsOnline []string
			if x, exists := ctx.ChannelUsersMap.Load(data.ChannelID); exists {
				x.Range(func(key string) bool {
					uidsOnline = append(uidsOnline, key)
					return true
				})
			}

			_ = model.ChannelReadInitInBatches(data.ChannelID, uids)
			_ = model.ChannelReadSetInBatch([]string{data.ChannelID}, uidsOnline)

			// 发送快速更新通知
			ctx.BroadcastJSON(noticePayload, uidsOnline)
		}

		return messageData, nil
	}

	return &struct {
		ErrStatus int    `json:"errStatus"`
		Echo      string `json:"echo"`
	}{
		ErrStatus: http.StatusInternalServerError,
		Echo:      echo,
	}, nil
}

func apiMessageList(ctx *ChatContext, data *struct {
	ChannelID string `json:"channel_id"`
	Next      string `json:"next"`

	// 以下两个字段用于查询某个时间段内的消息，可选
	Type     string `json:"type"` // 查询类型，不填为默认，若time则用下面两个值
	FromTime int64  `json:"from_time"`
	ToTime   int64  `json:"to_time"`
}) (any, error) {
	db := model.GetDB()

	// 权限检查
	channelId := data.ChannelID
	if len(channelId) < 30 { // 注意，这不是一个好的区分方式
		// 群内
		if !pm.CanWithChannelRole(ctx.User.ID, channelId, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
			return nil, nil
		}
	} else {
		// 好友/陌生人
		fr, _ := model.FriendRelationGetByID(channelId)
		if fr.ID == "" {
			return nil, nil
		}
	}

	var items []*model.MessageModel
	q := db.Where("channel_id = ?", data.ChannelID)
	q = q.Where("(is_whisper = ? OR user_id = ? OR whisper_to = ?)", false, ctx.User.ID, ctx.User.ID)

	if data.Type == "time" {
		// 如果有这俩，附加一个条件
		if data.FromTime > 0 {
			q = q.Where("created_at >= ?", time.UnixMilli(data.FromTime))
		}
		if data.ToTime > 0 {
			q = q.Where("created_at <= ?", time.UnixMilli(data.ToTime))
		}
	}

	var count int64
	var cursorOrder float64
	var cursorTime time.Time
	var cursorID string
	var hasCursor bool
	channel, _ := model.ChannelGet(data.ChannelID)
	canReorderAll := canReorderAllMessages(ctx.User.ID, channel)
	if data.Next != "" {
		if strings.Contains(data.Next, "|") {
			parts := strings.SplitN(data.Next, "|", 3)
			if len(parts) == 3 {
				if order, err := strconv.ParseFloat(parts[0], 64); err == nil {
					if ts, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						cursorOrder = order
						cursorTime = time.UnixMilli(ts)
						cursorID = parts[2]
						hasCursor = true
					}
				}
			}
		}
		if !hasCursor {
			t, err := strconv.ParseInt(data.Next, 36, 64)
			if err != nil {
				return nil, err
			}
			cursorOrder = float64(t)
			cursorTime = time.UnixMilli(t)
			hasCursor = true
		}

		if hasCursor {
			cond := "(display_order < ?) OR (display_order = ? AND created_at < ?)"
			args := []interface{}{cursorOrder, cursorOrder, cursorTime}
			if cursorID != "" {
				cond += " OR (display_order = ? AND created_at = ? AND id < ?)"
				args = append(args, cursorOrder, cursorTime, cursorID)
			}
			q = q.Where(cond, args...)
		}
	}

	q.Order("display_order desc").
		Order("created_at desc").
		Order("id desc").
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, username, nickname, avatar, is_bot")
		}).
		Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, channel_id")
		}).Limit(30).Find(&items)

	utils.QueryOneToManyMap(model.GetDB(), items, func(i *model.MessageModel) []string {
		return []string{i.QuoteID}
	}, func(i *model.MessageModel, x []*model.MessageModel) {
		i.Quote = x[0]
	}, "id, content, created_at, user_id, is_revoked")

	_ = model.ChannelReadSet(data.ChannelID, ctx.User.ID)

	q.Count(&count)
	var next string

	items = lo.Reverse(items)
	if count > int64(len(items)) && len(items) > 0 {
		orderStr := strconv.FormatFloat(items[0].DisplayOrder, 'f', 8, 64)
		timeStr := strconv.FormatInt(items[0].CreatedAt.UnixMilli(), 10)
		next = fmt.Sprintf("%s|%s|%s", orderStr, timeStr, items[0].ID)
	}

	whisperIdSet := map[string]struct{}{}
	for _, i := range items {
		if i.IsRevoked {
			i.Content = ""
		}
		if i.WhisperTo != "" {
			whisperIdSet[i.WhisperTo] = struct{}{}
		}
		if i.Quote != nil {
			if i.Quote.IsRevoked {
				i.Quote.Content = ""
			}
			if i.Quote.WhisperTo != "" {
				whisperIdSet[i.Quote.WhisperTo] = struct{}{}
			}
		}
	}

	if len(whisperIdSet) > 0 {
		var ids []string
		for id := range whisperIdSet {
			ids = append(ids, id)
		}
		var whisperUsers []*model.UserModel
		if len(ids) > 0 {
			model.GetDB().Where("id in ?", ids).Find(&whisperUsers)
		}
		id2User := map[string]*model.UserModel{}
		for _, u := range whisperUsers {
			id2User[u.ID] = u
		}
		for _, i := range items {
			if user, ok := id2User[i.WhisperTo]; ok {
				i.WhisperTarget = user
			}
			if i.Quote != nil {
				if user, ok := id2User[i.Quote.WhisperTo]; ok {
					i.Quote.WhisperTarget = user
				}
			}
		}
	}

	for _, i := range items {
		if i.IsWhisper && i.UserID != ctx.User.ID && i.WhisperTo != ctx.User.ID {
			// 理论上不会出现，因为已经过滤，但保险起见
			i.Content = ""
		}
		if i.Quote != nil && i.Quote.IsWhisper && i.Quote.UserID != ctx.User.ID && i.Quote.WhisperTo != ctx.User.ID {
			i.Quote.Content = ""
			i.Quote.WhisperTarget = nil
		}
	}

	return &struct {
		Data          []*model.MessageModel `json:"data"`
		Next          string                `json:"next"`
		CanReorderAll bool                  `json:"can_reorder_all"`
	}{
		Data:          items,
		Next:          next,
		CanReorderAll: canReorderAll,
	}, nil
}

func apiMessageUpdate(ctx *ChatContext, data *struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
}) (any, error) {
	if strings.TrimSpace(data.Content) == "" {
		return nil, fmt.Errorf("消息内容不能为空")
	}

	db := model.GetDB()

	var msg model.MessageModel
	db.Where("id = ? AND channel_id = ?", data.MessageID, data.ChannelID).Limit(1).Find(&msg)
	if msg.ID == "" {
		return nil, nil
	}
	if msg.UserID != ctx.User.ID {
		return nil, nil
	}
	if msg.IsRevoked {
		return nil, nil
	}

	channel, _ := model.ChannelGet(data.ChannelID)
	if channel.ID == "" {
		return nil, nil
	}
	channelData := channel.ToProtocolType()

	member, _ := model.MemberGetByUserIDAndChannelID(ctx.User.ID, data.ChannelID, ctx.User.Nickname)

	var quote model.MessageModel
	if msg.QuoteID != "" {
		db.Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, username, avatar, is_bot")
		}).Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, channel_id, user_id")
		}).Where("id = ?", msg.QuoteID).Limit(1).Find(&quote)
		if quote.WhisperTo != "" {
			quote.WhisperTarget = model.UserGet(quote.WhisperTo)
		}
	}

	if msg.WhisperTo != "" {
		msg.WhisperTarget = model.UserGet(msg.WhisperTo)
	}

	buildMessage := func() *protocol.Message {
		messageData := msg.ToProtocolType2(channelData)
		messageData.Content = msg.Content
		messageData.User = ctx.User.ToProtocolType()
		if member != nil {
			messageData.Member = member.ToProtocolType()
		}
		if msg.WhisperTarget != nil {
			messageData.WhisperTo = msg.WhisperTarget.ToProtocolType()
		}
		if quote.ID != "" {
			qData := quote.ToProtocolType2(channelData)
			qData.Content = quote.Content
			if quote.User != nil {
				qData.User = quote.User.ToProtocolType()
			}
			if quote.Member != nil {
				qData.Member = quote.Member.ToProtocolType()
			}
			if quote.WhisperTarget != nil {
				qData.WhisperTo = quote.WhisperTarget.ToProtocolType()
			}
			messageData.Quote = qData
		}
		return messageData
	}

	prevContent := msg.Content
	if prevContent == data.Content {
		return &struct {
			Message *protocol.Message `json:"message"`
		}{Message: buildMessage()}, nil
	}

	history := model.MessageEditHistoryModel{
		MessageID:    msg.ID,
		EditorID:     ctx.User.ID,
		PrevContent:  prevContent,
		ChannelID:    msg.ChannelID,
		EditedUserID: msg.UserID,
	}
	db.Create(&history)

	msg.Content = data.Content
	msg.IsEdited = true
	msg.EditCount = msg.EditCount + 1
	msg.UpdatedAt = time.Now()
	err := db.Model(&model.MessageModel{}).Where("id = ?", msg.ID).Updates(map[string]any{
		"content":    msg.Content,
		"is_edited":  msg.IsEdited,
		"edit_count": msg.EditCount,
		"updated_at": msg.UpdatedAt,
	}).Error
	if err != nil {
		return nil, err
	}

	messageData := buildMessage()

	ev := &protocol.Event{
		Type:    protocol.EventMessageUpdated,
		Message: messageData,
		Channel: channelData,
		User:    messageData.User,
	}

	if msg.IsWhisper && msg.WhisperTo != "" {
		recipients := lo.Uniq([]string{ctx.User.ID, msg.WhisperTo})
		ctx.BroadcastEventInChannelToUsers(data.ChannelID, recipients, ev)
	} else {
		ctx.BroadcastEventInChannel(data.ChannelID, ev)
		ctx.BroadcastEventInChannelForBot(data.ChannelID, ev)
	}

	return &struct {
		Message *protocol.Message `json:"message"`
	}{Message: messageData}, nil
}

func apiMessageReorder(ctx *ChatContext, data *struct {
	ChannelID  string `json:"channel_id"`
	MessageID  string `json:"message_id"`
	BeforeID   string `json:"before_id"`
	AfterID    string `json:"after_id"`
	ClientOpID string `json:"client_op_id"`
}) (any, error) {
	if strings.TrimSpace(data.ChannelID) == "" || strings.TrimSpace(data.MessageID) == "" {
		return nil, fmt.Errorf("缺少必要参数")
	}

	db := model.GetDB()

	var msg model.MessageModel
	err := db.Preload("User", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, username, nickname, avatar, is_bot")
	}).Preload("Member", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, nickname, channel_id, user_id")
	}).Where("id = ? AND channel_id = ?", data.MessageID, data.ChannelID).Limit(1).Find(&msg).Error
	if err != nil {
		return nil, err
	}
	if msg.ID == "" {
		return nil, nil
	}

	channel, _ := model.ChannelGet(data.ChannelID)
	if channel.ID == "" {
		return nil, nil
	}

	if !canReorderAllMessages(ctx.User.ID, channel) && msg.UserID != ctx.User.ID {
		return nil, fmt.Errorf("您没有权限调整该消息的位置")
	}

	if strings.TrimSpace(data.BeforeID) == "" && strings.TrimSpace(data.AfterID) == "" {
		return nil, fmt.Errorf("缺少目标位置参数")
	}

	var beforeMsg, afterMsg model.MessageModel
	if data.BeforeID != "" && data.BeforeID != data.MessageID {
		if err := db.Where("id = ? AND channel_id = ?", data.BeforeID, data.ChannelID).Limit(1).Find(&beforeMsg).Error; err != nil {
			return nil, err
		}
		if beforeMsg.ID == "" {
			return nil, fmt.Errorf("before_id 指定的消息不存在")
		}
	}

	if data.AfterID != "" && data.AfterID != data.MessageID {
		if err := db.Where("id = ? AND channel_id = ?", data.AfterID, data.ChannelID).Limit(1).Find(&afterMsg).Error; err != nil {
			return nil, err
		}
		if afterMsg.ID == "" {
			return nil, fmt.Errorf("after_id 指定的消息不存在")
		}
	}

	if beforeMsg.ID != "" && afterMsg.ID != "" && beforeMsg.ID == afterMsg.ID {
		return nil, fmt.Errorf("before_id 与 after_id 不应指向同一条消息")
	}

	newOrder := msg.DisplayOrder
	switch {
	case beforeMsg.ID != "" && afterMsg.ID != "":
		if beforeMsg.DisplayOrder <= afterMsg.DisplayOrder+displayOrderEpsilon {
			if err := model.RebalanceChannelDisplayOrder(data.ChannelID); err != nil {
				return nil, err
			}
			if err := db.Where("id = ? AND channel_id = ?", data.BeforeID, data.ChannelID).Limit(1).Find(&beforeMsg).Error; err != nil {
				return nil, err
			}
			if err := db.Where("id = ? AND channel_id = ?", data.AfterID, data.ChannelID).Limit(1).Find(&afterMsg).Error; err != nil {
				return nil, err
			}
		}
		if beforeMsg.ID == "" || afterMsg.ID == "" {
			return nil, fmt.Errorf("无法获取目标位置的邻居消息")
		}
		newOrder = (beforeMsg.DisplayOrder + afterMsg.DisplayOrder) / 2
	case beforeMsg.ID != "":
		newOrder = beforeMsg.DisplayOrder - displayOrderGap/2
	case afterMsg.ID != "":
		newOrder = afterMsg.DisplayOrder + displayOrderGap/2
	}

	if math.Abs(newOrder-msg.DisplayOrder) < displayOrderEpsilon {
		return &struct {
			MessageID    string  `json:"message_id"`
			ChannelID    string  `json:"channel_id"`
			DisplayOrder float64 `json:"display_order"`
		}{MessageID: msg.ID, ChannelID: data.ChannelID, DisplayOrder: msg.DisplayOrder}, nil
	}

	if err := db.Model(&model.MessageModel{}).Where("id = ?", msg.ID).UpdateColumn("display_order", newOrder).Error; err != nil {
		return nil, err
	}
	msg.DisplayOrder = newOrder

	if msg.WhisperTo != "" && msg.WhisperTarget == nil {
		msg.WhisperTarget = model.UserGet(msg.WhisperTo)
	}

	channelData := channel.ToProtocolType()
	messageData := msg.ToProtocolType2(channelData)
	messageData.Content = msg.Content
	if msg.User != nil {
		messageData.User = msg.User.ToProtocolType()
	}
	if msg.Member != nil {
		messageData.Member = msg.Member.ToProtocolType()
	}
	if msg.WhisperTarget != nil {
		messageData.WhisperTo = msg.WhisperTarget.ToProtocolType()
	}

	operatorData := ctx.User.ToProtocolType()
	ev := &protocol.Event{
		Type:     protocol.EventMessageReordered,
		Message:  messageData,
		Channel:  channelData,
		User:     operatorData,
		Operator: operatorData,
		Reorder: &protocol.MessageReorder{
			MessageID:    msg.ID,
			ChannelID:    data.ChannelID,
			DisplayOrder: msg.DisplayOrder,
			BeforeID:     data.BeforeID,
			AfterID:      data.AfterID,
			ClientOpID:   data.ClientOpID,
		},
	}

	if msg.IsWhisper && msg.WhisperTo != "" {
		recipients := lo.Uniq([]string{ctx.User.ID, msg.WhisperTo})
		ctx.BroadcastEventInChannelToUsers(data.ChannelID, recipients, ev)
	} else {
		ctx.BroadcastEventInChannel(data.ChannelID, ev)
		ctx.BroadcastEventInChannelForBot(data.ChannelID, ev)
	}

	return &struct {
		MessageID    string  `json:"message_id"`
		ChannelID    string  `json:"channel_id"`
		DisplayOrder float64 `json:"display_order"`
	}{
		MessageID:    msg.ID,
		ChannelID:    data.ChannelID,
		DisplayOrder: msg.DisplayOrder,
	}, nil
}

func apiMessageEditHistory(ctx *ChatContext, data *struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
}) (any, error) {
	channelId := data.ChannelID
	if len(channelId) < 30 {
		if !pm.CanWithChannelRole(ctx.User.ID, channelId, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
			return nil, nil
		}
	} else {
		fr, _ := model.FriendRelationGetByID(channelId)
		if fr.ID == "" {
			return nil, nil
		}
	}

	var histories []model.MessageEditHistoryModel
	model.GetDB().Where("message_id = ?", data.MessageID).Order("created_at asc").Find(&histories)

	userIDs := make([]string, 0, len(histories))
	for _, h := range histories {
		userIDs = append(userIDs, h.EditorID)
	}
	userIDs = lo.Uniq(userIDs)

	id2User := map[string]*model.UserModel{}
	if len(userIDs) > 0 {
		var users []*model.UserModel
		model.GetDB().Where("id in ?", userIDs).Find(&users)
		for _, u := range users {
			id2User[u.ID] = u
		}
	}

	type historyItem struct {
		PrevContent string         `json:"prev_content"`
		EditedAt    int64          `json:"edited_at"`
		Editor      *protocol.User `json:"editor"`
	}

	var resp []historyItem
	for _, h := range histories {
		var editor *protocol.User
		if u, ok := id2User[h.EditorID]; ok {
			editor = u.ToProtocolType()
		}
		resp = append(resp, historyItem{
			PrevContent: h.PrevContent,
			EditedAt:    h.CreatedAt.UnixMilli(),
			Editor:      editor,
		})
	}

	return &struct {
		History []historyItem `json:"history"`
	}{History: resp}, nil
}

func normalizeTypingState(raw string, enabled *bool) protocol.TypingState {
	state := strings.ToLower(strings.TrimSpace(raw))
	switch state {
	case string(protocol.TypingStateContent), string(protocol.TypingStateOn):
		return protocol.TypingStateContent
	case string(protocol.TypingStateSilent):
		return protocol.TypingStateSilent
	case string(protocol.TypingStateIndicator), string(protocol.TypingStateOff):
		return protocol.TypingStateIndicator
	}
	if enabled != nil {
		if *enabled {
			return protocol.TypingStateContent
		}
		return protocol.TypingStateIndicator
	}
	return protocol.TypingStateIndicator
}

func apiMessageTyping(ctx *ChatContext, data *struct {
	ChannelID string `json:"channel_id"`
	State     string `json:"state"`
	Content   string `json:"content"`
	MessageID string `json:"message_id"`
	Mode      string `json:"mode"`
	Enabled   *bool  `json:"enabled"`
}) (any, error) {
	channelId := data.ChannelID
	if len(channelId) < 30 {
		if !pm.CanWithChannelRole(ctx.User.ID, channelId, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
			return nil, nil
		}
	} else {
		fr, _ := model.FriendRelationGetByID(channelId)
		if fr.ID == "" {
			return nil, nil
		}
	}

	if ctx.ConnInfo == nil || ctx.ConnInfo.ChannelId != channelId {
		return &struct {
			Success bool `json:"success"`
		}{Success: false}, nil
	}

	runes := []rune(data.Content)
	if len(runes) > 500 {
		data.Content = string(runes[:500])
	}

	now := time.Now().UnixMilli()
	const typingThrottleGap int64 = 250

	state := normalizeTypingState(data.State, data.Enabled)

	isActive := state != protocol.TypingStateSilent

	if isActive {
		if ctx.ConnInfo.TypingEnabled &&
			ctx.ConnInfo.TypingState == state &&
			now-ctx.ConnInfo.TypingUpdatedAt < typingThrottleGap &&
			ctx.ConnInfo.TypingContent == data.Content {
			return &struct {
				Success bool `json:"success"`
			}{Success: true}, nil
		}
		ctx.ConnInfo.TypingEnabled = true
		ctx.ConnInfo.TypingState = state
		ctx.ConnInfo.TypingContent = data.Content
		ctx.ConnInfo.TypingUpdatedAt = now
	} else {
		ctx.ConnInfo.TypingEnabled = false
		ctx.ConnInfo.TypingState = protocol.TypingStateSilent
		ctx.ConnInfo.TypingContent = ""
		ctx.ConnInfo.TypingUpdatedAt = 0
	}

	channel, _ := model.ChannelGet(channelId)
	if channel.ID == "" {
		return nil, nil
	}
	channelData := channel.ToProtocolType()
	member, _ := model.MemberGetByUserIDAndChannelID(ctx.User.ID, channelId, ctx.User.Nickname)

	content := data.Content
	if state == protocol.TypingStateIndicator {
		content = ""
	}

	event := &protocol.Event{
		Type:    protocol.EventTypingPreview,
		Channel: channelData,
		User:    ctx.User.ToProtocolType(),
		Typing: &protocol.TypingPreview{
			State:     state,
			Enabled:   state != protocol.TypingStateSilent,
			Content:   content,
			Mode:      data.Mode,
			MessageID: data.MessageID,
		},
	}
	if member != nil {
		event.Member = member.ToProtocolType()
	}

	ctx.BroadcastEventInChannelExcept(channelId, []string{ctx.User.ID}, event)

	return &struct {
		Success bool `json:"success"`
	}{Success: true}, nil
}

func builtinSealBotSolve(ctx *ChatContext, data *struct {
	ChannelID string `json:"channel_id"`
	QuoteID   string `json:"quote_id"`
	Content   string `json:"content"`
	WhisperTo string `json:"whisper_to"`
	ClientID  string `json:"client_id"`
}, channelData *protocol.Channel) {
	content := data.Content
	if len(content) >= 2 && (content[0] == '/' || content[0] == '.') && content[1] == 'x' {
		vm := ds.NewVM()
		var botText string
		expr := strings.TrimSpace(content[2:])

		if expr == "" {
			expr = "d100"
		}

		err := vm.Run(expr)
		vm.Config.EnableDiceWoD = true
		vm.Config.EnableDiceCoC = true
		vm.Config.EnableDiceFate = true
		vm.Config.EnableDiceDoubleCross = true
		vm.Config.DefaultDiceSideExpr = "面数 ?? 100"
		vm.Config.OpCountLimit = 30000

		if err != nil {
			botText = "出错:" + err.Error()
		} else {
			sb := strings.Builder{}
			sb.WriteString(fmt.Sprintf("算式: %s\n", expr))
			sb.WriteString(fmt.Sprintf("过程: %s\n", vm.GetDetailText()))
			sb.WriteString(fmt.Sprintf("结果: %s\n", vm.Ret.ToString()))
			sb.WriteString(fmt.Sprintf("栈顶: %d 层数:%d 算力: %d\n", vm.StackTop(), vm.Depth(), vm.NumOpCount))
			sb.WriteString(fmt.Sprintf("注: 这是一只小海豹，只有基本骰点功能，完整功能请接入海豹核心"))
			botText = sb.String()
		}

		m := model.MessageModel{
			StringPKBaseModel: model.StringPKBaseModel{
				ID: utils.NewID(),
			},
			UserID:    "BOT:1000",
			ChannelID: data.ChannelID,
			MemberID:  "BOT:1000",
			Content:   botText,
		}
		model.GetDB().Create(&m)

		userData := &protocol.User{
			ID:     "BOT:1000",
			Nick:   "小海豹",
			Avatar: "",
			IsBot:  true,
		}
		messageData := m.ToProtocolType2(channelData)
		messageData.User = userData
		messageData.Member = &protocol.GuildMember{
			Name: userData.Nick,
			Nick: userData.Nick,
		}

		ctx.BroadcastEvent(&protocol.Event{
			// 协议规定: 事件中必须含有 channel，message，user
			Type:    protocol.EventMessageCreated,
			Message: messageData,
			Channel: channelData,
			User:    userData,
		})
	}
}

func apiUnreadCount(ctx *ChatContext, data *struct{}) (any, error) {
	chIds, _ := service.ChannelIdList(ctx.User.ID)
	lst, err := model.ChannelUnreadFetch(chIds, ctx.User.ID)
	if err != nil {
		return nil, err
	}
	return lst, err
}

package api

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"regexp"
	"sealchat/service"
	"sealchat/service/metrics"
	"sort"
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

var hiddenDiceForwardPattern = regexp.MustCompile(`SEALCHAT-Group:([A-Za-z0-9_-]+)`)

type typingOrderCandidate struct {
	userId    string
	orderKey  float64
	updatedAt int64
}

func buildTypingOrderSnapshot(
	channelId string,
	userConnInfo *utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]],
) []typingOrderCandidate {
	if channelId == "" || userConnInfo == nil {
		return nil
	}
	candidates := map[string]typingOrderCandidate{}
	userConnInfo.Range(func(userId string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		if connMap == nil {
			return true
		}
		connMap.Range(func(_ *WsSyncConn, info *ConnInfo) bool {
			if info == nil || info.ChannelId != channelId {
				return true
			}
			if !info.TypingEnabled || info.TypingState == protocol.TypingStateSilent {
				return true
			}
			if info.TypingWhisperTo != "" {
				return true
			}
			if info.TypingOrderKey <= 0 {
				return true
			}
			updated := info.TypingUpdatedAt
			existing, ok := candidates[userId]
			if !ok || updated > existing.updatedAt {
				candidates[userId] = typingOrderCandidate{
					userId:    userId,
					orderKey:  info.TypingOrderKey,
					updatedAt: updated,
				}
			}
			return true
		})
		return true
	})
	if len(candidates) == 0 {
		return nil
	}
	snapshot := make([]typingOrderCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		snapshot = append(snapshot, candidate)
	}
	sort.Slice(snapshot, func(i, j int) bool {
		if snapshot[i].orderKey != snapshot[j].orderKey {
			return snapshot[i].orderKey < snapshot[j].orderKey
		}
		if snapshot[i].updatedAt != snapshot[j].updatedAt {
			return snapshot[i].updatedAt > snapshot[j].updatedAt
		}
		return snapshot[i].userId < snapshot[j].userId
	})
	return snapshot
}

func resolveTypingPreviewDisplayOrder(
	snapshot []typingOrderCandidate,
	senderId string,
	nowMs int64,
	windowMs int64,
) (float64, bool) {
	if len(snapshot) == 0 || windowMs <= 0 {
		return 0, false
	}
	rank := -1
	for i, item := range snapshot {
		if item.userId == senderId {
			rank = i
			break
		}
	}
	count := len(snapshot)
	if rank < 0 {
		rank = count
		count += 1
	}
	base := (nowMs / windowMs) * windowMs
	step := float64(windowMs) / float64(count+1)
	return float64(base) + step*float64(rank+1), true
}

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

func apiMessageGet(ctx *ChatContext, data *struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
}) (any, error) {
	db := model.GetDB()

	// 权限检查
	channelId := data.ChannelID
	if ctx.IsReadOnly() {
		if len(channelId) >= 30 {
			return nil, fmt.Errorf("频道不可公开访问")
		}
		if _, err := service.CanGuestAccessChannel(channelId); err != nil {
			return nil, err
		}
	} else if len(channelId) < 30 {
		if !pm.CanWithChannelRole(ctx.User.ID, channelId, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
			return nil, nil
		}
	} else {
		fr, _ := model.FriendRelationGetByID(channelId)
		if fr.ID == "" {
			return nil, nil
		}
	}

	var item model.MessageModel
	q := db.Where("channel_id = ? AND id = ?", data.ChannelID, data.MessageID)
	q = q.Where("is_deleted = ?", false)
	q = q.Where(`(is_whisper = ? OR user_id = ? OR whisper_to = ? OR EXISTS (
		SELECT 1 FROM message_whisper_recipients r WHERE r.message_id = messages.id AND r.user_id = ?
	))`, false, ctx.User.ID, ctx.User.ID, ctx.User.ID)
	q.Limit(1).Find(&item)

	if item.ID == "" {
		return nil, nil
	}

	return map[string]any{
		"id":            item.ID,
		"channel_id":    item.ChannelID,
		"created_at":    item.CreatedAt.UnixMilli(),
		"display_order": item.DisplayOrder,
	}, nil
}

func apiMessageDelete(ctx *ChatContext, data *struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
}) (any, error) {
	db := model.GetDB()
	item := model.MessageModel{}
	db.Where("channel_id = ? and id = ?", data.ChannelID, data.MessageID).Limit(1).Find(&item)
	if item.ID != "" {
		if item.IsDeleted {
			return nil, fmt.Errorf("消息已删除，无法撤回")
		}
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

		ctx.BroadcastEventInChannel(data.ChannelID, &protocol.Event{
			// 协议规定: 事件中必须含有 channel，message，user
			Type:    protocol.EventMessageDeleted,
			Message: item.ToProtocolType2(channelData),
			Channel: channelData,
			User:    ctx.User.ToProtocolType(),
		})

		_ = model.WebhookEventLogAppendForMessage(data.ChannelID, "message-deleted", item.ID)

		return &struct {
			Success bool `json:"success"`
		}{Success: true}, nil
	}

	return nil, nil
}

func apiMessageRemove(ctx *ChatContext, data *struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
}) (any, error) {
	channelID := strings.TrimSpace(data.ChannelID)
	messageID := strings.TrimSpace(data.MessageID)
	if channelID == "" || messageID == "" {
		return nil, fmt.Errorf("channel_id 和 message_id 不能为空")
	}

	db := model.GetDB()
	var msg model.MessageModel
	query := db.Preload("User", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, username, nickname, avatar, is_bot")
	}).Preload("Member", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, nickname, user_id, channel_id")
	}).Where("channel_id = ? AND id = ?", channelID, messageID)
	result := query.Limit(1).Find(&msg)
	if result.Error != nil {
		return nil, result.Error
	}
	if msg.ID == "" || msg.IsDeleted {
		return nil, fmt.Errorf("消息不存在或已删除")
	}

	channel, err := model.ChannelGet(channelID)
	if err != nil || channel.ID == "" {
		return nil, fmt.Errorf("频道不存在")
	}

	operatorID := ctx.User.ID
	targetUserID := msg.UserID
	if targetUserID != operatorID {
		operatorIsAdmin := isChannelAdminUser(channel, channelID, operatorID) ||
			service.IsWorldAdmin(channel.WorldID, operatorID)
		if !operatorIsAdmin && !pm.CanWithSystemRole(operatorID, pm.PermModAdmin) {
			return nil, fmt.Errorf("无权限删除该消息")
		}
		if isChannelAdminUser(channel, channelID, targetUserID) ||
			service.IsWorldAdmin(channel.WorldID, targetUserID) {
			return nil, fmt.Errorf("无法删除拥有管理员权限的成员消息")
		}
	}

	now := time.Now()
	updateData := map[string]any{
		"is_deleted": true,
		"deleted_at": now,
		"deleted_by": operatorID,
		"content":    "",
	}
	if err := db.Model(&model.MessageModel{}).
		Where("id = ? AND channel_id = ? AND is_deleted = ?", msg.ID, channelID, false).
		Updates(updateData).Error; err != nil {
		return nil, err
	}

	msg.IsDeleted = true
	msg.DeletedBy = operatorID
	msg.Content = ""
	msg.DeletedAt = &now
	if msg.WhisperTo != "" {
		msg.WhisperTarget = loadWhisperTargetForChannel(channelID, msg.WhisperTo)
	}
	if msg.IsWhisper {
		msg.WhisperTargets = loadWhisperTargetsForMessage(channelID, msg.ID, msg.WhisperTarget)
	}
	msg.EnsureWhisperMeta()

	channelData := channel.ToProtocolType()
	messageData := buildProtocolMessage(&msg, channelData)
	messageData.User = msg.User.ToProtocolType()

	ev := &protocol.Event{
		Type:    protocol.EventMessageRemoved,
		Message: messageData,
		Channel: channelData,
		User:    ctx.User.ToProtocolType(),
	}

	if msg.IsWhisper {
		recipients := []string{ctx.User.ID}
		if msg.UserID != "" {
			recipients = append(recipients, msg.UserID)
		}
		if msg.WhisperTo != "" {
			recipients = append(recipients, msg.WhisperTo)
		}
		recipientIDs := model.GetWhisperRecipientIDs(msg.ID)
		if len(recipientIDs) > 0 {
			recipients = append(recipients, recipientIDs...)
		}
		recipients = lo.Uniq(recipients)
		ctx.BroadcastEventInChannelToUsers(channelID, recipients, ev)
	} else {
		ctx.BroadcastEventInChannel(channelID, ev)
		ctx.BroadcastEventInChannelForBot(channelID, ev)
	}

	_ = model.WebhookEventLogAppendForMessage(channelID, "message-removed", msg.ID)

	return &struct {
		Success bool `json:"success"`
	}{Success: true}, nil
}

func hydrateMessagesForBroadcast(messages []*model.MessageModel) {
	if len(messages) == 0 {
		return
	}

	utils.QueryOneToManyMap(model.GetDB(), messages, func(i *model.MessageModel) []string {
		if i.QuoteID == "" {
			return nil
		}
		return []string{i.QuoteID}
	}, func(i *model.MessageModel, x []*model.MessageModel) {
		if len(x) == 0 {
			return
		}
		i.Quote = x[0]
	}, "id, content, created_at, user_id, is_revoked, is_deleted, whisper_to, channel_id, whisper_sender_member_id, whisper_sender_member_name, whisper_sender_user_name, whisper_sender_user_nick, whisper_target_member_id, whisper_target_member_name, whisper_target_user_name, whisper_target_user_nick")

	var whisperMsgIDs []string
	for _, item := range messages {
		if item.IsWhisper {
			whisperMsgIDs = append(whisperMsgIDs, item.ID)
		}
		if item.Quote != nil && item.Quote.IsWhisper {
			whisperMsgIDs = append(whisperMsgIDs, item.Quote.ID)
		}
	}
	recipientMap := model.GetWhisperRecipientIDsBatch(whisperMsgIDs)

	whisperIDSet := map[string]struct{}{}
	for _, item := range messages {
		if item.WhisperTo != "" {
			whisperIDSet[item.WhisperTo] = struct{}{}
		}
		if ids, ok := recipientMap[item.ID]; ok {
			for _, id := range ids {
				whisperIDSet[id] = struct{}{}
			}
		}
		if item.Quote != nil {
			if item.Quote.WhisperTo != "" {
				whisperIDSet[item.Quote.WhisperTo] = struct{}{}
			}
			if ids, ok := recipientMap[item.Quote.ID]; ok {
				for _, id := range ids {
					whisperIDSet[id] = struct{}{}
				}
			}
		}
	}

	var ids []string
	for id := range whisperIDSet {
		ids = append(ids, id)
	}
	var whisperUsers []*model.UserModel
	if len(ids) > 0 {
		model.GetDB().Where("id IN ?", ids).Find(&whisperUsers)
	}
	id2User := make(map[string]*model.UserModel, len(whisperUsers))
	for _, u := range whisperUsers {
		id2User[u.ID] = u
	}

	channelBuckets := map[string]map[string]*model.UserModel{}
	for _, item := range messages {
		if user, ok := id2User[item.WhisperTo]; ok {
			item.WhisperTarget = user
			ch := item.ChannelID
			if ch != "" {
				if channelBuckets[ch] == nil {
					channelBuckets[ch] = map[string]*model.UserModel{}
				}
				channelBuckets[ch][user.ID] = user
			}
		}
		if ids, ok := recipientMap[item.ID]; ok && len(ids) > 0 {
			targets := make([]*model.UserModel, 0, len(ids))
			for _, id := range ids {
				if user, ok := id2User[id]; ok && user != nil {
					targets = append(targets, user)
					ch := item.ChannelID
					if ch != "" {
						if channelBuckets[ch] == nil {
							channelBuckets[ch] = map[string]*model.UserModel{}
						}
						channelBuckets[ch][user.ID] = user
					}
				}
			}
			item.WhisperTargets = targets
		} else if item.IsWhisper && item.WhisperTarget != nil {
			item.WhisperTargets = []*model.UserModel{item.WhisperTarget}
		}

		if item.Quote != nil {
			if user, ok := id2User[item.Quote.WhisperTo]; ok {
				item.Quote.WhisperTarget = user
				ch := item.Quote.ChannelID
				if ch == "" {
					ch = item.ChannelID
				}
				if ch != "" {
					if channelBuckets[ch] == nil {
						channelBuckets[ch] = map[string]*model.UserModel{}
					}
					channelBuckets[ch][user.ID] = user
				}
			}
			if ids, ok := recipientMap[item.Quote.ID]; ok && len(ids) > 0 {
				targets := make([]*model.UserModel, 0, len(ids))
				for _, id := range ids {
					if user, ok := id2User[id]; ok && user != nil {
						targets = append(targets, user)
						ch := item.Quote.ChannelID
						if ch == "" {
							ch = item.ChannelID
						}
						if ch != "" {
							if channelBuckets[ch] == nil {
								channelBuckets[ch] = map[string]*model.UserModel{}
							}
							channelBuckets[ch][user.ID] = user
						}
					}
				}
				item.Quote.WhisperTargets = targets
			} else if item.Quote.IsWhisper && item.Quote.WhisperTarget != nil {
				item.Quote.WhisperTargets = []*model.UserModel{item.Quote.WhisperTarget}
			}
		}
	}

	for channelID, users := range channelBuckets {
		applyChannelNicknamesForUsers(channelID, users)
	}
	for _, item := range messages {
		item.EnsureWhisperMeta()
		if item.Quote != nil {
			item.Quote.EnsureWhisperMeta()
		}
	}
}

func setUserNickFromMember(user *model.UserModel, member *model.MemberModel) {
	if user == nil || member == nil {
		return
	}
	nick := strings.TrimSpace(member.Nickname)
	if nick == "" {
		return
	}
	user.Nickname = nick
}

func loadWhisperTargetForChannel(channelID, userID string) *model.UserModel {
	if strings.TrimSpace(channelID) == "" || strings.TrimSpace(userID) == "" {
		return nil
	}
	user := model.UserGet(userID)
	if user == nil {
		return nil
	}
	if len(channelID) >= 30 {
		return user
	}
	member, _ := model.MemberGetByUserIDAndChannelIDBase(userID, channelID, "", false)
	setUserNickFromMember(user, member)
	return user
}

func applyChannelNicknamesForUsers(channelID string, userMap map[string]*model.UserModel) {
	if len(userMap) == 0 || strings.TrimSpace(channelID) == "" {
		return
	}
	if len(channelID) >= 30 {
		return
	}
	var userIDs []string
	for id := range userMap {
		if strings.TrimSpace(id) != "" {
			userIDs = append(userIDs, id)
		}
	}
	if len(userIDs) == 0 {
		return
	}
	var members []*model.MemberModel
	model.GetDB().Where("channel_id = ? AND user_id IN ?", channelID, userIDs).Find(&members)
	for _, member := range members {
		if member == nil || strings.TrimSpace(member.UserID) == "" {
			continue
		}
		if user, ok := userMap[member.UserID]; ok {
			setUserNickFromMember(user, member)
		}
	}
}

func loadWhisperTargetsForMessage(channelID, messageID string, fallback *model.UserModel) []*model.UserModel {
	if messageID == "" {
		if fallback != nil {
			return []*model.UserModel{fallback}
		}
		return nil
	}
	ids := model.GetWhisperRecipientIDs(messageID)
	if len(ids) == 0 {
		if fallback != nil {
			return []*model.UserModel{fallback}
		}
		return nil
	}
	var users []*model.UserModel
	model.GetDB().Where("id IN ?", ids).Find(&users)
	id2User := make(map[string]*model.UserModel, len(users))
	for _, user := range users {
		id2User[user.ID] = user
	}
	if fallback != nil && fallback.ID != "" {
		id2User[fallback.ID] = fallback
	}
	if len(channelID) < 30 {
		applyChannelNicknamesForUsers(channelID, id2User)
	}
	targets := make([]*model.UserModel, 0, len(ids))
	for _, id := range ids {
		if user, ok := id2User[id]; ok && user != nil {
			targets = append(targets, user)
		}
	}
	return targets
}

func buildProtocolMessage(msg *model.MessageModel, channelData *protocol.Channel) *protocol.Message {
	messageData := msg.ToProtocolType2(channelData)
	messageData.Content = msg.Content
	if msg.User != nil {
		messageData.User = msg.User.ToProtocolType()
	} else {
		if user := model.UserGet(msg.UserID); user != nil {
			messageData.User = user.ToProtocolType()
		}
	}
	if msg.Member != nil {
		messageData.Member = msg.Member.ToProtocolType()
	}
	if msg.WhisperTarget != nil {
		messageData.WhisperTo = msg.WhisperTarget.ToProtocolType()
	}
	if msg.Quote != nil {
		quoteData := msg.Quote.ToProtocolType2(channelData)
		quoteData.Content = msg.Quote.Content
		if msg.Quote.User != nil {
			quoteData.User = msg.Quote.User.ToProtocolType()
		}
		if msg.Quote.Member != nil {
			quoteData.Member = msg.Quote.Member.ToProtocolType()
		}
		if msg.Quote.WhisperTarget != nil {
			quoteData.WhisperTo = msg.Quote.WhisperTarget.ToProtocolType()
		}
		messageData.Quote = quoteData
	}
	return messageData
}

func messageArchiveMutate(ctx *ChatContext, channel *model.ChannelModel, ids []string, reason string, archived bool) ([]*model.MessageModel, error) {
	if channel == nil || channel.ID == "" {
		return nil, fmt.Errorf("频道不存在")
	}
	if len(ids) == 0 {
		return []*model.MessageModel{}, nil
	}

	db := model.GetDB()
	trimmedReason := strings.TrimSpace(reason)
	updates := map[string]any{
		"is_archived": archived,
	}
	var archivedAt time.Time
	if archived {
		archivedAt = time.Now()
		updates["archived_at"] = archivedAt
		updates["archived_by"] = ctx.User.ID
		updates["archive_reason"] = trimmedReason
	} else {
		updates["archived_at"] = gorm.Expr("NULL")
		updates["archived_by"] = ""
		updates["archive_reason"] = ""
	}

	result := db.Model(&model.MessageModel{}).
		Where("channel_id = ? AND id IN ?", channel.ID, ids).
		Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("未找到可归档的消息")
	}

	var messages []*model.MessageModel
	if err := db.Preload("User", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, username, nickname, avatar, is_bot")
	}).Preload("Member", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, nickname, channel_id, user_id")
	}).Where("channel_id = ? AND id IN ?", channel.ID, ids).Find(&messages).Error; err != nil {
		return nil, err
	}

	for _, msg := range messages {
		msg.IsArchived = archived
		if archived {
			msg.ArchivedBy = ctx.User.ID
			msg.ArchiveReason = trimmedReason
			if archivedAt.IsZero() {
				msg.ArchivedAt = nil
			} else {
				copyTime := archivedAt
				msg.ArchivedAt = &copyTime
			}
		} else {
			msg.ArchivedBy = ""
			msg.ArchiveReason = ""
			msg.ArchivedAt = nil
		}
	}

	hydrateMessagesForBroadcast(messages)

	payload := map[string]any{
		"reason":    trimmedReason,
		"archived":  archived,
		"operator":  ctx.User.ID,
		"timestamp": time.Now().UnixMilli(),
	}
	payloadBytes, _ := json.Marshal(payload)
	var logs []*model.MessageArchiveLogModel
	action := "archive"
	if !archived {
		action = "unarchive"
	}
	for _, id := range ids {
		logs = append(logs, &model.MessageArchiveLogModel{
			MessageID:   id,
			ChannelID:   channel.ID,
			OperatorID:  ctx.User.ID,
			Action:      action,
			PayloadJSON: string(payloadBytes),
		})
	}
	_ = model.MessageArchiveLogBatchCreate(logs)

	return messages, nil
}

func collectMessageIDs(messageIDs []string) []string {
	var ids []string
	for _, id := range messageIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	return ids
}

func loadArchiveContext(channelID string, messageIDs []string) (*model.ChannelModel, []*model.MessageModel, error) {
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, nil, err
	}
	if channel.ID == "" {
		return nil, nil, fmt.Errorf("频道不存在")
	}

	if len(messageIDs) == 0 {
		return channel, []*model.MessageModel{}, nil
	}

	db := model.GetDB()
	var messages []*model.MessageModel
	err = db.Preload("User", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, username, nickname, avatar, is_bot")
	}).Preload("Member", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, nickname, channel_id, user_id")
	}).Where("channel_id = ? AND id IN ? AND is_deleted = ?", channelID, messageIDs, false).Find(&messages).Error
	if err != nil {
		return nil, nil, err
	}

	return channel, messages, nil
}

func isChannelAdminUser(channel *model.ChannelModel, channelID, userID string) bool {
	if userID == "" {
		return false
	}
	if channel != nil && channel.UserID == userID {
		return true
	}
	return pm.CanWithChannelRole(userID, channelID,
		pm.PermFuncChannelManageInfo,
		pm.PermFuncChannelManageRole,
		pm.PermFuncChannelManageRoleRoot,
		pm.PermFuncChannelMessageArchive,
		pm.PermFuncChannelMessageDelete,
		pm.PermFuncChannelRoleLinkRoot,
		pm.PermFuncChannelRoleUnlinkRoot,
	)
}

func apiMessageArchive(ctx *ChatContext, data *struct {
	ChannelID  string   `json:"channel_id"`
	MessageIDs []string `json:"message_ids"`
	Reason     string   `json:"reason"`
}) (any, error) {
	if strings.TrimSpace(data.ChannelID) == "" {
		return nil, fmt.Errorf("channel_id 不能为空")
	}
	ids := collectMessageIDs(data.MessageIDs)
	if len(ids) == 0 {
		return nil, fmt.Errorf("message_ids 不能为空")
	}
	channel, messages, err := loadArchiveContext(data.ChannelID, ids)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("未找到可归档的消息或无权限执行该操作")
	}

	hasArchivePerm := pm.CanWithChannelRole(ctx.User.ID, data.ChannelID, pm.PermFuncChannelMessageArchive, pm.PermFuncChannelManageInfo)
	operatorID := ctx.User.ID
	if !hasArchivePerm {
		for _, msg := range messages {
			if msg.UserID != operatorID {
				return nil, fmt.Errorf("无权限归档目标消息")
			}
		}
	} else {
		operatorIsAdmin := isChannelAdminUser(channel, data.ChannelID, operatorID)
		for _, msg := range messages {
			if msg.UserID == operatorID {
				continue
			}
			if !operatorIsAdmin {
				return nil, fmt.Errorf("无权限归档目标消息")
			}
			if isChannelAdminUser(channel, data.ChannelID, msg.UserID) {
				return nil, fmt.Errorf("无法归档同样具有管理员权限的成员消息")
			}
		}
	}

	updatedMessages, err := messageArchiveMutate(ctx, channel, ids, data.Reason, true)
	if err != nil {
		return nil, err
	}

	channelData := channel.ToProtocolType()
	operator := ctx.User.ToProtocolType()

	for _, msg := range updatedMessages {
		messageData := buildProtocolMessage(msg, channelData)
		ev := &protocol.Event{
			Type:    protocol.EventMessageArchived,
			Message: messageData,
			Channel: channelData,
			User:    operator,
		}
		if msg.IsWhisper {
			recipients := []string{ctx.User.ID}
			if msg.WhisperTo != "" {
				recipients = append(recipients, msg.WhisperTo)
			}
			recipientIDs := model.GetWhisperRecipientIDs(msg.ID)
			if len(recipientIDs) > 0 {
				recipients = append(recipients, recipientIDs...)
			}
			recipients = lo.Uniq(recipients)
			ctx.BroadcastEventInChannelToUsers(data.ChannelID, recipients, ev)
		} else {
			ctx.BroadcastEventInChannel(data.ChannelID, ev)
			ctx.BroadcastEventInChannelForBot(data.ChannelID, ev)
		}
	}

	return &struct {
		MessageIDs []string `json:"message_ids"`
		Archived   bool     `json:"archived"`
	}{MessageIDs: lo.Uniq(ids), Archived: true}, nil
}

func apiMessageUnarchive(ctx *ChatContext, data *struct {
	ChannelID  string   `json:"channel_id"`
	MessageIDs []string `json:"message_ids"`
}) (any, error) {
	if strings.TrimSpace(data.ChannelID) == "" {
		return nil, fmt.Errorf("channel_id 不能为空")
	}
	ids := collectMessageIDs(data.MessageIDs)
	if len(ids) == 0 {
		return nil, fmt.Errorf("message_ids 不能为空")
	}
	channel, messages, err := loadArchiveContext(data.ChannelID, ids)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("未找到可取消归档的消息或无权限执行该操作")
	}

	hasArchivePerm := pm.CanWithChannelRole(ctx.User.ID, data.ChannelID, pm.PermFuncChannelMessageArchive, pm.PermFuncChannelManageInfo)
	operatorID := ctx.User.ID
	if !hasArchivePerm {
		for _, msg := range messages {
			if msg.UserID != operatorID {
				return nil, fmt.Errorf("无权限取消归档目标消息")
			}
		}
	} else {
		operatorIsAdmin := isChannelAdminUser(channel, data.ChannelID, operatorID)
		for _, msg := range messages {
			if msg.UserID == operatorID {
				continue
			}
			if !operatorIsAdmin {
				return nil, fmt.Errorf("无权限取消归档目标消息")
			}
			if isChannelAdminUser(channel, data.ChannelID, msg.UserID) {
				return nil, fmt.Errorf("无法操作同样具有管理员权限的成员消息")
			}
		}
	}

	updatedMessages, err := messageArchiveMutate(ctx, channel, ids, "", false)
	if err != nil {
		return nil, err
	}

	channelData := channel.ToProtocolType()
	operator := ctx.User.ToProtocolType()

	for _, msg := range updatedMessages {
		messageData := buildProtocolMessage(msg, channelData)
		ev := &protocol.Event{
			Type:    protocol.EventMessageUnarchived,
			Message: messageData,
			Channel: channelData,
			User:    operator,
		}
		if msg.IsWhisper {
			recipients := []string{ctx.User.ID}
			if msg.WhisperTo != "" {
				recipients = append(recipients, msg.WhisperTo)
			}
			recipientIDs := model.GetWhisperRecipientIDs(msg.ID)
			if len(recipientIDs) > 0 {
				recipients = append(recipients, recipientIDs...)
			}
			recipients = lo.Uniq(recipients)
			ctx.BroadcastEventInChannelToUsers(data.ChannelID, recipients, ev)
		} else {
			ctx.BroadcastEventInChannel(data.ChannelID, ev)
			ctx.BroadcastEventInChannelForBot(data.ChannelID, ev)
		}
	}

	return &struct {
		MessageIDs []string `json:"message_ids"`
		Archived   bool     `json:"archived"`
	}{MessageIDs: lo.Uniq(ids), Archived: false}, nil
}

func apiMessageCreate(ctx *ChatContext, data *struct {
	ChannelID    string   `json:"channel_id"`
	QuoteID      string   `json:"quote_id"`
	Content      string   `json:"content"`
	WhisperTo    string   `json:"whisper_to"`
	WhisperToIds []string `json:"whisper_to_ids"`
	ClientID     string   `json:"client_id"`
	IdentityID   string   `json:"identity_id"`
	ICMode       string   `json:"ic_mode"`
	DisplayOrder *float64 `json:"display_order"`
}) (any, error) {
	echo := ctx.Echo
	db := model.GetDB()
	channelId := data.ChannelID

	var privateOtherUser string
	botMsgContext := resolveBotMessageContext(ctx, channelId)

	icMode := strings.TrimSpace(strings.ToLower(data.ICMode))
	if icMode == "" && botMsgContext != nil {
		icMode = strings.TrimSpace(strings.ToLower(botMsgContext.ICMode))
	}
	if icMode == "" {
		icMode = "ic"
	}
	if icMode != "ic" && icMode != "ooc" {
		return nil, fmt.Errorf("unsupported ic_mode: %s", icMode)
	}

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

	// BOT 消息的 Satori 内容规范化
	if ctx.User.IsBot {
		content = protocol.EscapeSatoriText(content)
		if strings.Contains(content, "data:") || strings.Contains(content, "sealchat://asset/") {
			// 将 Base64 图片/文件或 asset 引用转换为附件
			satoriResult, satoriErr := service.NormalizeSatoriContent(content, ctx.User.ID, channelId, service.SatoriAttachmentConfig{
				ImageSizeLimit:       appConfig.ImageSizeLimit * 1024,
				ImageCompress:        appConfig.ImageCompress,
				ImageCompressQuality: appConfig.ImageCompressQuality,
				TempDir:              appConfig.Storage.Local.TempDir,
				MaxImagesPerMessage:  10,
			})
			if satoriErr != nil {
				log.Printf("[BOT] Satori 内容规范化失败: %v", satoriErr)
			} else if satoriResult != nil {
				content = satoriResult.Content
				if len(satoriResult.Errors) > 0 {
					log.Printf("[BOT] Satori 图片处理部分失败 (成功:%d 跳过:%d): %v",
						satoriResult.ProcessedCount, satoriResult.SkippedCount, satoriResult.Errors)
				}
			}
		} else {
			// 纯文本消息：检查是否包含 Satori 标签
			if protocol.ContainsSatoriTags(content) {
				// 包含 Satori 标签，通过 parse -> ToString 确保 XML 转义一致性
				if root := protocol.ElementParse(content); root != nil {
					content = root.ToString()
				}
			}
			// 不包含 Satori 标签的纯文本，保持原样
			// 前端的 @satorijs/element toString() 会进行必要的 HTML 转义
		}
	}

	member, err := model.MemberGetByUserIDAndChannelID(ctx.User.ID, data.ChannelID, ctx.User.Nickname)
	if err != nil {
		return nil, err
	}

	identity, err := service.ChannelIdentityValidateMessageIdentity(ctx.User.ID, data.ChannelID, data.IdentityID)
	if err != nil {
		return nil, err
	}

	// 如果未选择身份，使用隐形默认身份（群内频道才需要）
	if identity == nil && len(channelId) < 30 {
		identity, _ = service.EnsureHiddenDefaultIdentity(ctx.User.ID, channelId)
	}

	channel, _ := model.ChannelGet(channelId)
	if channel.ID == "" {
		return nil, nil
	}
	channelData := channel.ToProtocolType()
	var renderResult *service.DiceRenderResult
	var isHiddenDice bool
	if channel.BuiltInDiceEnabled {
		renderResult, err = service.RenderDiceContent(content, channel.DefaultDiceExpr, nil)
		if err != nil {
			return nil, err
		}
		if renderResult != nil {
			content = renderResult.Content
			isHiddenDice = renderResult.IsHidden
		}
	}
	if !isHiddenDice {
		isHiddenDice = service.ContainsHiddenDiceCommand(content)
	}

	hiddenWhisperToSelf := false
	whisperTo := strings.TrimSpace(data.WhisperTo)
	if whisperTo == "" && botMsgContext != nil && botMsgContext.IsWhisper && !botMsgContext.IsHiddenDice {
		if botMsgContext.WhisperToUserID != "" {
			whisperTo = botMsgContext.WhisperToUserID
		}
	}
	if ctx.User.IsBot && channel.BotFeatureEnabled {
		if pending := resolveBotHiddenDicePending(ctx, channelId); pending != nil && pending.TargetUserID != "" {
			if whisperTo != "" {
				if ctx.ConnInfo != nil && ctx.ConnInfo.BotHiddenDicePending != nil {
					ctx.ConnInfo.BotHiddenDicePending.Delete(channelId)
				}
			} else {
				pending.Count++
				if pending.Count >= 2 {
					whisperTo = pending.TargetUserID
					if ctx.ConnInfo != nil && ctx.ConnInfo.BotHiddenDicePending != nil {
						ctx.ConnInfo.BotHiddenDicePending.Delete(channelId)
					}
				} else if ctx.ConnInfo != nil && ctx.ConnInfo.BotHiddenDicePending != nil {
					ctx.ConnInfo.BotHiddenDicePending.Store(channelId, pending)
				}
			}
		}
	}
	if isHiddenDice && len(channelId) < 30 && whisperTo == "" && !channel.BotFeatureEnabled {
		hiddenWhisperToSelf = true
		whisperTo = ctx.User.ID
	}

	whisperRecipientIDs := resolveWhisperRecipients(whisperTo, data.WhisperToIds, ctx.User.ID)
	if len(whisperRecipientIDs) > 10 {
		return nil, fmt.Errorf("悄悄话收件人数量不能超过10人")
	}
	if len(whisperRecipientIDs) > 0 && whisperTo == "" {
		whisperTo = whisperRecipientIDs[0]
	}

	var whisperUser *model.UserModel
	var whisperMember *model.MemberModel
	if whisperTo != "" {
		if whisperTo == ctx.User.ID && !hiddenWhisperToSelf {
			return nil, nil
		}
		if len(channelId) < 30 {
			member, _ := model.MemberGetByUserIDAndChannelIDBase(whisperTo, channelId, "", false)
			if member == nil {
				return nil, nil
			}
			whisperMember = member
		} else {
			if whisperTo != privateOtherUser {
				return nil, nil
			}
		}
		whisperUser = model.UserGet(whisperTo)
		if whisperUser == nil {
			return nil, nil
		}
		setUserNickFromMember(whisperUser, whisperMember)
	}

	var whisperTargets []*model.UserModel
	if len(whisperRecipientIDs) > 0 {
		if len(channelId) >= 30 {
			for _, id := range whisperRecipientIDs {
				if id != privateOtherUser {
					return nil, nil
				}
			}
		}
		whisperTargets = make([]*model.UserModel, 0, len(whisperRecipientIDs))
		for _, id := range whisperRecipientIDs {
			if whisperUser != nil && id == whisperUser.ID {
				whisperTargets = append(whisperTargets, whisperUser)
				continue
			}
			target := loadWhisperTargetForChannel(channelId, id)
			if target == nil {
				return nil, nil
			}
			whisperTargets = append(whisperTargets, target)
		}
	}

	var quote model.MessageModel
	if data.QuoteID != "" {
		db.Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, username, avatar, is_bot")
		}).Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, channel_id, user_id")
		}).Where("id = ? AND is_deleted = ?", data.QuoteID, false).Limit(1).Find(&quote)
		if quote.ID == "" {
			return nil, nil
		}
		if quote.WhisperTo != "" {
			quote.WhisperTarget = loadWhisperTargetForChannel(channelId, quote.WhisperTo)
		}
		if quote.IsWhisper {
			quote.WhisperTargets = loadWhisperTargetsForMessage(channelId, quote.ID, quote.WhisperTarget)
		}
	}

	nowMs := time.Now().UnixMilli()
	displayOrder := float64(nowMs)
	if data.DisplayOrder != nil && *data.DisplayOrder > 0 {
		displayOrder = *data.DisplayOrder
	}
	if whisperTo == "" {
		windowMs := int64(0)
		if cfg := utils.GetConfig(); cfg != nil && cfg.TypingOrderWindowMs > 0 {
			windowMs = cfg.TypingOrderWindowMs
		}
		if windowMs > 0 {
			snapshot := buildTypingOrderSnapshot(channelId, ctx.UserId2ConnInfo)
			if order, ok := resolveTypingPreviewDisplayOrder(snapshot, ctx.User.ID, nowMs, windowMs); ok {
				displayOrder = order
			}
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
		DisplayOrder: displayOrder,
		ICMode:       icMode,

		SenderMemberName: member.Nickname,
		IsWhisper:        whisperUser != nil,
		WhisperTo:        whisperTo,
		WhisperTargets:   whisperTargets,
	}
	if identity != nil {
		m.SenderRoleID = identity.ID
		m.SenderIdentityID = identity.ID
		m.SenderIdentityName = identity.DisplayName
		m.SenderIdentityColor = identity.Color
		m.SenderIdentityAvatarID = identity.AvatarAttachmentID
		if identity.DisplayName != "" {
			m.SenderMemberName = identity.DisplayName
		}
	}
	if identity == nil && ctx.User.IsBot && ctx.User.NickColor != "" {
		m.SenderIdentityColor = ctx.User.NickColor
	}
	if whisperUser != nil {
		m.WhisperTarget = whisperUser
		m.WhisperSenderUserNick = ctx.User.Nickname
		m.WhisperSenderUserName = ctx.User.Username
		if member != nil {
			m.WhisperSenderMemberID = member.ID
			m.WhisperSenderMemberName = m.SenderMemberName
		}
		m.WhisperTargetUserNick = whisperUser.Nickname
		m.WhisperTargetUserName = whisperUser.Username
		if whisperMember != nil {
			m.WhisperTargetMemberID = whisperMember.ID
			m.WhisperTargetMemberName = whisperMember.Nickname
		}
	}
	createResult := db.Create(&m)
	if createResult.Error != nil {
		return nil, createResult.Error
	}
	if len(whisperRecipientIDs) > 0 {
		if err := model.CreateWhisperRecipients(m.ID, whisperRecipientIDs); err != nil {
			log.Printf("创建悄悄话收件人记录失败: %v", err)
		}
	}
	if renderResult != nil {
		if err := model.MessageDiceRollReplace(m.ID, renderResult.Rolls); err != nil {
			return nil, err
		}
	}
	rows := createResult.RowsAffected

	if rows > 0 {
		if collector := metrics.Get(); collector != nil {
			collector.RecordMessage()
		}
		ctx.TagCheck(data.ChannelID, m.ID, content)
		member.UpdateRecentSent()
		channel.UpdateRecentSent()

		userData := ctx.User.ToProtocolType()

		messageData := m.ToProtocolType2(channelData)
		messageData.Content = content
		messageData.User = userData
		messageData.Member = member.ToProtocolType()
		messageData.Member.Roles = []string{service.ResolveMemberRoleForProtocol(ctx.User.ID, data.ChannelID, channel.WorldID)}
		messageData.ClientID = data.ClientID
		if quote.ID != "" {
			qData := quote.ToProtocolType2(channelData)
			qData.Content = quote.Content
			if quote.User != nil {
				qData.User = quote.User.ToProtocolType()
			}
			if quote.Member != nil {
				qData.Member = quote.Member.ToProtocolType()
				qData.Member.Roles = []string{service.ResolveMemberRoleForProtocol(quote.Member.UserID, quote.Member.ChannelID, "")}
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

		// 构建消息上下文
		var msgContext *protocol.MessageContext
		if channel.BotFeatureEnabled || appConfig.BuiltInSealBotEnable {
			msgContext = &protocol.MessageContext{
				ICMode:       icMode,
				IsWhisper:    whisperUser != nil,
				IsHiddenDice: isHiddenDice,
				SenderUserID: ctx.User.ID,
			}
			if whisperUser != nil {
				msgContext.WhisperToUserID = whisperUser.ID
			}
		}

		// 发出广播事件
		ev := &protocol.Event{
			// 协议规定: 事件中必须含有 channel，message，user
			Type:           protocol.EventMessageCreated,
			Message:        messageData,
			Channel:        channelData,
			User:           userData,
			MessageContext: msgContext,
		}

		if whisperUser != nil {
			recipients := []string{ctx.User.ID}
			if whisperUser.ID != "" {
				recipients = append(recipients, whisperUser.ID)
			}
			if len(whisperRecipientIDs) > 0 {
				recipients = append(recipients, whisperRecipientIDs...)
			}
			recipients = lo.Uniq(recipients)
			ctx.BroadcastEventInChannelToUsers(data.ChannelID, recipients, ev)
			ctx.BroadcastEventInChannelForBot(data.ChannelID, ev)
		} else {
			ctx.BroadcastEventInChannel(data.ChannelID, ev)
			ctx.BroadcastEventInChannelForBot(data.ChannelID, ev)
		}

		_ = model.WebhookEventLogAppendForMessage(data.ChannelID, "message-created", m.ID)

		if isHiddenDice && len(channelId) < 30 {
			go sendHiddenDicePrivateCopy(ctx, channelData, messageData)
		}
		if channel.PermType == "private" && ctx.User != nil && ctx.User.IsBot {
			go forwardHiddenDiceWhisperCopy(ctx, channel, &m, privateOtherUser)
		}

		// 当频道启用了机器人骰点时，不再触发内置小海豹以避免覆盖自定义机器人回复
		if appConfig.BuiltInSealBotEnable && whisperUser == nil && channel.BuiltInDiceEnabled && !channel.BotFeatureEnabled {
			botReq := &struct {
				ChannelID string `json:"channel_id"`
				QuoteID   string `json:"quote_id"`
				Content   string `json:"content"`
				WhisperTo string `json:"whisper_to"`
				ClientID  string `json:"client_id"`
				ICMode    string `json:"ic_mode"`
			}{
				ChannelID: data.ChannelID,
				QuoteID:   data.QuoteID,
				Content:   data.Content,
				WhisperTo: whisperTo,
				ClientID:  data.ClientID,
				ICMode:    icMode,
			}
			builtinSealBotSolve(ctx, botReq, channelData, isHiddenDice)
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
			targets := lo.Uniq([]string{whisperTo})
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
	Type            string   `json:"type"` // 查询类型，不填为默认，若time则用下面两个值
	FromTime        int64    `json:"from_time"`
	ToTime          int64    `json:"to_time"`
	ICOnly          bool     `json:"ic_only"`
	IncludeOOC      *bool    `json:"include_ooc"`
	IncludeArchived bool     `json:"include_archived"`
	ArchivedOnly    bool     `json:"archived_only"`
	UserIDs         []string `json:"user_ids"`
	RoleIDs         []string `json:"role_ids"`
	IncludeRoleless bool     `json:"include_roleless"`
	Limit           int      `json:"limit"`
}) (any, error) {
	db := model.GetDB()

	// 权限检查
	channelId := data.ChannelID
	if ctx.IsReadOnly() {
		if len(channelId) >= 30 {
			return nil, fmt.Errorf("频道不可公开访问")
		}
		if _, err := service.CanGuestAccessChannel(channelId); err != nil {
			return nil, err
		}
	} else if len(channelId) < 30 { // 注意，这不是一个好的区分方式
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
	q = q.Where("is_deleted = ?", false)
	q = q.Where(`(is_whisper = ? OR user_id = ? OR whisper_to = ? OR EXISTS (
		SELECT 1 FROM message_whisper_recipients r WHERE r.message_id = messages.id AND r.user_id = ?
	))`, false, ctx.User.ID, ctx.User.ID, ctx.User.ID)

	if data.ArchivedOnly {
		q = q.Where("is_archived = ?", true)
	} else if !data.IncludeArchived {
		q = q.Where("is_archived = ?", false)
	}
	includeOOC := true
	if data.IncludeOOC != nil {
		includeOOC = *data.IncludeOOC
	}
	if data.ICOnly {
		q = q.Where("ic_mode = ?", "ic")
	} else if !includeOOC {
		q = q.Where("ic_mode <> ?", "ooc")
	}
	if len(data.UserIDs) > 0 {
		q = q.Where("user_id IN ?", data.UserIDs)
	}
	roleIDs := make([]string, 0, len(data.RoleIDs))
	for _, id := range data.RoleIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			roleIDs = append(roleIDs, trimmed)
		}
	}
	includeRoleless := data.IncludeRoleless
	if len(roleIDs) > 0 || includeRoleless {
		roleCond := "(sender_role_id IN ? OR sender_identity_id IN ?)"
		roleArgs := []any{roleIDs, roleIDs}
		if includeRoleless {
			roleCond = "(" + roleCond + " OR ((sender_role_id = '' OR sender_role_id IS NULL) AND (sender_identity_id = '' OR sender_identity_id IS NULL)))"
		}
		if len(roleIDs) == 0 && includeRoleless {
			roleCond = "((sender_role_id = '' OR sender_role_id IS NULL) AND (sender_identity_id = '' OR sender_identity_id IS NULL))"
			roleArgs = nil
		}
		if roleArgs != nil {
			q = q.Where(roleCond, roleArgs...)
		} else {
			q = q.Where(roleCond)
		}
	}

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
	var channel *model.ChannelModel
	canReorderAll := false
	if !ctx.IsReadOnly() {
		channel, _ = model.ChannelGet(data.ChannelID)
		canReorderAll = canReorderAllMessages(ctx.User.ID, channel)
	}
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

	limit := data.Limit
	if limit <= 0 {
		limit = 30
	}
	if limit > 50 {
		limit = 50
	}

	q.Order("display_order desc").
		Order("created_at desc").
		Order("id desc").
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, username, nickname, avatar, is_bot")
		}).
		Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, channel_id")
		}).Limit(limit).Find(&items)

	utils.QueryOneToManyMap(model.GetDB(), items, func(i *model.MessageModel) []string {
		return []string{i.QuoteID}
	}, func(i *model.MessageModel, x []*model.MessageModel) {
		i.Quote = x[0]
	}, "id, content, created_at, user_id, is_revoked, is_deleted, whisper_to, channel_id, sender_member_name, sender_identity_id, sender_identity_name, sender_identity_color, sender_identity_avatar_id, whisper_sender_member_id, whisper_sender_member_name, whisper_sender_user_name, whisper_sender_user_nick, whisper_target_member_id, whisper_target_member_name, whisper_target_user_name, whisper_target_user_nick")

	if !ctx.IsReadOnly() {
		_ = model.ChannelReadSet(data.ChannelID, ctx.User.ID)
	}

	q.Count(&count)
	var next string

	items = lo.Reverse(items)
	if count > int64(len(items)) && len(items) > 0 {
		orderStr := strconv.FormatFloat(items[0].DisplayOrder, 'f', 8, 64)
		timeStr := strconv.FormatInt(items[0].CreatedAt.UnixMilli(), 10)
		next = fmt.Sprintf("%s|%s|%s", orderStr, timeStr, items[0].ID)
	}

	var whisperMsgIDs []string
	for _, i := range items {
		if i.IsWhisper {
			whisperMsgIDs = append(whisperMsgIDs, i.ID)
		}
		if i.Quote != nil && i.Quote.IsWhisper {
			whisperMsgIDs = append(whisperMsgIDs, i.Quote.ID)
		}
	}
	recipientMap := model.GetWhisperRecipientIDsBatch(whisperMsgIDs)

	whisperIdSet := map[string]struct{}{}
	for _, i := range items {
		if i.IsRevoked || i.IsDeleted {
			i.Content = ""
		}
		if i.WhisperTo != "" {
			whisperIdSet[i.WhisperTo] = struct{}{}
		}
		if ids, ok := recipientMap[i.ID]; ok {
			for _, id := range ids {
				whisperIdSet[id] = struct{}{}
			}
		}
		if i.Quote != nil {
			if i.Quote.IsRevoked || i.Quote.IsDeleted {
				i.Quote.Content = ""
			}
			if i.Quote.WhisperTo != "" {
				whisperIdSet[i.Quote.WhisperTo] = struct{}{}
			}
			if ids, ok := recipientMap[i.Quote.ID]; ok {
				for _, id := range ids {
					whisperIdSet[id] = struct{}{}
				}
			}
		}
	}

	var ids []string
	for id := range whisperIdSet {
		ids = append(ids, id)
	}
	id2User := map[string]*model.UserModel{}
	if len(ids) > 0 {
		var whisperUsers []*model.UserModel
		model.GetDB().Where("id in ?", ids).Find(&whisperUsers)
		for _, u := range whisperUsers {
			id2User[u.ID] = u
		}
		applyChannelNicknamesForUsers(data.ChannelID, id2User)
	}

	for _, i := range items {
		if user, ok := id2User[i.WhisperTo]; ok {
			i.WhisperTarget = user
		}
		if ids, ok := recipientMap[i.ID]; ok && len(ids) > 0 {
			targets := make([]*model.UserModel, 0, len(ids))
			for _, id := range ids {
				if user, ok := id2User[id]; ok && user != nil {
					targets = append(targets, user)
				}
			}
			i.WhisperTargets = targets
		} else if i.IsWhisper && i.WhisperTarget != nil {
			i.WhisperTargets = []*model.UserModel{i.WhisperTarget}
		}
		if i.Quote != nil {
			if user, ok := id2User[i.Quote.WhisperTo]; ok {
				i.Quote.WhisperTarget = user
			}
			if ids, ok := recipientMap[i.Quote.ID]; ok && len(ids) > 0 {
				targets := make([]*model.UserModel, 0, len(ids))
				for _, id := range ids {
					if user, ok := id2User[id]; ok && user != nil {
						targets = append(targets, user)
					}
				}
				i.Quote.WhisperTargets = targets
			} else if i.Quote.IsWhisper && i.Quote.WhisperTarget != nil {
				i.Quote.WhisperTargets = []*model.UserModel{i.Quote.WhisperTarget}
			}
		}
	}

	isRecipient := func(ids []string, userID string) bool {
		for _, id := range ids {
			if id == userID {
				return true
			}
		}
		return false
	}

	for _, i := range items {
		if i.IsWhisper && i.UserID != ctx.User.ID && i.WhisperTo != ctx.User.ID && !isRecipient(recipientMap[i.ID], ctx.User.ID) {
			// 理论上不会出现，因为已经过滤，但保险起见
			i.Content = ""
		}
		if i.Quote != nil && i.Quote.IsWhisper &&
			i.Quote.UserID != ctx.User.ID &&
			i.Quote.WhisperTo != ctx.User.ID &&
			!isRecipient(recipientMap[i.Quote.ID], ctx.User.ID) {
			i.Quote.Content = ""
			i.Quote.WhisperTarget = nil
			i.Quote.WhisperTargets = nil
		}
		i.EnsureWhisperMeta()
		if i.Quote != nil {
			i.Quote.EnsureWhisperMeta()
		}
	}

	if ctx.User != nil && len(items) > 0 {
		ids := make([]string, 0, len(items))
		for _, item := range items {
			if item.ID != "" {
				ids = append(ids, item.ID)
			}
		}
		if len(ids) > 0 {
			reactionMap, err := service.ListMessageReactionsForMessages(ids, ctx.User.ID)
			if err != nil {
				log.Printf("加载消息反应摘要失败: %v", err)
			} else {
				for _, item := range items {
					if list, ok := reactionMap[item.ID]; ok {
						item.Reactions = list
					} else {
						item.Reactions = []model.MessageReactionListItem{}
					}
				}
			}
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
	ChannelID  string  `json:"channel_id"`
	MessageID  string  `json:"message_id"`
	Content    string  `json:"content"`
	ICMode     string  `json:"ic_mode"`
	IdentityID *string `json:"identity_id"`
}) (any, error) {
	if strings.TrimSpace(data.Content) == "" {
		return nil, fmt.Errorf("消息内容不能为空")
	}

	icMode := strings.ToLower(strings.TrimSpace(data.ICMode))
	if icMode != "" && icMode != "ic" && icMode != "ooc" {
		return nil, fmt.Errorf("ic_mode 仅支持 ic/ooc")
	}

	db := model.GetDB()

	var msg model.MessageModel
	db.Where("id = ? AND channel_id = ?", data.MessageID, data.ChannelID).Limit(1).Find(&msg)
	if msg.ID == "" {
		return nil, nil
	}
	if msg.IsRevoked || msg.IsDeleted {
		return nil, nil
	}

	channel, _ := model.ChannelGet(data.ChannelID)
	if channel.ID == "" {
		return nil, nil
	}

	// 权限检查：是否为消息作者，或世界管理员代编辑
	isAuthor := msg.UserID == ctx.User.ID
	isAdminEdit := false
	editorUserName := strings.TrimSpace(ctx.User.Nickname)
	if editorUserName == "" {
		editorUserName = ctx.User.Username
	}
	if !isAuthor && channel.WorldID != "" {
		world, err := service.GetWorldByID(channel.WorldID)
		if err == nil && world != nil && world.AllowAdminEditMessages {
			if service.IsWorldAdmin(channel.WorldID, ctx.User.ID) {
				// 检查目标消息作者是否为非管理员
				if !service.IsWorldAdmin(channel.WorldID, msg.UserID) {
					isAdminEdit = true
				}
			}
		}
	}
	if !isAuthor && !isAdminEdit {
		return nil, nil
	}
	channelData := channel.ToProtocolType()

	var authorUser *model.UserModel
	if msg.UserID != "" && msg.UserID != ctx.User.ID {
		authorUser = model.UserGet(msg.UserID)
	} else {
		authorUser = ctx.User
	}

	authorMember, _ := model.MemberGetByUserIDAndChannelID(msg.UserID, data.ChannelID, msg.SenderMemberName)

	identityChanged := false
	var resolvedIdentityProto *protocol.ChannelIdentity
	if data.IdentityID != nil && isAuthor {
		identityChanged = true
		rawIdentityID := strings.TrimSpace(*data.IdentityID)
		identity, err := service.ChannelIdentityValidateMessageIdentity(ctx.User.ID, data.ChannelID, rawIdentityID)
		if err != nil {
			return nil, err
		}
		if identity != nil {
			msg.SenderIdentityID = identity.ID
			msg.SenderIdentityName = identity.DisplayName
			msg.SenderIdentityColor = identity.Color
			msg.SenderIdentityAvatarID = identity.AvatarAttachmentID
			msg.SenderRoleID = identity.ID
			resolvedIdentityProto = identity.ToProtocolType()
			if identity.DisplayName != "" {
				msg.SenderMemberName = identity.DisplayName
			}
		} else {
			msg.SenderIdentityID = ""
			msg.SenderIdentityName = ""
			msg.SenderIdentityColor = ""
			msg.SenderIdentityAvatarID = ""
			msg.SenderRoleID = ""
			resolvedIdentityProto = nil
			if authorMember != nil && authorMember.Nickname != "" {
				msg.SenderMemberName = authorMember.Nickname
			} else if authorUser != nil && authorUser.Nickname != "" {
				msg.SenderMemberName = authorUser.Nickname
			} else if authorUser != nil {
				msg.SenderMemberName = authorUser.Username
			}
		}
	}

	var quote model.MessageModel
	if msg.QuoteID != "" {
		db.Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, username, avatar, is_bot")
		}).Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, channel_id, user_id")
		}).Where("id = ?", msg.QuoteID).Limit(1).Find(&quote)
		if quote.WhisperTo != "" {
			quote.WhisperTarget = loadWhisperTargetForChannel(data.ChannelID, quote.WhisperTo)
		}
		if quote.IsWhisper {
			quote.WhisperTargets = loadWhisperTargetsForMessage(data.ChannelID, quote.ID, quote.WhisperTarget)
		}
	}

	if msg.WhisperTo != "" {
		msg.WhisperTarget = loadWhisperTargetForChannel(data.ChannelID, msg.WhisperTo)
	}
	if msg.IsWhisper {
		msg.WhisperTargets = loadWhisperTargetsForMessage(data.ChannelID, msg.ID, msg.WhisperTarget)
	}

	existingRolls, err := model.MessageDiceRollListByMessageID(msg.ID)
	if err != nil {
		return nil, err
	}
	newContent := data.Content
	var renderResult *service.DiceRenderResult
	if channel.BuiltInDiceEnabled {
		renderResult, err = service.RenderDiceContent(newContent, channel.DefaultDiceExpr, existingRolls)
		if err != nil {
			return nil, err
		}
		if renderResult != nil {
			newContent = renderResult.Content
		}
	}

	buildMessage := func() *protocol.Message {
		messageData := msg.ToProtocolType2(channelData)
		messageData.Content = msg.Content
		if authorUser != nil {
			messageData.User = authorUser.ToProtocolType()
		} else if msg.UserID != "" {
			messageData.User = &protocol.User{ID: msg.UserID}
		}
		if authorMember != nil {
			messageData.Member = authorMember.ToProtocolType()
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
	icModeChanged := false
	if icMode != "" && icMode != msg.ICMode {
		icModeChanged = true
	}
	if prevContent == newContent && !icModeChanged {
		return &struct {
			Message *protocol.Message `json:"message"`
		}{Message: buildMessage()}, nil
	}

	if prevContent != newContent {
		history := model.MessageEditHistoryModel{
			MessageID:    msg.ID,
			EditorID:     ctx.User.ID,
			PrevContent:  prevContent,
			ChannelID:    msg.ChannelID,
			EditedUserID: msg.UserID,
		}
		db.Create(&history)
		msg.Content = newContent
	}
	if icModeChanged {
		msg.ICMode = icMode
	}
	msg.IsEdited = true
	msg.EditCount = msg.EditCount + 1
	msg.UpdatedAt = time.Now()
	updates := map[string]any{
		"is_edited":  msg.IsEdited,
		"edit_count": msg.EditCount,
		"updated_at": msg.UpdatedAt,
	}
	if prevContent != newContent {
		updates["content"] = msg.Content
	}
	if icModeChanged {
		updates["ic_mode"] = msg.ICMode
	}
	if identityChanged {
		updates["sender_identity_id"] = msg.SenderIdentityID
		updates["sender_identity_name"] = msg.SenderIdentityName
		updates["sender_identity_color"] = msg.SenderIdentityColor
		updates["sender_identity_avatar_id"] = msg.SenderIdentityAvatarID
		updates["sender_member_name"] = msg.SenderMemberName
		updates["sender_role_id"] = msg.SenderRoleID
	}
	updates["edited_by_user_id"] = ctx.User.ID
	updates["edited_by_user_name"] = editorUserName
	msg.EditedByUserID = ctx.User.ID
	msg.EditedByUserName = editorUserName
	err = db.Model(&model.MessageModel{}).Where("id = ?", msg.ID).Updates(updates).Error
	if err != nil {
		return nil, err
	}
	if renderResult != nil {
		if err := model.MessageDiceRollReplace(msg.ID, renderResult.Rolls); err != nil {
			return nil, err
		}
	}

	messageData := buildMessage()
	if identityChanged {
		if messageData.Member == nil {
			messageData.Member = &protocol.GuildMember{
				ID:   msg.MemberID,
				User: ctx.User.ToProtocolType(),
			}
		}
		if messageData.Member.User == nil {
			messageData.Member.User = ctx.User.ToProtocolType()
		}
		if msg.SenderMemberName != "" {
			messageData.Member.Nick = msg.SenderMemberName
		}
		messageData.Member.Identity = resolvedIdentityProto
	}

	ev := &protocol.Event{
		Type:    protocol.EventMessageUpdated,
		Message: messageData,
		Channel: channelData,
		User:    ctx.User.ToProtocolType(),
	}

	if msg.IsWhisper {
		recipients := []string{ctx.User.ID}
		if msg.WhisperTo != "" {
			recipients = append(recipients, msg.WhisperTo)
		}
		recipientIDs := model.GetWhisperRecipientIDs(msg.ID)
		if len(recipientIDs) > 0 {
			recipients = append(recipients, recipientIDs...)
		}
		recipients = lo.Uniq(recipients)
		ctx.BroadcastEventInChannelToUsers(data.ChannelID, recipients, ev)
	} else {
		ctx.BroadcastEventInChannel(data.ChannelID, ev)
		ctx.BroadcastEventInChannelForBot(data.ChannelID, ev)
	}

	_ = model.WebhookEventLogAppendForMessage(data.ChannelID, "message-updated", msg.ID)

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
	if msg.IsDeleted {
		return nil, fmt.Errorf("该消息已删除，无法调整顺序")
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
		if err := db.Where("id = ? AND channel_id = ? AND is_deleted = ?", data.BeforeID, data.ChannelID, false).Limit(1).Find(&beforeMsg).Error; err != nil {
			return nil, err
		}
		if beforeMsg.ID == "" {
			return nil, fmt.Errorf("before_id 指定的消息不存在")
		}
	}

	if data.AfterID != "" && data.AfterID != data.MessageID {
		if err := db.Where("id = ? AND channel_id = ? AND is_deleted = ?", data.AfterID, data.ChannelID, false).Limit(1).Find(&afterMsg).Error; err != nil {
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
		msg.WhisperTarget = loadWhisperTargetForChannel(data.ChannelID, msg.WhisperTo)
	}
	if msg.IsWhisper {
		msg.WhisperTargets = loadWhisperTargetsForMessage(data.ChannelID, msg.ID, msg.WhisperTarget)
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

	if msg.IsWhisper {
		recipients := []string{ctx.User.ID}
		if msg.WhisperTo != "" {
			recipients = append(recipients, msg.WhisperTo)
		}
		recipientIDs := model.GetWhisperRecipientIDs(msg.ID)
		if len(recipientIDs) > 0 {
			recipients = append(recipients, recipientIDs...)
		}
		recipients = lo.Uniq(recipients)
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

func normalizeIcMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "ooc" {
		return "ooc"
	}
	return "ic"
}

func apiMessageTyping(ctx *ChatContext, data *struct {
	ChannelID   string   `json:"channel_id"`
	State       string   `json:"state"`
	Content     string   `json:"content"`
	MessageID   string   `json:"message_id"`
	Mode        string   `json:"mode"`
	Enabled     *bool    `json:"enabled"`
	IdentityID  string   `json:"identity_id"`
	WhisperTo   string   `json:"whisper_to"`
	ICModeSnake string   `json:"ic_mode"`
	ICModeCamel string   `json:"icMode"`
	OrderKey    *float64 `json:"order_key"`
}) (any, error) {
	channelId := data.ChannelID
	var privateOtherUser string
	if len(channelId) < 30 {
		if !pm.CanWithChannelRole(ctx.User.ID, channelId, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
			return nil, nil
		}
	} else {
		fr, _ := model.FriendRelationGetByID(channelId)
		if fr.ID == "" {
			return nil, nil
		}
		privateOtherUser = fr.UserID1
		if fr.UserID1 == ctx.User.ID {
			privateOtherUser = fr.UserID2
		}
	}

	if ctx.ConnInfo == nil || ctx.ConnInfo.ChannelId != channelId {
		return &struct {
			Success bool `json:"success"`
		}{Success: false}, nil
	}

	// 富文本 JSON 不截断，否则会破坏 JSON 结构导致无法渲染
	if !service.LooksLikeTipTapJSON(data.Content) {
		runes := []rune(data.Content)
		if len(runes) > 3000 {
			data.Content = string(runes[:3000])
		}
	}

	now := time.Now().UnixMilli()
	const typingThrottleGap int64 = 250

	state := normalizeTypingState(data.State, data.Enabled)
	rawIcMode := data.ICModeSnake
	if rawIcMode == "" {
		rawIcMode = data.ICModeCamel
	}
	typingTone := normalizeIcMode(rawIcMode)

	isActive := state != protocol.TypingStateSilent

	var broadcastOrderKey float64
	if data.OrderKey != nil && *data.OrderKey > 0 {
		broadcastOrderKey = *data.OrderKey
	}
	if isActive {
		if ctx.ConnInfo.TypingEnabled &&
			ctx.ConnInfo.TypingState == state &&
			now-ctx.ConnInfo.TypingUpdatedAt < typingThrottleGap &&
			ctx.ConnInfo.TypingContent == data.Content &&
			ctx.ConnInfo.TypingWhisperTo == data.WhisperTo &&
			ctx.ConnInfo.TypingIcMode == typingTone &&
			ctx.ConnInfo.TypingIdentityID == data.IdentityID &&
			(broadcastOrderKey == 0 || ctx.ConnInfo.TypingOrderKey == broadcastOrderKey) {
			return &struct {
				Success bool `json:"success"`
			}{Success: true}, nil
		}
		ctx.ConnInfo.TypingEnabled = true
		ctx.ConnInfo.TypingState = state
		ctx.ConnInfo.TypingContent = data.Content
		ctx.ConnInfo.TypingWhisperTo = data.WhisperTo
		ctx.ConnInfo.TypingUpdatedAt = now
		ctx.ConnInfo.TypingIcMode = typingTone
		ctx.ConnInfo.TypingIdentityID = data.IdentityID
		if broadcastOrderKey > 0 {
			ctx.ConnInfo.TypingOrderKey = broadcastOrderKey
		}
	} else {
		ctx.ConnInfo.TypingEnabled = false
		ctx.ConnInfo.TypingState = protocol.TypingStateSilent
		ctx.ConnInfo.TypingContent = ""
		ctx.ConnInfo.TypingWhisperTo = ""
		ctx.ConnInfo.TypingUpdatedAt = 0
		ctx.ConnInfo.TypingIcMode = "ic"
		ctx.ConnInfo.TypingIdentityID = ""
		ctx.ConnInfo.TypingOrderKey = 0
	}

	channel, _ := model.ChannelGet(channelId)
	if channel.ID == "" {
		return nil, nil
	}
	channelData := channel.ToProtocolType()
	member, _ := model.MemberGetByUserIDAndChannelID(ctx.User.ID, channelId, ctx.User.Nickname)

	var whisperUser *model.UserModel
	if data.WhisperTo != "" {
		if data.WhisperTo == ctx.User.ID {
			return nil, nil
		}
		if len(channelId) < 30 {
			mem, _ := model.MemberGetByUserIDAndChannelIDBase(data.WhisperTo, channelId, "", false)
			if mem == nil {
				return nil, nil
			}
		} else if data.WhisperTo != privateOtherUser {
			return nil, nil
		}
		whisperUser = model.UserGet(data.WhisperTo)
		if whisperUser == nil {
			return nil, nil
		}
	}

	content := data.Content
	if state == protocol.TypingStateIndicator {
		content = ""
	}

	if broadcastOrderKey == 0 {
		broadcastOrderKey = ctx.ConnInfo.TypingOrderKey
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
	event.Typing.ICMode = typingTone
	event.Typing.Tone = typingTone
	if member != nil {
		event.Member = member.ToProtocolType()
	}
	if broadcastOrderKey > 0 {
		event.Typing.OrderKey = broadcastOrderKey
	}
	if data.IdentityID != "" {
		identity, _ := model.ChannelIdentityGetByID(data.IdentityID)
		if identity != nil && identity.ChannelID == channelId && identity.UserID == ctx.User.ID {
			event.Member.Identity = identity.ToProtocolType()
		}
	}

	if whisperUser != nil {
		event.Typing.TargetUserID = whisperUser.ID
		ctx.BroadcastEventInChannelToUsers(channelId, []string{whisperUser.ID}, event)
	} else {
		ctx.BroadcastEventInChannelExcept(channelId, []string{ctx.User.ID}, event)
	}

	return &struct {
		Success bool `json:"success"`
	}{Success: true}, nil
}

func resolveBotMessageContext(ctx *ChatContext, channelId string) *protocol.MessageContext {
	if ctx == nil || ctx.User == nil || !ctx.User.IsBot || ctx.ConnInfo == nil {
		return nil
	}
	if ctx.ConnInfo.BotLastMessageContext == nil {
		return nil
	}
	msgContext, ok := ctx.ConnInfo.BotLastMessageContext.Load(channelId)
	if !ok {
		return nil
	}
	return msgContext
}

func resolveBotHiddenDicePending(ctx *ChatContext, channelId string) *BotHiddenDicePending {
	if ctx == nil || ctx.User == nil || !ctx.User.IsBot || ctx.ConnInfo == nil {
		return nil
	}
	if ctx.ConnInfo.BotHiddenDicePending == nil {
		return nil
	}
	pending, ok := ctx.ConnInfo.BotHiddenDicePending.Load(channelId)
	if !ok {
		return nil
	}
	return pending
}

func builtinSealBotSolve(ctx *ChatContext, data *struct {
	ChannelID string `json:"channel_id"`
	QuoteID   string `json:"quote_id"`
	Content   string `json:"content"`
	WhisperTo string `json:"whisper_to"`
	ClientID  string `json:"client_id"`
	ICMode    string `json:"ic_mode"`
}, channelData *protocol.Channel, isHiddenDice bool) {
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

		msgICMode := strings.TrimSpace(strings.ToLower(data.ICMode))
		if msgICMode == "" {
			msgICMode = "ic"
		}

		m := model.MessageModel{
			StringPKBaseModel: model.StringPKBaseModel{
				ID: utils.NewID(),
			},
			UserID:    "BOT:1000",
			ChannelID: data.ChannelID,
			MemberID:  "BOT:1000",
			Content:   botText,
			ICMode:    msgICMode,
		}
		if isHiddenDice && len(data.ChannelID) < 30 {
			m.IsWhisper = true
			m.WhisperTo = ctx.User.ID
			m.WhisperTarget = ctx.User
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

		if m.IsWhisper {
			ctx.BroadcastEventInChannelToUsers(data.ChannelID, []string{ctx.User.ID}, &protocol.Event{
				// 协议规定: 事件中必须含有 channel，message，user
				Type:    protocol.EventMessageCreated,
				Message: messageData,
				Channel: channelData,
				User:    userData,
			})
		} else {
			ctx.BroadcastEventInChannel(data.ChannelID, &protocol.Event{
				// 协议规定: 事件中必须含有 channel，message，user
				Type:    protocol.EventMessageCreated,
				Message: messageData,
				Channel: channelData,
				User:    userData,
			})
		}

		_ = model.WebhookEventLogAppendForMessage(data.ChannelID, "message-created", m.ID)
	}
}

func sendHiddenDicePrivateCopy(ctx *ChatContext, sourceChannel *protocol.Channel, originalMsg *protocol.Message) {
	if ctx == nil || ctx.User == nil || ctx.User.ID == "" || sourceChannel == nil || originalMsg == nil {
		return
	}
	const botID = "BOT:1000"
	botUser := model.UserGet(botID)
	if botUser == nil {
		return
	}
	ch, _ := model.ChannelPrivateGet(ctx.User.ID, botID)
	if ch == nil || ch.ID == "" {
		var isNew bool
		ch, isNew = model.ChannelPrivateNew(ctx.User.ID, botID)
		if ch == nil || ch.ID == "" {
			return
		}
		if isNew {
			if f := model.FriendRelationGet(ctx.User.ID, botID); f.ID != "" {
				model.FriendRelationSetVisible(ctx.User.ID, botID)
			} else {
				_ = model.FriendRelationCreate(ctx.User.ID, botID, false)
			}
		}
	}
	dmChannel := ch.ToProtocolType()
	content := fmt.Sprintf("暗骰结果 (来自 #%s)\n%s", sourceChannel.Name, originalMsg.Content)
	msgICMode := strings.TrimSpace(strings.ToLower(originalMsg.IcMode))
	if msgICMode == "" {
		msgICMode = "ic"
	}
	m := model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID: utils.NewID(),
		},
		UserID:    botID,
		ChannelID: ch.ID,
		MemberID:  botID,
		Content:   content,
		ICMode:    msgICMode,
	}
	model.GetDB().Create(&m)
	botNick := strings.TrimSpace(botUser.Nickname)
	if botNick == "" {
		botNick = "小海豹"
	}
	userData := &protocol.User{
		ID:     botID,
		Nick:   botNick,
		Avatar: botUser.Avatar,
		IsBot:  true,
	}
	messageData := m.ToProtocolType2(dmChannel)
	messageData.User = userData
	messageData.Member = &protocol.GuildMember{
		Name: userData.Nick,
		Nick: userData.Nick,
	}
	ctx.BroadcastEventInChannelToUsers(ch.ID, []string{ctx.User.ID}, &protocol.Event{
		Type:    protocol.EventMessageCreated,
		Message: messageData,
		Channel: dmChannel,
		User:    userData,
	})
	_ = model.WebhookEventLogAppendForMessage(ch.ID, "message-created", m.ID)
}

func forwardHiddenDiceWhisperCopy(ctx *ChatContext, sourceChannel *model.ChannelModel, msg *model.MessageModel, privateOtherUser string) {
	if ctx == nil || ctx.User == nil || !ctx.User.IsBot || sourceChannel == nil || msg == nil {
		return
	}
	if sourceChannel.PermType != "private" {
		return
	}
	if privateOtherUser == "" {
		return
	}
	if !strings.Contains(msg.Content, "暗骰") {
		return
	}
	match := hiddenDiceForwardPattern.FindStringSubmatch(msg.Content)
	if len(match) < 2 {
		return
	}
	targetChannelID := strings.TrimSpace(match[1])
	if targetChannelID == "" {
		return
	}
	if pending := resolveBotHiddenDicePending(ctx, targetChannelID); pending != nil && pending.TargetUserID == privateOtherUser {
		if ctx.ConnInfo != nil && ctx.ConnInfo.BotHiddenDicePending != nil {
			ctx.ConnInfo.BotHiddenDicePending.Delete(targetChannelID)
		}
	}
	targetChannel, _ := model.ChannelGet(targetChannelID)
	if targetChannel == nil || targetChannel.ID == "" {
		return
	}
	member, err := model.MemberGetByUserIDAndChannelIDBase(ctx.User.ID, targetChannelID, ctx.User.Nickname, true)
	if err != nil || member == nil {
		return
	}
	whisperUser := model.UserGet(privateOtherUser)
	if whisperUser == nil {
		return
	}
	var whisperMember *model.MemberModel
	if len(targetChannelID) < 30 {
		whisperMember, _ = model.MemberGetByUserIDAndChannelIDBase(privateOtherUser, targetChannelID, "", false)
	}
	setUserNickFromMember(whisperUser, whisperMember)

	msgICMode := strings.TrimSpace(strings.ToLower(msg.ICMode))
	if botCtx := resolveBotMessageContext(ctx, targetChannelID); botCtx != nil && botCtx.ICMode != "" {
		msgICMode = strings.TrimSpace(strings.ToLower(botCtx.ICMode))
	}
	if msgICMode == "" {
		msgICMode = "ic"
	}

	now := time.Now()
	var existingCount int64
	model.GetDB().Model(&model.MessageModel{}).
		Where("channel_id = ? AND user_id = ? AND is_whisper = ? AND whisper_to = ? AND content = ? AND created_at >= ?",
			targetChannelID, ctx.User.ID, true, privateOtherUser, msg.Content, now.Add(-5*time.Second)).
		Count(&existingCount)
	if existingCount > 0 {
		return
	}
	nowMs := now.UnixMilli()
	m := model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID: utils.NewID(),
		},
		UserID:           ctx.User.ID,
		ChannelID:        targetChannelID,
		MemberID:         member.ID,
		Content:          msg.Content,
		DisplayOrder:     float64(nowMs),
		ICMode:           msgICMode,
		SenderMemberName: member.Nickname,
		IsWhisper:        true,
		WhisperTo:        privateOtherUser,
	}
	m.WhisperTarget = whisperUser
	m.WhisperSenderUserNick = ctx.User.Nickname
	m.WhisperSenderUserName = ctx.User.Username
	m.WhisperSenderMemberID = member.ID
	m.WhisperSenderMemberName = m.SenderMemberName
	m.WhisperTargetUserNick = whisperUser.Nickname
	m.WhisperTargetUserName = whisperUser.Username
	if whisperMember != nil {
		m.WhisperTargetMemberID = whisperMember.ID
		m.WhisperTargetMemberName = whisperMember.Nickname
	}

	if err := model.GetDB().Create(&m).Error; err != nil {
		return
	}
	channelData := targetChannel.ToProtocolType()
	userData := ctx.User.ToProtocolType()
	messageData := m.ToProtocolType2(channelData)
	messageData.Content = msg.Content
	messageData.User = userData
	messageData.Member = member.ToProtocolType()
	messageData.WhisperTo = whisperUser.ToProtocolType()

	recipients := lo.Uniq([]string{ctx.User.ID, whisperUser.ID})
	ctx.BroadcastEventInChannelToUsers(targetChannelID, recipients, &protocol.Event{
		Type:    protocol.EventMessageCreated,
		Message: messageData,
		Channel: channelData,
		User:    userData,
	})
	_ = model.WebhookEventLogAppendForMessage(targetChannelID, "message-created", m.ID)
}

func resolveWhisperRecipients(whisperTo string, whisperToIds []string, senderID string) []string {
	idSet := make(map[string]struct{})
	if whisperTo != "" && whisperTo != senderID {
		idSet[whisperTo] = struct{}{}
	}
	for _, id := range whisperToIds {
		id = strings.TrimSpace(id)
		if id != "" && id != senderID {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return nil
	}
	result := make([]string, 0, len(idSet))
	for id := range idSet {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func apiUnreadCount(ctx *ChatContext, data *struct {
	WorldID        string `json:"world_id"`
	IncludePrivate *bool  `json:"include_private"`
}) (any, error) {
	worldID := strings.TrimSpace(data.WorldID)
	includePrivate := false
	if data.IncludePrivate != nil {
		includePrivate = *data.IncludePrivate
	}
	var chIds []string
	if worldID == "" {
		chIds, _ = service.ChannelIdList(ctx.User.ID)
	} else {
		chIds, _ = service.ChannelIdListByWorld(ctx.User.ID, worldID, includePrivate)
	}
	lst, err := model.ChannelUnreadFetch(chIds, ctx.User.ID)
	if err != nil {
		return nil, err
	}
	return lst, err
}

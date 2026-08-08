package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strings"

	"sealchat/model"
	"sealchat/pkg/contentstats"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/service/metrics"
	"sealchat/utils"

	"gorm.io/gorm"
)

const (
	messageForwardMaxSources = 50
	messageForwardMaxTargets = 20
	messageForwardMaxPairs   = 500
)

type messageForwardBatchTarget struct {
	ChannelID         string `json:"channel_id"`
	IdentityID        string `json:"identity_id"`
	IdentityVariantID string `json:"identity_variant_id"`
}

type messageForwardBatchRequest struct {
	SourceChannelID  string                      `json:"source_channel_id"`
	MessageIDs       []string                    `json:"message_ids"`
	Targets          []messageForwardBatchTarget `json:"targets"`
	ClientID         string                      `json:"client_id"`
	WhisperConfirmed bool                        `json:"whisper_confirmed"`
}

type messageForwardPreparedTarget struct {
	channel     *model.ChannelModel
	channelData *protocol.Channel
	member      *model.MemberModel
	identity    *model.ChannelIdentityModel
	appearance  *service.ResolvedIdentityAppearance
}

type messageForwardCreated struct {
	sourceMessageID string
	target          *messageForwardPreparedTarget
	message         *model.MessageModel
	newMessage      bool
}

func messageForwardPairClientID(operationID, sourceMessageID, targetChannelID string) string {
	raw := operationID + "\x00" + sourceMessageID + "\x00" + targetChannelID
	sum := sha256.Sum256([]byte(raw))
	return "forward:" + hex.EncodeToString(sum[:])
}

func apiMessageForwardBatch(ctx *ChatContext, data *messageForwardBatchRequest) (any, error) {
	if ctx == nil || ctx.User == nil || ctx.IsReadOnly() {
		return nil, fmt.Errorf("当前用户无法转发消息")
	}

	sourceChannelID := strings.TrimSpace(data.SourceChannelID)
	if sourceChannelID == "" {
		return nil, fmt.Errorf("缺少源频道")
	}

	messageIDs := make([]string, 0, len(data.MessageIDs))
	seenMessageIDs := make(map[string]struct{}, len(data.MessageIDs))
	for _, rawID := range data.MessageIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, exists := seenMessageIDs[id]; exists {
			continue
		}
		seenMessageIDs[id] = struct{}{}
		messageIDs = append(messageIDs, id)
	}
	if len(messageIDs) == 0 {
		return nil, fmt.Errorf("未选择消息")
	}
	if len(messageIDs) > messageForwardMaxSources {
		return nil, fmt.Errorf("单次最多转发%d条消息", messageForwardMaxSources)
	}

	operationID := strings.TrimSpace(data.ClientID)
	if operationID == "" {
		operationID = "message-forward-" + utils.NewID()
	}

	db := model.GetDB()
	sourceChannel, err := model.ChannelGet(sourceChannelID)
	if err != nil || sourceChannel == nil || sourceChannel.ID == "" {
		return nil, fmt.Errorf("源频道不存在")
	}
	if sourceChannel.WorldID == "" || sourceChannel.IsPrivate || strings.EqualFold(sourceChannel.PermType, "private") || service.IsChannelDeletedForAccess(sourceChannel) {
		return nil, fmt.Errorf("仅支持世界频道消息转发")
	}
	if !pm.CanWithChannelRole(ctx.User.ID, sourceChannelID, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
		return nil, fmt.Errorf("无权限读取源频道")
	}

	canReadAllWhispers := canUserReadAllWhispersInChannel(ctx.User.ID, sourceChannelID)
	var sourceMessages []*model.MessageModel
	q := db.Where("channel_id = ? AND id IN ? AND (is_deleted = ? OR is_deleted IS NULL) AND (is_revoked = ? OR is_revoked IS NULL)", sourceChannelID, messageIDs, false, false)
	q = applyWhisperVisibilityFilterWithReadAll(q, ctx.User.ID, canReadAllWhispers)
	if err := q.Find(&sourceMessages).Error; err != nil {
		return nil, err
	}
	if len(sourceMessages) != len(messageIDs) {
		return nil, fmt.Errorf("部分源消息不存在、已撤回或当前用户不可见")
	}
	sort.SliceStable(sourceMessages, func(i, j int) bool {
		left, right := sourceMessages[i], sourceMessages[j]
		if left.DisplayOrder != right.DisplayOrder {
			return left.DisplayOrder < right.DisplayOrder
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.ID < right.ID
	})
	for _, source := range sourceMessages {
		if source.IsWhisper && !data.WhisperConfirmed {
			return nil, fmt.Errorf("源消息包含悄悄话，请确认公开转发")
		}
	}

	preparedTargets := make([]*messageForwardPreparedTarget, 0, len(data.Targets))
	seenTargets := make(map[string]struct{}, len(data.Targets))
	for _, rawTarget := range data.Targets {
		target := messageForwardBatchTarget{
			ChannelID:         strings.TrimSpace(rawTarget.ChannelID),
			IdentityID:        strings.TrimSpace(rawTarget.IdentityID),
			IdentityVariantID: strings.TrimSpace(rawTarget.IdentityVariantID),
		}
		if target.ChannelID == "" {
			return nil, fmt.Errorf("目标频道不能为空")
		}
		if _, exists := seenTargets[target.ChannelID]; exists {
			return nil, fmt.Errorf("目标频道不能重复：%s", target.ChannelID)
		}
		seenTargets[target.ChannelID] = struct{}{}
		if len(preparedTargets) >= messageForwardMaxTargets {
			return nil, fmt.Errorf("单次最多转发到%d个频道", messageForwardMaxTargets)
		}

		channel, channelErr := model.ChannelGet(target.ChannelID)
		if channelErr != nil || channel == nil || channel.ID == "" {
			return nil, fmt.Errorf("目标频道不存在：%s", target.ChannelID)
		}
		if target.ChannelID == sourceChannelID {
			return nil, fmt.Errorf("目标频道必须不同于源频道")
		}
		if channel.WorldID == "" || channel.IsPrivate || strings.EqualFold(channel.PermType, "private") || service.IsChannelDeletedForAccess(channel) {
			return nil, fmt.Errorf("目标频道不是可转发的世界频道：%s", target.ChannelID)
		}
		if len(sourceMessages)*len(preparedTargets) >= messageForwardMaxPairs {
			return nil, fmt.Errorf("单次转发消息数量超过限制")
		}
		var world model.WorldModel
		if err := db.Where("id = ?", channel.WorldID).Limit(1).Find(&world).Error; err != nil || world.ID == "" || strings.EqualFold(world.Status, "deleted") {
			return nil, fmt.Errorf("目标世界不存在：%s", channel.WorldID)
		}
		if channel.WorldID != sourceChannel.WorldID && world.OwnerID != ctx.User.ID {
			return nil, fmt.Errorf("只能转发到当前世界或自己拥有的其他世界")
		}
		if !pm.CanWithChannelRole(ctx.User.ID, channel.ID, pm.PermFuncChannelTextSend, pm.PermFuncChannelTextSendAll) {
			return nil, fmt.Errorf("无权限在目标频道发言：%s", channel.Name)
		}
		member, memberErr := model.MemberGetByUserIDAndChannelID(ctx.User.ID, channel.ID, ctx.User.Nickname)
		if memberErr != nil {
			return nil, memberErr
		}
		if member == nil || member.ID == "" {
			return nil, fmt.Errorf("用户不在目标频道：%s", channel.Name)
		}
		identity, identityErr := service.ChannelIdentityValidateMessageIdentity(ctx.User.ID, channel.ID, target.IdentityID)
		if identityErr != nil {
			return nil, identityErr
		}
		if identity == nil {
			identity, identityErr = service.EnsureHiddenDefaultIdentity(ctx.User.ID, channel.ID)
			if identityErr != nil {
				return nil, identityErr
			}
		}
		variant, variantErr := service.ChannelIdentityVariantValidateMessageVariant(ctx.User.ID, channel.ID, identity, target.IdentityVariantID)
		if variantErr != nil {
			return nil, variantErr
		}
		preparedTargets = append(preparedTargets, &messageForwardPreparedTarget{
			channel:     channel,
			channelData: channel.ToProtocolType(),
			member:      member,
			identity:    identity,
			appearance:  service.ResolveChannelIdentityAppearance(identity, variant),
		})
	}
	if len(preparedTargets) == 0 {
		return nil, fmt.Errorf("未选择目标频道")
	}
	if len(sourceMessages)*len(preparedTargets) > messageForwardMaxPairs {
		return nil, fmt.Errorf("单次转发消息数量超过限制")
	}

	created := make([]messageForwardCreated, 0, len(sourceMessages)*len(preparedTargets))
	for attempt := 0; attempt < 3; attempt++ {
		created = created[:0]
		err = db.Transaction(func(tx *gorm.DB) error {
			for _, target := range preparedTargets {
				var maxOrder float64
				if err := tx.Model(&model.MessageModel{}).
					Where("channel_id = ?", target.channel.ID).
					Select("COALESCE(MAX(display_order), 0)").
					Scan(&maxOrder).Error; err != nil {
					return err
				}
				nextOrder := maxOrder
				for _, source := range sourceMessages {
					pairClientID := messageForwardPairClientID(operationID, source.ID, target.channel.ID)
					var existing model.MessageModel
					if err := tx.Where("channel_id = ? AND user_id = ? AND client_id = ?", target.channel.ID, ctx.User.ID, pairClientID).
						Limit(1).Find(&existing).Error; err != nil {
						return err
					}
					if existing.ID != "" {
						created = append(created, messageForwardCreated{sourceMessageID: source.ID, target: target, message: &existing})
						continue
					}

					nextOrder += displayOrderGap
					icMode := strings.ToLower(strings.TrimSpace(source.ICMode))
					if icMode != "ic" && icMode != "ooc" {
						icMode = "ic"
					}
					m := model.MessageModel{
						StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
						UserID:            ctx.User.ID,
						ChannelID:         target.channel.ID,
						MemberID:          target.member.ID,
						Content:           source.Content,
						VisibleCharCount:  contentstats.CountVisibleTextChars(source.Content),
						WidgetData:        service.BuildStateWidgetDataFromContentWithPrevious(source.Content, source.WidgetData),
						DisplayOrder:      nextOrder,
						ICMode:            icMode,
						SenderMemberName:  target.member.Nickname,
					}
					m.ClientID = &pairClientID
					if target.identity != nil {
						m.SenderRoleID = target.identity.ID
						m.SenderIdentityID = target.identity.ID
						m.SenderIdentityIsTemporary = target.identity.IsTemporary
						if target.appearance != nil {
							m.SenderIdentityVariantID = target.appearance.VariantID
							m.SenderIdentityName = target.appearance.DisplayName
							m.SenderIdentityColor = target.appearance.Color
							m.SenderIdentityAvatarID = target.appearance.AvatarAttachmentID
							m.SenderIdentityDecorations = target.appearance.AvatarDecorations
							m.SenderTheaterPresentation = target.appearance.TheaterPresentation
							if target.appearance.DisplayName != "" {
								m.SenderMemberName = target.appearance.DisplayName
							}
						}
					}
					if err := tx.Create(&m).Error; err != nil {
						return err
					}
					created = append(created, messageForwardCreated{sourceMessageID: source.ID, target: target, message: &m, newMessage: true})
				}
			}
			return nil
		})
		if err == nil || !isUniqueConstraintError(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	createdResult := make([]map[string]string, 0, len(created))
	for _, item := range created {
		if item.message == nil || item.message.ID == "" {
			continue
		}
		createdResult = append(createdResult, map[string]string{
			"source_message_id": item.sourceMessageID,
			"target_channel_id": item.target.channel.ID,
			"message_id":        item.message.ID,
		})
		if !item.newMessage {
			continue
		}
		item.target.member.UpdateRecentSent()
		item.target.channel.UpdateRecentSent()
		messageData := item.message.ToProtocolType2(item.target.channelData)
		messageData.Content = item.message.Content
		messageData.User = ctx.User.ToProtocolType()
		messageData.Member = item.target.member.ToProtocolType()
		messageData.Member.Roles = []string{service.ResolveMemberRoleForProtocol(ctx.User.ID, item.target.channel.ID, item.target.channel.WorldID)}
		event := &protocol.Event{
			Type:    protocol.EventMessageCreated,
			Message: messageData,
			Channel: item.target.channelData,
			User:    ctx.User.ToProtocolType(),
		}
		ctx.BroadcastEventInChannel(item.target.channel.ID, event)
		ctx.BroadcastEventInChannelForBot(item.target.channel.ID, event)
		if collector := metrics.Get(); collector != nil {
			collector.RecordMessage()
		}
		ctx.TagCheck(item.target.channel.ID, item.message.ID, item.message.Content)
		_ = model.WebhookEventLogAppendForMessage(item.target.channel.ID, "message-created", item.message.ID)
		notifyAppMessageCreated(item.message.ID)
		go func(channelID string, message model.MessageModel) {
			if err := service.RecordDigestWindowMessage(channelID, &message); err != nil {
				log.Printf("digest-push: 记录转发消息摘要窗口失败 channel=%s message=%s err=%v", channelID, message.ID, err)
			}
		}(item.target.channel.ID, *item.message)
		notifyForwardMessageCreated(ctx, item.target.channel.ID, item.message.Content)
	}

	return &struct {
		OperationID     string              `json:"operation_id"`
		SourceChannelID string              `json:"source_channel_id"`
		Created         []map[string]string `json:"created"`
		Failed          []any               `json:"failed"`
	}{
		OperationID:     operationID,
		SourceChannelID: sourceChannelID,
		Created:         createdResult,
		Failed:          []any{},
	}, nil
}

func notifyForwardMessageCreated(ctx *ChatContext, channelID, content string) {
	if ctx == nil || ctx.UserId2ConnInfo == nil || channelID == "" {
		return
	}
	var userIDs []string
	ctx.UserId2ConnInfo.Range(func(userID string, _ *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		userIDs = append(userIDs, userID)
		return true
	})
	var onlineUserIDs []string
	if ctx.ChannelUsersMap != nil {
		if channelUsers, exists := ctx.ChannelUsersMap.Load(channelID); exists && channelUsers != nil {
			channelUsers.Range(func(userID string) bool {
				onlineUserIDs = append(onlineUserIDs, userID)
				return true
			})
		}
	}
	_ = model.ChannelReadInitInBatches(channelID, userIDs)
	_ = model.ChannelReadSetInBatch([]string{channelID}, onlineUserIDs)
	onlineSet := make(map[string]struct{}, len(onlineUserIDs))
	for _, userID := range onlineUserIDs {
		onlineSet[userID] = struct{}{}
	}
	for _, userID := range userIDs {
		if userID == "" {
			continue
		}
		if _, online := onlineSet[userID]; online {
			continue
		}
		ctx.BroadcastToUserJSON(userID, buildMessageCreatedNoticePayload(channelID, content, userID))
	}
}

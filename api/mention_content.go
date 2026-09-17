package api

import (
	"sort"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

const worldMessageNoticePreviewRuneLimit = 120

var (
	worldNoticeChannelReadInitInBatches = model.ChannelReadInitInBatches
	worldNoticeChannelReadSetInBatch    = model.ChannelReadSetInBatch
)

type worldMessageNoticeSource struct {
	WorldID      string
	ChannelID    string
	ChannelName  string
	MessageID    string
	SenderUserID string
	SpeakerName  string
	Content      string
	CreatedAt    int64
}

type worldMessageNoticeTarget struct {
	connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]
	conn    *WsSyncConn
}

func collectMentionTargetIDsFromContent(content string) map[string]struct{} {
	return service.CollectMentionTargetIDsFromContent(content)
}

func worldMessageNoticeSourceFromProtocol(
	worldID string,
	channel *protocol.Channel,
	message *protocol.Message,
	senderUserID string,
	senderMemberName string,
) worldMessageNoticeSource {
	source := worldMessageNoticeSource{
		WorldID:      strings.TrimSpace(worldID),
		SenderUserID: strings.TrimSpace(senderUserID),
	}
	if channel != nil {
		source.ChannelID = strings.TrimSpace(channel.ID)
		source.ChannelName = strings.TrimSpace(channel.Name)
	}
	if message == nil {
		return source
	}
	if source.ChannelID == "" && message.Channel != nil {
		source.ChannelID = strings.TrimSpace(message.Channel.ID)
		source.ChannelName = strings.TrimSpace(message.Channel.Name)
	}
	source.MessageID = strings.TrimSpace(message.ID)
	source.Content = message.Content
	source.CreatedAt = message.CreatedAt
	if source.SenderUserID == "" && message.User != nil {
		source.SenderUserID = strings.TrimSpace(message.User.ID)
	}

	speakerCandidates := make([]string, 0, 6)
	if message.Identity != nil {
		speakerCandidates = append(speakerCandidates, message.Identity.DisplayName)
	}
	speakerCandidates = append(speakerCandidates, senderMemberName)
	if message.Member != nil {
		speakerCandidates = append(speakerCandidates, message.Member.Name, message.Member.Nick)
	}
	if message.User != nil {
		speakerCandidates = append(speakerCandidates, message.User.Nick, message.User.Name)
	}
	for _, candidate := range speakerCandidates {
		if name := strings.TrimSpace(candidate); name != "" {
			source.SpeakerName = name
			break
		}
	}
	return source
}

func buildWorldMessageNoticePreview(content string) string {
	preview := strings.TrimSpace(service.NormalizeMessageContentToPlainText(content))
	if preview == "" {
		return "发送了一条消息"
	}
	runes := []rune(preview)
	if len(runes) > worldMessageNoticePreviewRuneLimit {
		preview = string(runes[:worldMessageNoticePreviewRuneLimit])
	}
	return preview
}

func buildMessageCreatedNoticePayload(source worldMessageNoticeSource, recipientID string, mentionTargets map[string]struct{}, preview string) map[string]any {
	mentioned := false
	if recipientID != "" {
		_, mentioned = mentionTargets[recipientID]
		if !mentioned {
			_, mentioned = mentionTargets["all"]
		}
	}
	channelName := strings.TrimSpace(source.ChannelName)
	if channelName == "" {
		channelName = "未知频道"
	}
	speakerName := strings.TrimSpace(source.SpeakerName)
	if speakerName == "" {
		speakerName = "新消息"
	}
	createdAt := source.CreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().UnixMilli()
	}
	return map[string]any{
		"op":           0,
		"type":         "message-created-notice",
		"worldId":      source.WorldID,
		"channelId":    source.ChannelID,
		"messageId":    source.MessageID,
		"channelName":  channelName,
		"senderUserId": source.SenderUserID,
		"speakerName":  speakerName,
		"preview":      preview,
		"createdAt":    createdAt,
		"mentioned":    mentioned,
	}
}

func collectWorldMessageNoticeTargets(ctx *ChatContext, source worldMessageNoticeSource) (map[string][]worldMessageNoticeTarget, []string) {
	targets := make(map[string][]worldMessageNoticeTarget)
	if ctx == nil || ctx.UserId2ConnInfo == nil {
		return targets, nil
	}
	ctx.UserId2ConnInfo.Range(func(userID string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		userID = strings.TrimSpace(userID)
		if userID == "" || userID == source.SenderUserID || connMap == nil {
			return true
		}
		connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if conn != nil && info.canReceiveWorldMessageNotice(source.WorldID, source.ChannelID) {
				targets[userID] = append(targets[userID], worldMessageNoticeTarget{connMap: connMap, conn: conn})
			}
			return true
		})
		return true
	})

	userIDs := make([]string, 0, len(targets))
	for userID := range targets {
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)
	return targets, userIDs
}

func broadcastWorldMessageCreatedNotice(
	ctx *ChatContext,
	source worldMessageNoticeSource,
	mentionTargets map[string]struct{},
	sendNotice ...bool,
) {
	source.WorldID = strings.TrimSpace(source.WorldID)
	source.ChannelID = strings.TrimSpace(source.ChannelID)
	source.MessageID = strings.TrimSpace(source.MessageID)
	source.SenderUserID = strings.TrimSpace(source.SenderUserID)
	if ctx == nil || source.WorldID == "" || source.ChannelID == "" || source.MessageID == "" {
		return
	}

	targets, targetUserIDs := collectWorldMessageNoticeTargets(ctx, source)

	sourceChannelOnlineUserSet := make(map[string]struct{})
	if ctx.ChannelUsersMap != nil {
		if users, exists := ctx.ChannelUsersMap.Load(source.ChannelID); exists && users != nil {
			users.Range(func(userID string) bool {
				if userID = strings.TrimSpace(userID); userID != "" {
					sourceChannelOnlineUserSet[userID] = struct{}{}
				}
				return true
			})
		}
	}
	sourceChannelOnlineUserIDs := make([]string, 0, len(sourceChannelOnlineUserSet))
	readStateUserSet := make(map[string]struct{}, len(targetUserIDs)+len(sourceChannelOnlineUserSet))
	for _, userID := range targetUserIDs {
		readStateUserSet[userID] = struct{}{}
	}
	for userID := range sourceChannelOnlineUserSet {
		sourceChannelOnlineUserIDs = append(sourceChannelOnlineUserIDs, userID)
		readStateUserSet[userID] = struct{}{}
	}
	sort.Strings(sourceChannelOnlineUserIDs)
	readStateUserIDs := make([]string, 0, len(readStateUserSet))
	for userID := range readStateUserSet {
		readStateUserIDs = append(readStateUserIDs, userID)
	}
	sort.Strings(readStateUserIDs)
	if len(readStateUserIDs) > 0 {
		_ = worldNoticeChannelReadInitInBatches(source.ChannelID, readStateUserIDs)
	}
	if len(sourceChannelOnlineUserIDs) > 0 {
		_ = worldNoticeChannelReadSetInBatch([]string{source.ChannelID}, sourceChannelOnlineUserIDs)
	}

	if len(sendNotice) > 0 && !sendNotice[0] {
		return
	}
	preview := buildWorldMessageNoticePreview(source.Content)
	for _, userID := range targetUserIDs {
		payload := buildMessageCreatedNoticePayload(source, userID, mentionTargets, preview)
		for _, target := range targets[userID] {
			writeConnJSONAndPrune(target.connMap, target.conn, payload)
		}
	}
}

package api

import (
	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

func collectMentionTargetIDsFromContent(content string) map[string]struct{} {
	return service.CollectMentionTargetIDsFromContent(content)
}

func buildMessageCreatedNoticePayload(channelID, content, recipientID, messageID, worldID string) map[string]any {
	mentioned := false
	if recipientID != "" {
		targets := collectMentionTargetIDsFromContent(content)
		_, mentioned = targets[recipientID]
		if !mentioned {
			_, mentioned = targets["all"]
		}
	}
	payload := map[string]any{
		"op":        0,
		"type":      "message-created-notice",
		"channelId": channelID,
		"mentioned": mentioned,
	}
	if messageID != "" {
		payload["messageId"] = messageID
	}
	if worldID != "" {
		payload["worldId"] = worldID
	}
	return payload
}

func broadcastMessageCreatedNoticeOutsideChannel(ctx *ChatContext, userID, channelID string, payload any) {
	if ctx == nil || userID == "" || channelID == "" || ctx.UserId2ConnInfo == nil {
		return
	}
	connMap, ok := ctx.UserId2ConnInfo.Load(userID)
	if !ok || connMap == nil {
		return
	}
	connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
		if info != nil && info.ChannelId != channelID {
			writeConnJSONAndPrune(connMap, conn, payload)
		}
		return true
	})
}

// broadcastMessageCreatedNoticeToUsers mirrors the existing unread-notice fanout
// for message producers that do not go through apiMessageCreate.
func broadcastMessageCreatedNoticeToUsers(ctx *ChatContext, channelID, content, messageID, worldID string) {
	if ctx == nil || ctx.UserId2ConnInfo == nil || channelID == "" {
		return
	}
	var userIDs []string
	ctx.UserId2ConnInfo.Range(func(userID string, _ *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		if userID != "" {
			userIDs = append(userIDs, userID)
		}
		return true
	})
	if len(userIDs) == 0 {
		return
	}

	var onlineUserIDs []string
	if ctx.ChannelUsersMap != nil {
		if users, exists := ctx.ChannelUsersMap.Load(channelID); exists && users != nil {
			users.Range(func(userID string) bool {
				if userID != "" {
					onlineUserIDs = append(onlineUserIDs, userID)
				}
				return true
			})
		}
	}
	_ = model.ChannelReadInitInBatches(channelID, userIDs)
	_ = model.ChannelReadSetInBatch([]string{channelID}, onlineUserIDs)
	for _, userID := range userIDs {
		broadcastMessageCreatedNoticeOutsideChannel(
			ctx,
			userID,
			channelID,
			buildMessageCreatedNoticePayload(channelID, content, userID, messageID, worldID),
		)
	}
}

package api

import "sealchat/service"

func collectMentionTargetIDsFromContent(content string) map[string]struct{} {
	return service.CollectMentionTargetIDsFromContent(content)
}

func buildMessageCreatedNoticePayload(channelID, content, recipientID string) map[string]any {
	mentioned := false
	if recipientID != "" {
		targets := collectMentionTargetIDsFromContent(content)
		_, mentioned = targets[recipientID]
		if !mentioned {
			_, mentioned = targets["all"]
		}
	}
	return map[string]any{
		"op":        0,
		"type":      "message-created-notice",
		"channelId": channelID,
		"mentioned": mentioned,
	}
}

package api

import (
	"strings"
	"time"

	"sealchat/protocol"
)

type channelIdentityRefreshPayload struct {
	ChannelID      string
	TargetUserID   string
	OperatorUserID string
	Reason         string
}

func buildChannelIdentityRefreshRecipients(targetUserID string, operatorUserID string) []string {
	recipients := make([]string, 0, 2)
	seen := map[string]struct{}{}
	for _, raw := range []string{operatorUserID, targetUserID} {
		userID := strings.TrimSpace(raw)
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		recipients = append(recipients, userID)
	}
	return recipients
}

func buildChannelIdentityRefreshEvent(payload channelIdentityRefreshPayload) *protocol.Event {
	channelID := strings.TrimSpace(payload.ChannelID)
	if channelID == "" {
		return nil
	}
	now := time.Now().UnixMilli()
	return &protocol.Event{
		Type: protocol.EventChannelIdentitiesUpdated,
		Channel: &protocol.Channel{
			ID: channelID,
		},
		Argv: &protocol.Argv{
			Options: map[string]interface{}{
				"channelId":      channelID,
				"targetUserId":   strings.TrimSpace(payload.TargetUserID),
				"operatorUserId": strings.TrimSpace(payload.OperatorUserID),
				"reason":         strings.TrimSpace(payload.Reason),
				"version":        now,
				"revision":       now,
				"forceReload":    true,
			},
		},
	}
}

func broadcastChannelIdentityRefresh(payload channelIdentityRefreshPayload) {
	if userId2ConnInfoGlobal == nil {
		return
	}
	event := buildChannelIdentityRefreshEvent(payload)
	if event == nil {
		return
	}
	recipients := buildChannelIdentityRefreshRecipients(payload.TargetUserID, payload.OperatorUserID)
	if len(recipients) == 0 {
		return
	}
	ctx := &ChatContext{
		ChannelUsersMap: getChannelUsersMap(),
		UserId2ConnInfo: getUserConnInfoMap(),
	}
	ctx.BroadcastEventInChannelToUsers(strings.TrimSpace(payload.ChannelID), recipients, event)
}

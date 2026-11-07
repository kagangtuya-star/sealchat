package api

import (
	"fmt"
	"strings"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
)

// At timestamp-sized orders, the epsilon can be smaller than one float64 ULP.
// Check the actual values that insertion will write, including batch inserts.
func hasDisplayOrderSpace(lower, upper float64, count int) bool {
	if count < 1 || upper <= lower+displayOrderEpsilon*float64(count+1) {
		return false
	}
	step := (upper - lower) / float64(count+1)
	previous := lower
	for i := 0; i < count; i++ {
		next := previous + step
		if !(next > previous && next < upper) {
			return false
		}
		previous = next
	}
	return true
}

func broadcastDisplayOrderReset(ctx *ChatContext, channelID string) {
	// Rebalancing also changes private/deleted rows. Never broadcast their IDs,
	// content or visibility: each client refreshes only its already-loaded IDs.
	ev := &protocol.Event{
		Type:    protocol.EventMessageReordered,
		Channel: &protocol.Channel{ID: channelID},
		Reorder: &protocol.MessageReorder{ChannelID: channelID, Reset: true},
	}
	ctx.BroadcastEventInChannel(channelID, ev)
	ctx.BroadcastEventInChannelForBot(channelID, ev)
}

type messageDisplayOrder struct {
	ID           string  `json:"id"`
	DisplayOrder float64 `json:"display_order"`
}

func apiMessageOrderGet(ctx *ChatContext, data *struct {
	ChannelID  string   `json:"channel_id"`
	MessageIDs []string `json:"message_ids"`
}) (any, error) {
	channelID := strings.TrimSpace(data.ChannelID)
	ids := collectMessageIDs(data.MessageIDs)
	if channelID == "" || len(ids) == 0 || len(ids) > 200 {
		return nil, fmt.Errorf("channel_id and 1-200 message_ids are required")
	}
	if ctx.IsReadOnly() {
		if _, err := checkReadOnlyChannelAccess(ctx, channelID); err != nil {
			return nil, err
		}
	} else if strings.Contains(channelID, ":") {
		friend, err := model.FriendRelationGetByID(channelID)
		if err != nil || friend == nil || (friend.UserID1 != ctx.User.ID && friend.UserID2 != ctx.User.ID) {
			return nil, fmt.Errorf("permission denied")
		}
	} else {
		channel, err := model.ChannelGet(channelID)
		if err != nil || channel == nil || channel.ID == "" || service.IsChannelDeletedForAccess(channel) ||
			!pm.CanWithChannelRole(ctx.User.ID, channelID, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) ||
			!service.CanReadChannelByUserId(ctx.User.ID, channelID) {
			return nil, fmt.Errorf("permission denied")
		}
	}
	items := make([]messageDisplayOrder, 0, len(ids))
	query := model.GetDB().Model(&model.MessageModel{}).
		Where("channel_id = ? AND id IN ? AND is_deleted = ?", channelID, ids, false)
	query = applyWhisperVisibilityFilterWithReadAll(query, ctx.User.ID, canUserReadAllWhispersInChannel(ctx.User.ID, channelID))
	if err := query.Select("id, display_order").Scan(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

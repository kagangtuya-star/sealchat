package api

import (
	"errors"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/protocol"
)

const onlineCharacterCardTTL = 2 * time.Minute

type characterOnlineCardRequestPayload struct {
	ChannelID string `json:"channel_id"`
}

type characterOnlineCardBroadcastPayload struct {
	ChannelID  string                            `json:"channel_id"`
	IdentityID string                            `json:"identity_id"`
	Card       *protocol.OnlineCharacterCardData `json:"card"`
	Action     string                            `json:"action"` // update/clear
}

type characterOnlineCardSnapshotPayload struct {
	ChannelID string `json:"channel_id"`
}

type onlineCharacterCardCache struct {
	sync.RWMutex
	items map[string]map[string]*protocol.OnlineCharacterCardItem
}

var onlineCharacterCardState = &onlineCharacterCardCache{
	items: map[string]map[string]*protocol.OnlineCharacterCardItem{},
}

func resetOnlineCharacterCardCacheForTest() {
	onlineCharacterCardState.Lock()
	defer onlineCharacterCardState.Unlock()
	onlineCharacterCardState.items = map[string]map[string]*protocol.OnlineCharacterCardItem{}
}

func apiCharacterOnlineCardRequest(ctx *ChatContext, data *characterOnlineCardRequestPayload) (any, error) {
	channelID := strings.TrimSpace(data.ChannelID)
	if err := ensureOnlineCardChannelAccess(ctx, channelID); err != nil {
		return nil, err
	}
	requesterID := strings.TrimSpace(ctx.User.ID)
	ctx.BroadcastEventInChannelExcept(channelID, []string{requesterID}, &protocol.Event{
		Type:    protocol.EventCharacterOnlineCardRequested,
		Channel: &protocol.Channel{ID: channelID},
		OnlineCharacterCardRequest: &protocol.OnlineCharacterCardRequestPayload{
			RequesterID: requesterID,
		},
	})
	return map[string]any{"ok": true}, nil
}

func apiCharacterOnlineCardBroadcast(ctx *ChatContext, data *characterOnlineCardBroadcastPayload) (any, error) {
	channelID := strings.TrimSpace(data.ChannelID)
	if err := ensureOnlineCardChannelAccess(ctx, channelID); err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(ctx.User.ID)
	identityID := strings.TrimSpace(data.IdentityID)
	action := strings.TrimSpace(data.Action)
	if action == "" {
		action = "update"
	}
	if action != "update" && action != "clear" {
		return nil, errors.New("action 参数错误")
	}
	if action == "clear" || data.Card == nil || strings.TrimSpace(data.Card.Name) == "" || identityID == "" {
		removeOnlineCharacterCardCache(channelID, userID)
		broadcastOnlineCharacterCardUpdate(ctx, channelID, &protocol.OnlineCharacterCardItem{UserID: userID}, "clear")
		return map[string]any{"ok": true}, nil
	}
	item := buildOnlineCharacterCardItem(ctx, channelID, identityID, data.Card)
	upsertOnlineCharacterCardCache(channelID, userID, item)
	broadcastOnlineCharacterCardUpdate(ctx, channelID, item, "update")
	return map[string]any{"ok": true}, nil
}

func apiCharacterOnlineCardSnapshot(ctx *ChatContext, data *characterOnlineCardSnapshotPayload) (any, error) {
	channelID := strings.TrimSpace(data.ChannelID)
	if err := ensureOnlineCardChannelAccess(ctx, channelID); err != nil {
		return nil, err
	}
	requesterID := strings.TrimSpace(ctx.User.ID)
	items := snapshotOnlineCharacterCards(channelID, requesterID, func(userID string) bool {
		return userHasChannelConnection(userID, channelID, ctx.UserId2ConnInfo, nil)
	})
	if ctx.Conn != nil {
		event := &protocol.Event{
			Type:    protocol.EventCharacterOnlineCardSnapshot,
			Channel: &protocol.Channel{ID: channelID},
			OnlineCharacterCardSnapshot: &protocol.OnlineCharacterCardSnapshotPayload{
				Items: items,
			},
		}
		_ = ctx.Conn.WriteJSON(struct {
			protocol.Event
			Op protocol.Opcode `json:"op"`
		}{
			Event: *event,
			Op:    protocol.OpEvent,
		})
	}
	return map[string]any{"ok": true}, nil
}

func ensureOnlineCardChannelAccess(ctx *ChatContext, channelID string) error {
	if ctx == nil || ctx.User == nil {
		return errors.New("未登录")
	}
	if channelID == "" {
		return errors.New("缺少频道ID")
	}
	if ctx.ConnInfo != nil && ctx.ConnInfo.ChannelId == channelID {
		return nil
	}
	if ctx.ChannelUsersMap != nil {
		if userSet, ok := ctx.ChannelUsersMap.Load(channelID); ok && userSet != nil && userSet.Exists(ctx.User.ID) {
			return nil
		}
	}
	if userHasChannelConnection(ctx.User.ID, channelID, ctx.UserId2ConnInfo, nil) {
		return nil
	}
	return errors.New("无权操作")
}

func buildOnlineCharacterCardItem(ctx *ChatContext, channelID, identityID string, card *protocol.OnlineCharacterCardData) *protocol.OnlineCharacterCardItem {
	item := &protocol.OnlineCharacterCardItem{
		UserID:     strings.TrimSpace(ctx.User.ID),
		Username:   strings.TrimSpace(ctx.User.Username),
		UserNick:   strings.TrimSpace(ctx.User.Nickname),
		UserColor:  strings.TrimSpace(ctx.User.NickColor),
		IdentityID: identityID,
		Card: &protocol.OnlineCharacterCardData{
			Name:         strings.TrimSpace(card.Name),
			SheetType:    strings.TrimSpace(card.SheetType),
			Attrs:        card.Attrs,
			TemplateText: card.TemplateText,
		},
		UpdatedAt: time.Now().Unix(),
	}
	if identity, err := model.ChannelIdentityGetByID(identityID); err == nil && identity != nil && identity.ChannelID == channelID && identity.UserID == item.UserID {
		item.IdentityName = strings.TrimSpace(identity.DisplayName)
		item.IdentityColor = strings.TrimSpace(identity.Color)
		item.IdentityAvatar = strings.TrimSpace(identity.AvatarAttachmentID)
	}
	return item
}

func broadcastOnlineCharacterCardUpdate(ctx *ChatContext, channelID string, item *protocol.OnlineCharacterCardItem, action string) {
	ctx.BroadcastEventInChannel(channelID, &protocol.Event{
		Type:    protocol.EventCharacterOnlineCardUpdated,
		Channel: &protocol.Channel{ID: channelID},
		OnlineCharacterCard: &protocol.OnlineCharacterCardEventPayload{
			Item:   item,
			Action: action,
		},
	})
}

func upsertOnlineCharacterCardCache(channelID, userID string, item *protocol.OnlineCharacterCardItem) {
	if channelID == "" || userID == "" || item == nil {
		return
	}
	onlineCharacterCardState.Lock()
	defer onlineCharacterCardState.Unlock()
	channelMap, ok := onlineCharacterCardState.items[channelID]
	if !ok || channelMap == nil {
		channelMap = map[string]*protocol.OnlineCharacterCardItem{}
		onlineCharacterCardState.items[channelID] = channelMap
	}
	channelMap[userID] = item
}

func removeOnlineCharacterCardCache(channelID, userID string) {
	if channelID == "" || userID == "" {
		return
	}
	onlineCharacterCardState.Lock()
	defer onlineCharacterCardState.Unlock()
	channelMap := onlineCharacterCardState.items[channelID]
	delete(channelMap, userID)
	if len(channelMap) == 0 {
		delete(onlineCharacterCardState.items, channelID)
	}
}

func snapshotOnlineCharacterCards(channelID, requesterID string, isOnline func(userID string) bool) []*protocol.OnlineCharacterCardItem {
	if channelID == "" {
		return nil
	}
	now := time.Now().Unix()
	onlineCharacterCardState.Lock()
	defer onlineCharacterCardState.Unlock()
	channelMap := onlineCharacterCardState.items[channelID]
	if len(channelMap) == 0 {
		return nil
	}
	items := make([]*protocol.OnlineCharacterCardItem, 0, len(channelMap))
	for userID, item := range channelMap {
		if userID == "" || userID == requesterID || item == nil {
			continue
		}
		if item.UpdatedAt > 0 && now-item.UpdatedAt > int64(onlineCharacterCardTTL/time.Second) {
			delete(channelMap, userID)
			continue
		}
		if isOnline != nil && !isOnline(userID) {
			continue
		}
		items = append(items, item)
	}
	if len(channelMap) == 0 {
		delete(onlineCharacterCardState.items, channelID)
	}
	return items
}

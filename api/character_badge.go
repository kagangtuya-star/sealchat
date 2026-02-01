package api

import (
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	"sealchat/model"
	"sealchat/protocol"
)

type characterBadgeBroadcastPayload struct {
	ChannelID  string         `json:"channel_id"`
	IdentityID string         `json:"identity_id"`
	Template   string         `json:"template"`
	Attrs      map[string]any `json:"attrs"`
	Action     string         `json:"action"` // update/clear
}

type characterBadgeSnapshotPayload struct {
	ChannelID string `json:"channel_id"`
}

type characterBadgeCache struct {
	sync.RWMutex
	items map[string]map[string]*protocol.CharacterCardBadgeEventPayload
}

var characterBadgeState = &characterBadgeCache{
	items: map[string]map[string]*protocol.CharacterCardBadgeEventPayload{},
}

func apiCharacterBadgeBroadcast(ctx *ChatContext, data *characterBadgeBroadcastPayload) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	channelID := strings.TrimSpace(data.ChannelID)
	identityID := strings.TrimSpace(data.IdentityID)
	if channelID == "" || identityID == "" {
		return nil, errors.New("缺少频道或身份ID")
	}
	if ctx.IsReadOnly() {
		return nil, errors.New("无权操作")
	}
	if err := ensureChannelMembership(ctx.User.ID, channelID); err != nil {
		return nil, err
	}
	identity, err := model.ChannelIdentityGetByID(identityID)
	if err != nil {
		return nil, err
	}
	if identity == nil || identity.ID == "" || identity.ChannelID != channelID || identity.UserID != ctx.User.ID {
		return nil, errors.New("无权操作")
	}
	action := strings.TrimSpace(data.Action)
	if action == "" {
		action = "update"
	}
	if action != "update" && action != "clear" {
		return nil, errors.New("action 参数错误")
	}
	template := strings.TrimSpace(data.Template)
	if action == "clear" || template == "" {
		removeBadgeCache(channelID, identityID)
		broadcastBadgeEvent(ctx, channelID, &protocol.CharacterCardBadgeEventPayload{
			IdentityID: identityID,
			Action:     "clear",
		})
		return map[string]any{"ok": true}, nil
	}
	if utf8.RuneCountInString(template) > 512 {
		return nil, errors.New("徽章模板长度需在512个字符以内")
	}
	attrs := data.Attrs
	if attrs == nil {
		attrs = map[string]any{}
	}
	payload := &protocol.CharacterCardBadgeEventPayload{
		IdentityID: identityID,
		Template:   template,
		Attrs:      attrs,
		Action:     "update",
	}
	upsertBadgeCache(channelID, payload)
	broadcastBadgeEvent(ctx, channelID, payload)
	return map[string]any{"ok": true}, nil
}

func apiCharacterBadgeSnapshot(ctx *ChatContext, data *characterBadgeSnapshotPayload) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	channelID := strings.TrimSpace(data.ChannelID)
	if channelID == "" {
		return nil, errors.New("缺少频道ID")
	}
	if ctx.IsReadOnly() {
		if ctx.ConnInfo == nil || ctx.ConnInfo.ChannelId != channelID {
			return nil, errors.New("无权操作")
		}
	} else if err := ensureChannelMembership(ctx.User.ID, channelID); err != nil {
		return nil, err
	}
	items := snapshotBadgeCache(channelID)
	if ctx.Conn != nil {
		event := &protocol.Event{
			Type:    protocol.EventCharacterCardBadgeSnapshot,
			Channel: &protocol.Channel{ID: channelID},
			CharacterCardBadgeSnapshot: &protocol.CharacterCardBadgeSnapshotPayload{
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

func broadcastBadgeEvent(ctx *ChatContext, channelID string, payload *protocol.CharacterCardBadgeEventPayload) {
	if ctx == nil || payload == nil || channelID == "" {
		return
	}
	ctx.BroadcastEventInChannel(channelID, &protocol.Event{
		Type:              protocol.EventCharacterCardBadgeUpdated,
		Channel:           &protocol.Channel{ID: channelID},
		CharacterCardBadge: payload,
	})
}

func upsertBadgeCache(channelID string, payload *protocol.CharacterCardBadgeEventPayload) {
	if payload == nil || channelID == "" || payload.IdentityID == "" {
		return
	}
	characterBadgeState.Lock()
	defer characterBadgeState.Unlock()
	channelMap, ok := characterBadgeState.items[channelID]
	if !ok || channelMap == nil {
		channelMap = map[string]*protocol.CharacterCardBadgeEventPayload{}
		characterBadgeState.items[channelID] = channelMap
	}
	channelMap[payload.IdentityID] = payload
}

func removeBadgeCache(channelID, identityID string) {
	if channelID == "" || identityID == "" {
		return
	}
	characterBadgeState.Lock()
	defer characterBadgeState.Unlock()
	channelMap, ok := characterBadgeState.items[channelID]
	if !ok || channelMap == nil {
		return
	}
	delete(channelMap, identityID)
	if len(channelMap) == 0 {
		delete(characterBadgeState.items, channelID)
	}
}

func snapshotBadgeCache(channelID string) []*protocol.CharacterCardBadgeEventPayload {
	if channelID == "" {
		return nil
	}
	characterBadgeState.RLock()
	channelMap := characterBadgeState.items[channelID]
	characterBadgeState.RUnlock()
	if len(channelMap) == 0 {
		return nil
	}
	items := make([]*protocol.CharacterCardBadgeEventPayload, 0, len(channelMap))
	for _, item := range channelMap {
		if item == nil || item.IdentityID == "" || item.Action == "clear" {
			continue
		}
		attrs := map[string]any{}
		for key, val := range item.Attrs {
			attrs[key] = val
		}
		items = append(items, &protocol.CharacterCardBadgeEventPayload{
			IdentityID: item.IdentityID,
			Template:   item.Template,
			Attrs:      attrs,
			Action:     "update",
		})
	}
	return items
}

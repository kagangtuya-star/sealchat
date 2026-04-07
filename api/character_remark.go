package api

import (
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"sealchat/model"
	"sealchat/protocol"
)

const characterRemarkMaxRunes = 80

type characterRemarkBroadcastPayload struct {
	ChannelID  string `json:"channel_id"`
	IdentityID string `json:"identity_id"`
	Content    string `json:"content"`
	Action     string `json:"action"` // update/clear
}

type characterRemarkSnapshotPayload struct {
	ChannelID string `json:"channel_id"`
}

type characterRemarkCache struct {
	sync.RWMutex
	items     map[string]map[string]*protocol.CharacterRemarkEventPayload
	revisions map[string]map[string]int64
}

func newCharacterRemarkCache() *characterRemarkCache {
	return &characterRemarkCache{
		items:     map[string]map[string]*protocol.CharacterRemarkEventPayload{},
		revisions: map[string]map[string]int64{},
	}
}

func (c *characterRemarkCache) upsert(channelID string, payload *protocol.CharacterRemarkEventPayload) {
	if c == nil || channelID == "" || payload == nil || payload.IdentityID == "" {
		return
	}
	c.Lock()
	defer c.Unlock()
	channelMap, ok := c.items[channelID]
	if !ok || channelMap == nil {
		channelMap = map[string]*protocol.CharacterRemarkEventPayload{}
		c.items[channelID] = channelMap
	}
	revisionMap, ok := c.revisions[channelID]
	if !ok || revisionMap == nil {
		revisionMap = map[string]int64{}
		c.revisions[channelID] = revisionMap
	}
	lastRevision := revisionMap[payload.IdentityID]
	if payload.Revision > 0 {
		if payload.Revision < lastRevision {
			return
		}
		revisionMap[payload.IdentityID] = payload.Revision
	} else {
		payload.Revision = lastRevision
	}
	channelMap[payload.IdentityID] = &protocol.CharacterRemarkEventPayload{
		IdentityID: payload.IdentityID,
		UserID:     payload.UserID,
		Content:    payload.Content,
		Revision:   payload.Revision,
		Action:     "update",
	}
}

func (c *characterRemarkCache) remove(channelID, identityID string) {
	c.removeWithRevision(channelID, identityID, 0)
}

func (c *characterRemarkCache) removeWithRevision(channelID, identityID string, revision int64) {
	if c == nil || channelID == "" || identityID == "" {
		return
	}
	c.Lock()
	defer c.Unlock()
	revisionMap, ok := c.revisions[channelID]
	if !ok || revisionMap == nil {
		revisionMap = map[string]int64{}
		c.revisions[channelID] = revisionMap
	}
	lastRevision := revisionMap[identityID]
	if revision > 0 {
		if revision < lastRevision {
			return
		}
		revisionMap[identityID] = revision
	}
	channelMap, ok := c.items[channelID]
	if !ok || channelMap == nil {
		return
	}
	delete(channelMap, identityID)
	if len(channelMap) == 0 {
		delete(c.items, channelID)
	}
}

func (c *characterRemarkCache) nextRevision(channelID, identityID string) int64 {
	if c == nil || channelID == "" || identityID == "" {
		return 0
	}
	c.Lock()
	defer c.Unlock()
	revisionMap, ok := c.revisions[channelID]
	if !ok || revisionMap == nil {
		revisionMap = map[string]int64{}
		c.revisions[channelID] = revisionMap
	}
	now := time.Now().UnixMilli()
	lastRevision := revisionMap[identityID]
	if lastRevision >= now {
		now = lastRevision + 1
	}
	revisionMap[identityID] = now
	return now
}

func (c *characterRemarkCache) snapshot(channelID string) []*protocol.CharacterRemarkEventPayload {
	if c == nil || channelID == "" {
		return nil
	}
	c.RLock()
	channelMap := c.items[channelID]
	c.RUnlock()
	if len(channelMap) == 0 {
		return nil
	}
	items := make([]*protocol.CharacterRemarkEventPayload, 0, len(channelMap))
	for _, item := range channelMap {
		if item == nil || item.IdentityID == "" || item.Action == "clear" || strings.TrimSpace(item.Content) == "" {
			continue
		}
		items = append(items, &protocol.CharacterRemarkEventPayload{
			IdentityID: item.IdentityID,
			UserID:     item.UserID,
			Content:    item.Content,
			Revision:   item.Revision,
			Action:     "update",
		})
	}
	return items
}

var characterRemarkState = newCharacterRemarkCache()

func normalizeCharacterRemarkContent(raw string) (string, bool, error) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return "", true, nil
	}
	if utf8.RuneCountInString(content) > characterRemarkMaxRunes {
		return "", false, errors.New("角色备注长度需在80个字符以内")
	}
	return content, false, nil
}

func apiCharacterRemarkBroadcast(ctx *ChatContext, data *characterRemarkBroadcastPayload) (any, error) {
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
	content, shouldClear, err := normalizeCharacterRemarkContent(data.Content)
	if err != nil {
		return nil, err
	}
	revision := characterRemarkState.nextRevision(channelID, identityID)
	if action == "clear" || shouldClear {
		characterRemarkState.removeWithRevision(channelID, identityID, revision)
		broadcastCharacterRemarkEvent(ctx, channelID, &protocol.CharacterRemarkEventPayload{
			IdentityID: identityID,
			UserID:     ctx.User.ID,
			Revision:   revision,
			Action:     "clear",
		})
		return map[string]any{"ok": true}, nil
	}
	payload := &protocol.CharacterRemarkEventPayload{
		IdentityID: identityID,
		UserID:     ctx.User.ID,
		Content:    content,
		Revision:   revision,
		Action:     "update",
	}
	characterRemarkState.upsert(channelID, payload)
	broadcastCharacterRemarkEvent(ctx, channelID, payload)
	return map[string]any{"ok": true}, nil
}

func apiCharacterRemarkSnapshot(ctx *ChatContext, data *characterRemarkSnapshotPayload) (any, error) {
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
	items := characterRemarkState.snapshot(channelID)
	if ctx.Conn != nil {
		event := &protocol.Event{
			Type:    protocol.EventCharacterRemarkSnapshot,
			Channel: &protocol.Channel{ID: channelID},
			CharacterRemarkSnapshot: &protocol.CharacterRemarkSnapshotPayload{
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

func broadcastCharacterRemarkEvent(ctx *ChatContext, channelID string, payload *protocol.CharacterRemarkEventPayload) {
	if ctx == nil || payload == nil || channelID == "" {
		return
	}
	ctx.BroadcastEventInChannel(channelID, &protocol.Event{
		Type:            protocol.EventCharacterRemarkUpdated,
		Channel:         &protocol.Channel{ID: channelID},
		CharacterRemark: payload,
	})
}

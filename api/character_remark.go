package api

import (
	"errors"
	"strings"
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
	if action == "clear" || shouldClear {
		content = ""
	}
	item, err := model.CharacterRemarkSave(channelID, identityID, ctx.User.ID, content)
	if err != nil {
		return nil, err
	}
	if content == "" {
		broadcastCharacterRemarkEvent(ctx, channelID, &protocol.CharacterRemarkEventPayload{
			IdentityID: identityID,
			UserID:     ctx.User.ID,
			Revision:   item.Revision,
			Action:     "clear",
		})
		return map[string]any{"ok": true}, nil
	}
	payload := &protocol.CharacterRemarkEventPayload{
		IdentityID: identityID,
		UserID:     ctx.User.ID,
		Content:    content,
		Revision:   item.Revision,
		Action:     "update",
	}
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
	rows, err := model.CharacterRemarkListByChannel(channelID)
	if err != nil {
		return nil, err
	}
	items := make([]*protocol.CharacterRemarkEventPayload, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.IdentityID == "" || strings.TrimSpace(row.Content) == "" {
			continue
		}
		items = append(items, &protocol.CharacterRemarkEventPayload{
			IdentityID: row.IdentityID,
			UserID:     row.UserID,
			Content:    row.Content,
			Revision:   row.Revision,
			Action:     "update",
		})
	}
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

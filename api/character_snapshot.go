package api

import (
	"errors"
	"strings"
	"time"

	"sealchat/protocol"
	"sealchat/service"
)

type characterSnapshotListRequest struct {
	ChannelID string `json:"channelId"`
}

type characterSnapshotUpsertRequest struct {
	ChannelID       string                         `json:"channelId"`
	IdentityID      string                         `json:"identityId"`
	SourceType      string                         `json:"sourceType"`
	SourceCardID    string                         `json:"sourceCardId"`
	SourceUpdatedAt int64                          `json:"sourceUpdatedAt"`
	Data            protocol.CharacterSnapshotData `json:"data"`
}

type characterSnapshotClearRequest struct {
	ChannelID  string `json:"channelId"`
	IdentityID string `json:"identityId"`
}

type characterSnapshotSettingsRequest struct {
	ChannelID string `json:"channelId"`
}

type characterSnapshotSettingsUpdateRequest struct {
	ChannelID                  string `json:"channelId"`
	BadgeTemplate              string `json:"badgeTemplate"`
	TheaterOverlayTemplateJSON string `json:"theaterOverlayTemplateJson"`
}

type characterSnapshotPreferenceUpdateRequest struct {
	ChannelID                  string `json:"channelId"`
	BadgeTemplateMode          string `json:"badgeTemplateMode"`
	BadgeTemplate              string `json:"badgeTemplate"`
	TheaterOverlayTemplateMode string `json:"theaterOverlayTemplateMode"`
	TheaterOverlayTemplateJSON string `json:"theaterOverlayTemplateJson"`
}

func apiCharacterSnapshotList(ctx *ChatContext, data *characterSnapshotListRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	channelID := strings.TrimSpace(data.ChannelID)
	items, err := service.CharacterSnapshotList(channelID, ctx.User.ID)
	if err != nil {
		return nil, err
	}
	payload := &protocol.CharacterSnapshotListPayload{ChannelID: channelID, Items: items}
	writeCharacterSnapshotEvent(ctx, &protocol.Event{
		Type: protocol.EventCharacterSnapshotList, Channel: &protocol.Channel{ID: channelID}, CharacterSnapshotList: payload,
	})
	return payload, nil
}

func apiCharacterSnapshotUpsert(ctx *ChatContext, data *characterSnapshotUpsertRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	if ctx.IsReadOnly() {
		return nil, errors.New("无权操作")
	}
	result, err := service.CharacterSnapshotUpsert(data.ChannelID, data.IdentityID, ctx.User.ID, data.SourceType, data.SourceCardID, data.SourceUpdatedAt, data.Data)
	if err != nil {
		return nil, err
	}
	if result.Changed && result.Item != nil {
		ctx.BroadcastEventInChannel(result.Item.ChannelID, &protocol.Event{
			Type:              protocol.EventCharacterSnapshotUpdated,
			Channel:           &protocol.Channel{ID: result.Item.ChannelID},
			CharacterSnapshot: &protocol.CharacterSnapshotEventPayload{Item: result.Item, Action: "update"},
		})
	}
	return result, nil
}

func apiCharacterSnapshotClear(ctx *ChatContext, data *characterSnapshotClearRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	if ctx.IsReadOnly() {
		return nil, errors.New("无权操作")
	}
	result, err := service.CharacterSnapshotClear(data.ChannelID, data.IdentityID, ctx.User.ID)
	if err != nil {
		return nil, err
	}
	if result.Changed && result.Item != nil {
		ctx.BroadcastEventInChannel(result.Item.ChannelID, &protocol.Event{
			Type:              protocol.EventCharacterSnapshotUpdated,
			Channel:           &protocol.Channel{ID: result.Item.ChannelID},
			CharacterSnapshot: &protocol.CharacterSnapshotEventPayload{Item: result.Item, Action: "clear"},
		})
	}
	return result, nil
}

func apiCharacterSnapshotSettingsGet(ctx *ChatContext, data *characterSnapshotSettingsRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	return service.CharacterSnapshotSettingsGet(data.ChannelID, ctx.User.ID)
}

func apiCharacterSnapshotSettingsUpdate(ctx *ChatContext, data *characterSnapshotSettingsUpdateRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	if ctx.IsReadOnly() {
		return nil, errors.New("无权操作")
	}
	payload, err := service.CharacterSnapshotSettingsUpdate(data.ChannelID, ctx.User.ID, service.CharacterSnapshotSettingsUpdateInput{BadgeTemplate: data.BadgeTemplate, TheaterOverlayTemplateJSON: data.TheaterOverlayTemplateJSON})
	if err != nil {
		return nil, err
	}
	ctx.BroadcastEventInChannel(payload.ChannelID, &protocol.Event{
		Type: protocol.EventCharacterSnapshotSettingsUpdated, Channel: &protocol.Channel{ID: payload.ChannelID}, CharacterSnapshotSettings: payload,
	})
	return payload, nil
}

func apiCharacterSnapshotPreferenceGet(ctx *ChatContext, data *characterSnapshotSettingsRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	return service.CharacterSnapshotPreferenceGet(data.ChannelID, ctx.User.ID)
}

func apiCharacterSnapshotPreferenceUpdate(ctx *ChatContext, data *characterSnapshotPreferenceUpdateRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	if ctx.IsReadOnly() {
		return nil, errors.New("无权操作")
	}
	payload, err := service.CharacterSnapshotPreferenceUpdate(data.ChannelID, ctx.User.ID, service.CharacterSnapshotPreferenceUpdateInput{
		BadgeTemplateMode: data.BadgeTemplateMode, BadgeTemplate: data.BadgeTemplate,
		TheaterOverlayTemplateMode: data.TheaterOverlayTemplateMode, TheaterOverlayTemplateJSON: data.TheaterOverlayTemplateJSON,
	})
	if err != nil {
		return nil, err
	}
	ctx.BroadcastEventInChannel(payload.ChannelID, &protocol.Event{
		Type: protocol.EventCharacterSnapshotPreferenceUpdated, Channel: &protocol.Channel{ID: payload.ChannelID}, CharacterSnapshotPreference: payload,
	})
	return payload, nil
}

func writeCharacterSnapshotEvent(ctx *ChatContext, event *protocol.Event) {
	if ctx == nil || ctx.Conn == nil || event == nil {
		return
	}
	event.Timestamp = time.Now().Unix()
	_ = ctx.Conn.WriteJSON(struct {
		protocol.Event
		Op protocol.Opcode `json:"op"`
	}{Event: *event, Op: protocol.OpEvent})
}

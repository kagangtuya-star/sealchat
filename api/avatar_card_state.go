package api

import (
	"errors"
	"strings"

	"sealchat/protocol"
	"sealchat/service"
)

type avatarCardSettingsRequest struct {
	ChannelID string `json:"channelId"`
}

type avatarCardSettingsUpdateRequest struct {
	ChannelID      string  `json:"channelId"`
	SourceMode     *string `json:"sourceMode"`
	TemplateSource string  `json:"templateSource"`
	TemplateJSON   *string `json:"templateJson"`
}

type worldCharacterStatePatchRequest struct {
	ChannelID  string   `json:"channelId"`
	IdentityID string   `json:"identityId"`
	Path       string   `json:"path"`
	Op         string   `json:"op"`
	Value      *float64 `json:"value"`
}

func apiAvatarCardSettingsGet(ctx *ChatContext, data *avatarCardSettingsRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	if data == nil {
		return nil, errors.New("缺少频道ID")
	}
	if ctx.IsReadOnly() {
		if _, err := checkReadOnlyChannelAccess(ctx, data.ChannelID); err != nil {
			return nil, err
		}
		return service.AvatarCardSettingsGetByChannel(data.ChannelID)
	}
	return service.AvatarCardSettingsGet(data.ChannelID, ctx.User.ID)
}

func apiAvatarCardSettingsUpdate(ctx *ChatContext, data *avatarCardSettingsUpdateRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	if ctx.IsReadOnly() {
		return nil, errors.New("无权操作")
	}
	if data == nil {
		return nil, errors.New("缺少头像卡片设置")
	}
	payload, err := service.AvatarCardSettingsUpdate(data.ChannelID, ctx.User.ID, service.AvatarCardSettingsUpdateInput{
		SourceMode: data.SourceMode, TemplateSource: data.TemplateSource, TemplateJSON: data.TemplateJSON,
	})
	if err != nil {
		return nil, err
	}
	ctx.BroadcastEventInChannel(payload.ChannelID, &protocol.Event{Type: protocol.EventAvatarCardSettingsUpdated,
		Channel: &protocol.Channel{ID: payload.ChannelID}, AvatarCardSettings: payload})
	return payload, nil
}

func apiWorldCharacterStateList(ctx *ChatContext, data *avatarCardSettingsRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	if data == nil {
		return nil, errors.New("缺少频道ID")
	}
	if ctx.IsReadOnly() {
		if _, err := checkReadOnlyChannelAccess(ctx, data.ChannelID); err != nil {
			return nil, err
		}
		return service.WorldCharacterStateListByChannel(data.ChannelID)
	}
	return service.WorldCharacterStateList(data.ChannelID, ctx.User.ID)
}

func apiWorldCharacterStatePatch(ctx *ChatContext, data *worldCharacterStatePatchRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("未登录")
	}
	if ctx.IsReadOnly() {
		return nil, errors.New("无权操作")
	}
	if data == nil || data.Value == nil {
		return nil, errors.New("缺少世界角色状态修改值")
	}
	payload, err := service.WorldCharacterStatePatch(data.ChannelID, data.IdentityID, ctx.User.ID,
		data.Path, data.Op, *data.Value)
	if err != nil {
		return nil, err
	}
	ctx.BroadcastEventInWorld(payload.WorldID, &protocol.Event{Type: protocol.EventWorldCharacterStateUpdated,
		Channel: &protocol.Channel{ID: strings.TrimSpace(data.ChannelID)}, WorldCharacterState: payload})
	return payload, nil
}

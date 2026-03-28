package service

import (
	"errors"
	"strings"

	"sealchat/protocol"
)

const (
	avatarDecorationOffsetMin       = -128
	avatarDecorationOffsetMax       = 128
	avatarDecorationRotationMin     = 0
	avatarDecorationRotationMax     = 360
	avatarDecorationOpacityMin      = 0
	avatarDecorationOpacityMax      = 1
)

func NormalizeAvatarDecoration(userID string, decoration *protocol.AvatarDecoration) (*protocol.AvatarDecoration, error) {
	return NormalizeAvatarDecorationWithAccess(userID, userID, "", decoration)
}

func NormalizeAvatarDecorationWithAccess(ownerUserID string, operatorUserID string, channelID string, decoration *protocol.AvatarDecoration) (*protocol.AvatarDecoration, error) {
	if decoration == nil {
		return nil, nil
	}
	list, err := NormalizeAvatarDecorationsWithAccess(ownerUserID, operatorUserID, channelID, protocol.AvatarDecorationList{*decoration})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	first := list[0]
	return &first, nil
}

func NormalizeAvatarDecorations(userID string, decorations protocol.AvatarDecorationList) (protocol.AvatarDecorationList, error) {
	return NormalizeAvatarDecorationsWithAccess(userID, userID, "", decorations)
}

func NormalizeAvatarDecorationsWithAccess(ownerUserID string, operatorUserID string, channelID string, decorations protocol.AvatarDecorationList) (protocol.AvatarDecorationList, error) {
	if len(decorations) == 0 {
		return nil, nil
	}
	result := make(protocol.AvatarDecorationList, 0, len(decorations))
	for _, rawDecoration := range decorations {
		resourceID := strings.TrimSpace(rawDecoration.ResourceAttachmentID)
		if !rawDecoration.Enabled || resourceID == "" {
			continue
		}

		resourceAtt, err := ResolveAttachmentAccessible(ownerUserID, operatorUserID, channelID, resourceID)
		if err != nil {
			return nil, errors.New("头像装饰资源无效或不属于当前用户")
		}
		if resourceAtt == nil {
			return nil, errors.New("头像装饰资源不存在")
		}
		resourceMime := strings.ToLower(strings.TrimSpace(resourceAtt.MimeType))
		if resourceMime != "image/png" && resourceMime != "image/webp" && resourceMime != "video/webm" {
			return nil, errors.New("头像装饰资源仅支持 PNG、WEBP 或 WEBM")
		}

		fallbackID := strings.TrimSpace(rawDecoration.FallbackAttachmentID)
		if fallbackID != "" {
			fallbackAtt, err := ResolveAttachmentAccessible(ownerUserID, operatorUserID, channelID, fallbackID)
			if err != nil {
				return nil, errors.New("头像装饰兜底资源无效或不属于当前用户")
			}
			if fallbackAtt == nil {
				return nil, errors.New("头像装饰兜底资源不存在")
			}
			fallbackMime := strings.ToLower(strings.TrimSpace(fallbackAtt.MimeType))
			if fallbackMime != "image/png" && fallbackMime != "image/webp" {
				return nil, errors.New("头像装饰兜底资源仅支持 PNG 或 WEBP")
			}
		}

		settings := rawDecoration.Settings
		if settings.Scale == 0 {
			settings.Scale = 1
		}
		if settings.Opacity == 0 {
			settings.Opacity = 1
		}
		if settings.PlaybackRate == 0 {
			settings.PlaybackRate = 1
		}
		if settings.ZIndex == 0 {
			settings.ZIndex = 1
		}
		if settings.BlendMode == "" {
			settings.BlendMode = "normal"
		}

		if settings.OffsetX < avatarDecorationOffsetMin || settings.OffsetX > avatarDecorationOffsetMax ||
			settings.OffsetY < avatarDecorationOffsetMin || settings.OffsetY > avatarDecorationOffsetMax {
			return nil, errors.New("头像装饰位移超出允许范围")
		}
		if settings.Rotation < avatarDecorationRotationMin || settings.Rotation > avatarDecorationRotationMax {
			return nil, errors.New("头像装饰旋转超出允许范围")
		}
		if settings.Opacity < avatarDecorationOpacityMin || settings.Opacity > avatarDecorationOpacityMax {
			return nil, errors.New("头像装饰透明度超出允许范围")
		}
		if settings.ZIndex != 1 && settings.ZIndex != -1 {
			return nil, errors.New("头像装饰层级仅支持前景或背景")
		}

		result = append(result, protocol.AvatarDecoration{
			ID:                   strings.TrimSpace(rawDecoration.ID),
			Enabled:              true,
			DecorationID:         strings.TrimSpace(rawDecoration.DecorationID),
			ResourceAttachmentID: resourceID,
			FallbackAttachmentID: fallbackID,
			Settings:             settings,
		})
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

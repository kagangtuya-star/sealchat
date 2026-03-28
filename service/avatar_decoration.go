package service

import (
	"errors"
	"strings"

	"sealchat/protocol"
)

const (
	avatarDecorationScaleMin    = 0.5
	avatarDecorationScaleMax    = 1.5
	avatarDecorationOffsetMin   = -128
	avatarDecorationOffsetMax   = 128
	avatarDecorationRotationMin = 0
	avatarDecorationRotationMax = 360
	avatarDecorationOpacityMin  = 0
	avatarDecorationOpacityMax  = 1
)

func NormalizeAvatarDecoration(userID string, decoration *protocol.AvatarDecoration) (*protocol.AvatarDecoration, error) {
	if decoration == nil {
		return nil, nil
	}

	resourceID := strings.TrimSpace(decoration.ResourceAttachmentID)
	if !decoration.Enabled || resourceID == "" {
		return nil, nil
	}

	resourceAtt, err := ResolveAttachmentOwnership(userID, resourceID)
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

	fallbackID := strings.TrimSpace(decoration.FallbackAttachmentID)
	if fallbackID != "" {
		fallbackAtt, err := ResolveAttachmentOwnership(userID, fallbackID)
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

	settings := decoration.Settings
	if settings.Scale == 0 {
		settings.Scale = 1
	}
	if settings.Opacity == 0 {
		settings.Opacity = 1
	}
	if settings.ZIndex == 0 {
		settings.ZIndex = 1
	}
	if settings.BlendMode == "" {
		settings.BlendMode = "normal"
	}

	if settings.Scale < avatarDecorationScaleMin || settings.Scale > avatarDecorationScaleMax {
		return nil, errors.New("头像装饰缩放超出允许范围")
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

	return &protocol.AvatarDecoration{
		Enabled:              true,
		DecorationID:         strings.TrimSpace(decoration.DecorationID),
		ResourceAttachmentID: resourceID,
		FallbackAttachmentID: fallbackID,
		Settings:             settings,
	}, nil
}

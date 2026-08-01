package service

import (
	"errors"
	"strings"

	"sealchat/model"
	"sealchat/protocol"
)

func ValidateStickyNoteAppearanceForWorld(worldID, actorID string, value *protocol.StickyNoteAppearance) error {
	if err := protocol.ValidateStickyNoteAppearance(value); err != nil {
		return err
	}
	if value == nil || value.Background == nil {
		return nil
	}
	attachment, err := ResolveAttachment(value.Background.AttachmentID)
	if err != nil || attachment == nil {
		return errors.New("便签背景附件不存在")
	}
	switch strings.ToLower(strings.TrimSpace(attachment.MimeType)) {
	case "image/png", "image/jpeg", "image/webp", "image/avif":
	default:
		return errors.New("便签背景图片格式不受支持")
	}
	if attachment.UserID == actorID {
		return nil
	}
	channelID := strings.TrimSpace(attachment.ChannelID)
	if channelID == "" {
		return ErrWorldPermission
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil || channel == nil || channel.WorldID != worldID {
		return ErrWorldPermission
	}
	return nil
}

package api

import (
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
)

func canUserReadAllWhispersInChannel(userID, channelID string) bool {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" {
		return false
	}
	if len(channelID) >= 30 {
		return false
	}

	// 默认严格保密。只有在世界明确关闭该开关时，才允许旧的“可读全部悄悄话”旁路逻辑。
	strictPrivacy := true
	db := model.GetDB()
	if db == nil {
		return false
	}
	var channel model.ChannelModel
	if err := db.Select("id, world_id").Where("id = ?", channelID).Limit(1).Find(&channel).Error; err != nil || channel.ID == "" {
		return false
	}
	if worldID := strings.TrimSpace(channel.WorldID); worldID != "" {
		var world model.WorldModel
		if err := db.
			Select("id, strict_whisper_privacy").
			Where("id = ? AND status = ?", worldID, "active").
			Limit(1).
			Find(&world).Error; err == nil && world.ID != "" {
			strictPrivacy = world.StrictWhisperPrivacy
		}
	}
	if strictPrivacy {
		return false
	}

	if pm.CanWithSystemRole(userID, pm.PermModAdmin) {
		return true
	}
	return pm.CanWithChannelRole(userID, channelID, pm.PermFuncChannelMessageReadWhisperAll)
}

func applyWhisperVisibilityFilter(q *gorm.DB, userID, channelID string) *gorm.DB {
	if q == nil {
		return q
	}
	if canUserReadAllWhispersInChannel(userID, channelID) {
		return q
	}
	return q.Where(`(is_whisper = ? OR user_id = ? OR whisper_to = ? OR EXISTS (
		SELECT 1 FROM message_whisper_recipients r WHERE r.message_id = messages.id AND r.user_id = ?
	))`, false, userID, userID, userID)
}

func eventContainsWhisper(data *protocol.Event) bool {
	if data == nil {
		return false
	}
	if data.Message != nil && data.Message.IsWhisper {
		return true
	}
	if data.MessageContext != nil && data.MessageContext.IsWhisper {
		return true
	}
	return false
}

func canUserAccessWhisperMessage(userID, channelID string, msg *model.MessageModel) bool {
	if msg == nil || !msg.IsWhisper {
		return true
	}
	if canUserReadAllWhispersInChannel(userID, channelID) {
		return true
	}
	if msg.UserID == userID || msg.WhisperTo == userID {
		return true
	}
	return model.HasWhisperRecipient(msg.ID, userID)
}

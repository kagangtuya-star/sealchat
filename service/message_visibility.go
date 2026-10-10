package service

import (
	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/pm"
	"strings"
)

func CanUserReadAllWhispersInChannel(userID, channelID string) bool {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" || len(channelID) >= 30 {
		return false
	}
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
		if err := db.Select("id, strict_whisper_privacy").Where("id = ? AND status = ?", worldID, "active").Limit(1).Find(&world).Error; err == nil && world.ID != "" {
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
func ApplyWhisperVisibilityFilter(q *gorm.DB, userID, channelID string) *gorm.DB {
	return ApplyWhisperVisibilityFilterWithReadAll(q, userID, CanUserReadAllWhispersInChannel(userID, channelID))
}
func ApplyWhisperVisibilityFilterWithReadAll(q *gorm.DB, userID string, canReadAll bool) *gorm.DB {
	if q == nil || canReadAll {
		return q
	}
	return q.Where(`(is_whisper = ? OR user_id = ? OR whisper_to = ? OR EXISTS (
		SELECT 1 FROM message_whisper_recipients r WHERE r.message_id = messages.id AND r.user_id = ?
	))`, false, userID, userID, userID)
}

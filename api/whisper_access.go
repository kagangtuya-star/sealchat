package api

import (
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

func canUserReadAllWhispersInChannel(userID, channelID string) bool {
	return service.CanUserReadAllWhispersInChannel(userID, channelID)
}

func applyWhisperVisibilityFilter(q *gorm.DB, userID, channelID string) *gorm.DB {
	if q == nil {
		return q
	}
	return applyWhisperVisibilityFilterWithReadAll(q, userID, canUserReadAllWhispersInChannel(userID, channelID))
}

func applyWhisperVisibilityFilterWithReadAll(q *gorm.DB, userID string, canReadAll bool) *gorm.DB {
	return service.ApplyWhisperVisibilityFilterWithReadAll(q, userID, canReadAll)
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

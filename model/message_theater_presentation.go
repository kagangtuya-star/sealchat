package model

import (
	"encoding/json"
	"strings"

	"sealchat/protocol"

	"gorm.io/gorm"
)

func resolveMessageTheaterPresentation(tx *gorm.DB, message *MessageModel) *protocol.TheaterPresentation {
	if message == nil || strings.TrimSpace(message.SenderIdentityID) == "" {
		return nil
	}
	if tx == nil {
		tx = db
	}
	var identity ChannelIdentityModel
	if err := tx.Where("id = ? AND channel_id = ? AND user_id = ?", message.SenderIdentityID, message.ChannelID, message.UserID).
		Limit(1).Find(&identity).Error; err != nil || identity.ID == "" {
		return nil
	}
	var patch *protocol.TheaterPresentationPatch
	patchConfigured := false
	if strings.TrimSpace(message.SenderIdentityVariantID) != "" {
		var variant ChannelIdentityVariantModel
		if err := tx.Where("id = ? AND identity_id = ? AND channel_id = ? AND user_id = ?", message.SenderIdentityVariantID, identity.ID, message.ChannelID, message.UserID).
			Limit(1).Find(&variant).Error; err == nil && variant.ID != "" {
			var document struct {
				TheaterPresentation json.RawMessage `json:"theaterPresentation"`
			}
			if json.Unmarshal([]byte(variant.AppearanceJSON), &document) == nil && len(document.TheaterPresentation) > 0 {
				patchConfigured = true
				if strings.TrimSpace(string(document.TheaterPresentation)) != "null" {
					var value protocol.TheaterPresentationPatch
					if json.Unmarshal(document.TheaterPresentation, &value) == nil {
						patch = &value
					} else {
						patchConfigured = false
					}
				}
			}
		}
	}
	if identity.TheaterPresentation == nil && !patchConfigured {
		return nil
	}
	base := protocol.DefaultTheaterPresentation()
	if identity.TheaterPresentation != nil {
		base = *identity.TheaterPresentation
	}
	resolved := protocol.ResolveTheaterPresentation(base, patch)
	return &resolved
}

package service

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"sealchat/model"
)

func ttsAccessibleVoice(db *gorm.DB, userID, voiceID string) (*model.TTSVoice, error) {
	var voice model.TTSVoice
	err := db.Where("id = ? AND deleted_at IS NULL AND lifecycle = ? AND provider_status = ? AND (owner_user_id = ? OR is_public = ?)", voiceID, "saved", "OK", userID, true).First(&voice).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTTSDenied
	}
	return &voice, err
}

// Save promotes a preview; replacement keeps the old immutable cloud version
// until no running task references it. Public metadata never exposes samples.
func TTSSaveVoice(db *gorm.DB, userID, voiceID, replaceID string, defaultSlots int, now time.Time) error {
	return withTTSUserPolicy(db, userID, func(tx *gorm.DB, p *model.TTSUserPolicy) error {
		var voice model.TTSVoice
		if err := tx.Where("id = ? AND owner_user_id = ? AND deleted_at IS NULL", voiceID, userID).First(&voice).Error; err != nil {
			return err
		}
		if voice.Lifecycle == "saved" && replaceID == "" {
			return nil
		}
		if voice.Lifecycle != "preview" || voice.ProviderStatus != "OK" || voice.PreviewExpiresAt == nil || !now.Before(*voice.PreviewExpiresAt) {
			return ErrTTSConflict
		}
		if replaceID != "" {
			r := tx.Model(&model.TTSVoice{}).Where("id = ? AND owner_user_id = ? AND lifecycle = ? AND deleted_at IS NULL", replaceID, userID, "saved").Updates(map[string]any{"lifecycle": "delete_pending", "is_public": false, "revision": gorm.Expr("revision + 1")})
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return ErrTTSConflict
			}
		}
		var count int64
		if err := tx.Model(&model.TTSVoice{}).Where("owner_user_id = ? AND lifecycle = ? AND deleted_at IS NULL", userID, "saved").Count(&count).Error; err != nil {
			return err
		}
		slots := defaultSlots
		if p.Slots != nil {
			slots = *p.Slots
		}
		// Replacing an existing saved voice does not require an extra slot,
		// even when the administrator has since lowered the user's limit.
		if replaceID == "" && count >= int64(slots) {
			return TTSValidationError("个人音色槽位已满")
		}
		r := tx.Model(&model.TTSVoice{}).Where("id = ? AND lifecycle = ? AND revision = ?", voice.ID, "preview", voice.Revision).Updates(map[string]any{"lifecycle": "saved", "preview_expires_at": nil, "revision": gorm.Expr("revision + 1")})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrTTSConflict
		}
		return nil
	})
}

// Claim expiry under the same user lock as Save. Never automatically GC saved
// voices, and retain cloud IDs until upstream deletion is confirmed.
func TTSExpirePreviews(db *gorm.DB, userID string, now time.Time) error {
	return withTTSUserPolicy(db, userID, func(tx *gorm.DB, _ *model.TTSUserPolicy) error {
		return tx.Model(&model.TTSVoice{}).Where("owner_user_id = ? AND deleted_at IS NULL AND lifecycle IN ? AND preview_expires_at <= ?", userID, []string{"creating", "preview"}, now).Updates(map[string]any{"lifecycle": "delete_pending", "is_public": false, "revision": gorm.Expr("revision + 1")}).Error
	})
}

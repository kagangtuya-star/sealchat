package service

import (
	"gorm.io/gorm"
	"math"
	"sealchat/model"
	"strings"
)

func persistMCPIdentityAvatar(input *ChannelIdentityInput, ownerID, operatorID, identityID string, write func(*gorm.DB) error) error {
	if !input.ConfirmMCPAvatar {
		return write(model.GetDB())
	}
	att, err := ResolveAttachmentAccessible(ownerID, operatorID, input.ChannelID, input.AvatarAttachmentID)
	if err != nil {
		return err
	}
	if att != nil && att.DeletedAt != nil {
		return gorm.ErrRecordNotFound
	}
	if att == nil || att.Extra != "mcp-upload" || !att.IsTemp {
		return write(model.GetDB())
	}
	if !strings.HasPrefix(att.MimeType, "image/") || !isUnboundMCPAttachment(att, att.UserID) {
		return ErrWorldClueInvalid
	}
	ch, err := model.ChannelGet(input.ChannelID)
	if err != nil {
		return err
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		r := tx.Model(&model.AttachmentModel{}).Where("id = ? AND user_id = ? AND is_temp = ? AND extra = ? AND deleted_at IS NULL AND root_id = '' AND root_id_type = '' AND channel_id = ''", att.ID, att.UserID, true, "mcp-upload").Updates(map[string]any{"is_temp": false, "channel_id": ch.ID, "root_id": ch.WorldID, "root_id_type": "world", "parent_id": identityID, "parent_id_type": "channel_identity"})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrWorldClueConflict
		}
		return write(tx)
	})
}

func ValidateMCPAudioReferences(worldID string, in AudioPlaybackUpdateInput) error {
	checkAsset := func(id string) error {
		if id == "" {
			return nil
		}
		v, err := AudioGetAsset(id)
		if err != nil {
			return err
		}
		if v.Scope == model.AudioScopeWorld && (v.WorldID == nil || *v.WorldID != worldID) {
			return ErrWorldPermission
		}
		return nil
	}
	if in.SceneID != nil && *in.SceneID != "" {
		scene, err := AudioGetScene(*in.SceneID)
		if err != nil {
			return err
		}
		if scene.Scope == model.AudioScopeWorld && (scene.WorldID == nil || *scene.WorldID != worldID) || scene.ChannelScope != nil && *scene.ChannelScope != "" && *scene.ChannelScope != in.ChannelID {
			return ErrWorldPermission
		}
	}
	for _, t := range in.Tracks {
		if math.IsNaN(t.Volume) || math.IsInf(t.Volume, 0) || t.Volume < 0 || t.Volume > 1 || t.Position < 0 || len(t.PlaylistAssetIDs) > 200 {
			return ErrWorldClueInvalid
		}
		if t.AssetID != nil {
			if err := checkAsset(*t.AssetID); err != nil {
				return err
			}
		}
		for _, id := range t.PlaylistAssetIDs {
			if err := checkAsset(id); err != nil {
				return err
			}
		}
		if t.PlaylistFolderID != nil && *t.PlaylistFolderID != "" {
			folder, err := AudioGetFolder(*t.PlaylistFolderID)
			if err != nil {
				return err
			}
			if folder.Scope == model.AudioScopeWorld && (folder.WorldID == nil || *folder.WorldID != worldID) {
				return ErrWorldPermission
			}
		}
		if t.PlaylistMode != nil && *t.PlaylistMode != "single" && *t.PlaylistMode != "sequential" && *t.PlaylistMode != "shuffle" {
			return ErrWorldClueInvalid
		}
		if math.IsNaN(t.Position) || math.IsInf(t.Position, 0) || t.PlaybackRate < 0 || t.PlaybackRate > 4 || t.FadeIn < 0 || t.FadeOut < 0 || t.PlaylistIndex < 0 {
			return ErrWorldClueInvalid
		}
	}
	return nil
}

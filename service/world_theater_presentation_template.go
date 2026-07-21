package service

import (
	"encoding/json"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
)

func WorldTheaterPresentationTemplateSet(worldID, actorID string, template protocol.WorldTheaterPresentationTemplate) (*model.WorldModel, error) {
	worldID = strings.TrimSpace(worldID)
	actorID = strings.TrimSpace(actorID)
	if worldID == "" || actorID == "" {
		return nil, ErrWorldPermission
	}
	if !pm.CanWithSystemRole(actorID, pm.PermModAdmin) && !IsWorldAdmin(worldID, actorID) {
		return nil, ErrWorldPermission
	}
	if err := protocol.ValidateWorldTheaterPresentationTemplate(template); err != nil {
		return nil, err
	}
	if template.Dialogue != nil && template.Dialogue.Frame != nil {
		var asset model.TheaterAppearanceAssetModel
		if err := model.GetDB().Where("id = ? AND deleted_at IS NULL", template.Dialogue.Frame.Media.AssetID).Limit(1).Find(&asset).Error; err != nil {
			return nil, err
		}
		if asset.ID == "" || asset.Status != "ready" || asset.Purpose != "dialogue-frame" || !theaterMediaRefMatchesAsset(template.Dialogue.Frame.Media, asset) {
			return nil, newTheaterError(TheaterAppearanceAssetErrorInvalid, "世界默认对话框资源无效", 400, nil)
		}
		var channel model.ChannelModel
		if err := model.GetDB().Where("id = ? AND world_id = ?", asset.ChannelID, worldID).Limit(1).Find(&channel).Error; err != nil {
			return nil, err
		}
		if channel.ID == "" {
			return nil, newTheaterError(TheaterAppearanceAssetErrorScopeMismatch, "对话框资源不属于当前世界", 400, nil)
		}
	}

	world, err := GetWorldByID(worldID)
	if err != nil {
		return nil, err
	}
	if world == nil || world.Status != "active" {
		return nil, ErrWorldNotFound
	}
	oldTemplate := world.GetTheaterPresentationTemplate()

	raw, err := json.Marshal(template)
	if err != nil {
		return nil, err
	}

	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.WorldModel{}).
			Where("id = ? AND status = ?", worldID, "active").
			Update("theater_presentation_template_json", string(raw))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrWorldNotFound
		}
		return cascadeWorldTheaterPresentationDefaults(tx, worldID, oldTemplate, template)
	})
	if err != nil {
		return nil, err
	}
	return GetWorldByID(worldID)
}

// cascadeWorldTheaterPresentationDefaults rewrites identity presentations that still
// look like defaults so they pick up the new world template sections.
func cascadeWorldTheaterPresentationDefaults(tx *gorm.DB, worldID string, oldTemplate, newTemplate protocol.WorldTheaterPresentationTemplate) error {
	var channelIDs []string
	if err := tx.Model(&model.ChannelModel{}).
		Where("world_id = ?", worldID).
		Pluck("id", &channelIDs).Error; err != nil {
		return err
	}
	if len(channelIDs) == 0 {
		return nil
	}

	var identities []model.ChannelIdentityModel
	if err := tx.Where("channel_id IN ?", channelIDs).Find(&identities).Error; err != nil {
		return err
	}

	nilReplacement := protocol.MaterializeWorldTheaterPresentationDefaults(newTemplate)

	for i := range identities {
		identity := &identities[i]
		var encoded []byte
		var err error

		if identity.TheaterPresentation == nil {
			if nilReplacement == nil {
				continue
			}
			// No stored presentation: treat as pure default and materialize new world defaults.
			encoded, err = json.Marshal(nilReplacement)
			if err != nil {
				return err
			}
		} else {
			updated, changed := protocol.ReplaceMatchingWorldTheaterDefaults(*identity.TheaterPresentation, oldTemplate, newTemplate)
			if !changed {
				continue
			}
			encoded, err = json.Marshal(updated)
			if err != nil {
				return err
			}
		}

		if err := tx.Model(&model.ChannelIdentityModel{}).
			Where("id = ?", identity.ID).
			Update("theater_presentation", string(encoded)).Error; err != nil {
			return err
		}
	}
	return nil
}

func WorldTheaterPresentationDefaultsForChannel(channelID string) *protocol.TheaterPresentation {
	channel, err := model.ChannelGet(strings.TrimSpace(channelID))
	if err != nil || channel == nil || strings.TrimSpace(channel.WorldID) == "" {
		return nil
	}
	world, err := GetWorldByID(channel.WorldID)
	if err != nil || world == nil {
		return nil
	}
	template := world.GetTheaterPresentationTemplate()
	return protocol.MaterializeWorldTheaterPresentationDefaults(template)
}

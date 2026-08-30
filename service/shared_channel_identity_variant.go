package service

import (
	"encoding/json"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

func sharedVariantAppearanceParts(item *model.ChannelIdentityVariantModel) (map[string]any, *protocol.TheaterPresentationPatch, bool) {
	appearance := map[string]any{}
	if item != nil {
		for key, value := range item.Appearance() {
			if key != "theaterPresentation" {
				appearance[key] = value
			}
		}
	}
	patch, set := variantTheaterPresentationPatch(item)
	return appearance, patch, set
}

func sharedVariantAppearanceJSON(appearance map[string]any, patch *protocol.TheaterPresentationPatch, patchSet bool) (string, error) {
	value := make(map[string]any, len(appearance)+1)
	for key, item := range appearance {
		if key != "theaterPresentation" {
			value[key] = item
		}
	}
	if patchSet {
		value["theaterPresentation"] = patch
	}
	return serializeChannelIdentityVariantAppearanceJSON(value)
}

func sharedIdentityCopiesTx(tx *gorm.DB, sharedIdentityID string) ([]*model.ChannelIdentityModel, error) {
	var copies []*model.ChannelIdentityModel
	err := tx.Where("shared_identity_id = ?", sharedIdentityID).
		Order("channel_id ASC, created_at ASC").Find(&copies).Error
	return copies, err
}

func createSharedVariantCopiesTx(tx *gorm.DB, sourceIdentity *model.ChannelIdentityModel, source *model.ChannelIdentityVariantModel, identityCopies []*model.ChannelIdentityModel) error {
	globalAppearance, patch, patchSet := sharedVariantAppearanceParts(source)
	sourceWorldID, err := sharedChannelIdentityWorldIDTx(tx, sourceIdentity.ChannelID)
	if err != nil {
		return err
	}
	for _, identityCopy := range identityCopies {
		if identityCopy.ID == sourceIdentity.ID {
			continue
		}
		var existing int64
		if err := tx.Model(&model.ChannelIdentityVariantModel{}).
			Where("identity_id = ? AND shared_variant_id = ?", identityCopy.ID, source.SharedVariantID).
			Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			continue
		}
		appearanceJSON, err := sharedVariantAppearanceJSON(globalAppearance, nil, false)
		if err != nil {
			return err
		}
		variantID := utils.NewID()
		if identityCopy.ChannelID != sourceIdentity.ChannelID {
			targetWorldID, worldErr := sharedChannelIdentityWorldIDTx(tx, identityCopy.ChannelID)
			if worldErr != nil {
				return worldErr
			}
			if patchSet && targetWorldID == sourceWorldID {
				mapped, mapErr := mapSharedTheaterPresentationPatchTx(tx, sourceIdentity.ChannelID, sourceIdentity.ID, source.ID, identityCopy, variantID, patch)
				if mapErr != nil {
					return mapErr
				}
				appearanceJSON, err = sharedVariantAppearanceJSON(globalAppearance, mapped, true)
				if err != nil {
					return err
				}
			}
		}
		copy := &model.ChannelIdentityVariantModel{
			StringPKBaseModel:  model.StringPKBaseModel{ID: variantID},
			IdentityID:         identityCopy.ID,
			ChannelID:          identityCopy.ChannelID,
			UserID:             identityCopy.UserID,
			SharedVariantID:    source.SharedVariantID,
			SharedRevision:     source.SharedRevision,
			SelectorEmoji:      source.SelectorEmoji,
			Keyword:            source.Keyword,
			MatchMode:          source.MatchMode,
			MatchConfig:        source.MatchConfig,
			Note:               source.Note,
			AvatarAttachmentID: source.AvatarAttachmentID,
			DisplayName:        source.DisplayName,
			Color:              source.Color,
			AppearanceJSON:     appearanceJSON,
			SortOrder:          source.SortOrder,
			Enabled:            source.Enabled,
		}
		if err := tx.Create(copy).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizeSharedVariantSourceAppearanceTx(tx *gorm.DB, sourceIdentity *model.ChannelIdentityModel, source *model.ChannelIdentityVariantModel) error {
	globalAppearance, patch, patchSet := sharedVariantAppearanceParts(source)
	normalizedPatch, err := mapSharedTheaterPresentationPatchTx(
		tx, sourceIdentity.ChannelID, sourceIdentity.ID, source.ID, sourceIdentity, source.ID, patch,
	)
	if err != nil {
		return err
	}
	appearanceJSON, err := sharedVariantAppearanceJSON(globalAppearance, normalizedPatch, patchSet)
	if err != nil {
		return err
	}
	source.AppearanceJSON = appearanceJSON
	return tx.Model(source).Update("appearance_json", appearanceJSON).Error
}

func promoteSharedChannelIdentityVariantsTx(tx *gorm.DB, sourceIdentity *model.ChannelIdentityModel, template *model.SharedChannelIdentityModel) error {
	var variants []*model.ChannelIdentityVariantModel
	if err := tx.Where("identity_id = ? AND channel_id = ? AND user_id = ?", sourceIdentity.ID, sourceIdentity.ChannelID, sourceIdentity.UserID).
		Order("sort_order ASC, created_at ASC").Find(&variants).Error; err != nil {
		return err
	}
	if len(variants) == 0 {
		return nil
	}
	identityCopies, err := sharedIdentityCopiesTx(tx, template.ID)
	if err != nil {
		return err
	}
	for _, variant := range variants {
		variant.SharedVariantID = utils.NewID()
		variant.SharedRevision = 1
		if err := tx.Model(variant).Updates(map[string]any{
			"shared_variant_id": variant.SharedVariantID,
			"shared_revision":   variant.SharedRevision,
		}).Error; err != nil {
			return err
		}
		if err := normalizeSharedVariantSourceAppearanceTx(tx, sourceIdentity, variant); err != nil {
			return err
		}
		if err := createSharedVariantCopiesTx(tx, sourceIdentity, variant, identityCopies); err != nil {
			return err
		}
	}
	return nil
}

func createSharedChannelIdentityVariant(sourceIdentity *model.ChannelIdentityModel, source *model.ChannelIdentityVariantModel) error {
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		var lockedIdentity model.ChannelIdentityModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND shared_identity_id = ?", sourceIdentity.ID, sourceIdentity.SharedIdentityID).Take(&lockedIdentity).Error; err != nil {
			return err
		}
		source.SharedVariantID = utils.NewID()
		source.SharedRevision = 1
		if err := tx.Create(source).Error; err != nil {
			return err
		}
		if err := normalizeSharedVariantSourceAppearanceTx(tx, &lockedIdentity, source); err != nil {
			return err
		}
		identityCopies, err := sharedIdentityCopiesTx(tx, lockedIdentity.SharedIdentityID)
		if err != nil {
			return err
		}
		return createSharedVariantCopiesTx(tx, &lockedIdentity, source, identityCopies)
	})
}

func syncSharedChannelIdentityVariant(sourceIdentity *model.ChannelIdentityModel, source *model.ChannelIdentityVariantModel, expectedRevision int64) (*model.ChannelIdentityVariantModel, error) {
	var updated model.ChannelIdentityVariantModel
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var copies []*model.ChannelIdentityVariantModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("shared_variant_id = ?", source.SharedVariantID).
			Order("channel_id ASC, created_at ASC").Find(&copies).Error; err != nil {
			return err
		}
		if len(copies) == 0 {
			return gorm.ErrRecordNotFound
		}
		// Highest committed shared revision is authority. A successful write
		// replaces it atomically and projects source-local media refs to every copy.
		var authority *model.ChannelIdentityVariantModel
		var sourceCopy *model.ChannelIdentityVariantModel
		for _, copy := range copies {
			if authority == nil || copy.SharedRevision > authority.SharedRevision {
				authority = copy
			}
			if copy.ID == source.ID {
				sourceCopy = copy
			}
		}
		if sourceCopy == nil || authority == nil {
			return gorm.ErrRecordNotFound
		}
		if expectedRevision > 0 && authority.SharedRevision != expectedRevision {
			return ErrSharedChannelIdentityVariantRevisionConflict
		}
		globalAppearance, patch, patchSet := sharedVariantAppearanceParts(source)
		sourceWorldID, err := sharedChannelIdentityWorldIDTx(tx, sourceIdentity.ChannelID)
		if err != nil {
			return err
		}
		normalizedPatch, err := mapSharedTheaterPresentationPatchTx(tx, sourceIdentity.ChannelID, sourceIdentity.ID, source.ID, sourceIdentity, source.ID, patch)
		if err != nil {
			return err
		}
		nextRevision := authority.SharedRevision + 1
		for _, copy := range copies {
			_, copyPatch, copyPatchSet := sharedVariantAppearanceParts(copy)
			appearancePatch := copyPatch
			appearancePatchSet := copyPatchSet
			targetIdentity := sourceIdentity
			if copy.IdentityID != sourceIdentity.ID {
				targetIdentity = new(model.ChannelIdentityModel)
				if err := tx.Where("id = ? AND shared_identity_id = ?", copy.IdentityID, sourceIdentity.SharedIdentityID).Take(targetIdentity).Error; err != nil {
					return err
				}
			}
			targetWorldID, err := sharedChannelIdentityWorldIDTx(tx, targetIdentity.ChannelID)
			if err != nil {
				return err
			}
			if targetWorldID == sourceWorldID {
				appearancePatch, err = mapSharedTheaterPresentationPatchTx(tx, sourceIdentity.ChannelID, sourceIdentity.ID, source.ID, targetIdentity, copy.ID, normalizedPatch)
				if err != nil {
					return err
				}
				appearancePatchSet = patchSet
			}
			appearanceJSON, err := sharedVariantAppearanceJSON(globalAppearance, appearancePatch, appearancePatchSet)
			if err != nil {
				return err
			}
			values := map[string]any{
				"selector_emoji": source.SelectorEmoji, "keyword": source.Keyword, "match_mode": source.MatchMode, "match_config": source.MatchConfig, "note": source.Note,
				"avatar_attachment_id": source.AvatarAttachmentID, "display_name": source.DisplayName, "color": source.Color,
				"appearance_json": appearanceJSON, "enabled": source.Enabled, "shared_revision": nextRevision,
			}
			if err := tx.Model(copy).Updates(values).Error; err != nil {
				return err
			}
			if copy.ID == source.ID {
				if err := tx.Where("id = ?", copy.ID).Take(&updated).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func deleteSharedChannelIdentityVariant(sharedVariantID string) error {
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		return tx.Where("shared_variant_id = ?", strings.TrimSpace(sharedVariantID)).Delete(&model.ChannelIdentityVariantModel{}).Error
	})
}

func reorderSharedChannelIdentityVariants(ordered []*model.ChannelIdentityVariantModel) error {
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		for index, source := range ordered {
			if source.SharedVariantID == "" {
				continue
			}
			if err := tx.Model(&model.ChannelIdentityVariantModel{}).
				Where("shared_variant_id = ?", source.SharedVariantID).
				Update("sort_order", index+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func mapSharedTheaterPresentationPatchTx(tx *gorm.DB, sourceChannelID, sourceIdentityID, sourceVariantID string, target *model.ChannelIdentityModel, targetVariantID string, patch *protocol.TheaterPresentationPatch) (*protocol.TheaterPresentationPatch, error) {
	if patch == nil {
		return nil, nil
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}
	var result protocol.TheaterPresentationPatch
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	var worldTemplateRef *protocol.TheaterMediaRef
	worldID, err := sharedChannelIdentityWorldIDTx(tx, target.ChannelID)
	if err != nil {
		return nil, err
	}
	if worldID != "" {
		var world model.WorldModel
		if err := tx.Where("id = ?", worldID).Limit(1).Find(&world).Error; err != nil {
			return nil, err
		}
		if frame := world.GetTheaterPresentationTemplate().Dialogue; frame != nil && frame.Frame != nil {
			ref := frame.Frame.Media
			worldTemplateRef = &ref
		}
	}
	remapLayer := func(layer *protocol.TheaterVisualLayer) error {
		if layer == nil || strings.TrimSpace(layer.Media.AssetID) == "" {
			return nil
		}
		if worldTemplateRef != nil && theaterMediaRefsEqual(layer.Media, *worldTemplateRef) {
			return nil
		}
		mapped, err := mapSharedTheaterAppearanceAssetTx(tx, sourceChannelID, sourceIdentityID, sourceVariantID, target, targetVariantID, layer.Media)
		if err != nil {
			return err
		}
		layer.Media = mapped
		return nil
	}
	if result.Portrait.Set && result.Portrait.Value != nil {
		if err := remapLayer(result.Portrait.Value); err != nil {
			return nil, err
		}
	}
	if result.PortraitDecorations.Set && result.PortraitDecorations.Value != nil {
		for index := range *result.PortraitDecorations.Value {
			if err := remapLayer(&(*result.PortraitDecorations.Value)[index]); err != nil {
				return nil, err
			}
		}
	}
	if result.Dialogue.Set && result.Dialogue.Value != nil {
		if err := remapLayer(result.Dialogue.Value.Frame); err != nil {
			return nil, err
		}
	}
	return &result, nil
}

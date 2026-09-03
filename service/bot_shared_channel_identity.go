package service

import (
	"context"
	"errors"
	"log"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
)

func ensureBotManagedChannelIdentityActor(actor *ChannelIdentityActorContext, channelID string) (*ChannelIdentityActorContext, error) {
	if actor == nil {
		return nil, ErrChannelPermissionDenied
	}
	resolved, err := ResolveChannelIdentityActor(channelID, actor.OperatorUserID, actor.TargetUserID)
	if err != nil {
		return nil, err
	}
	if !resolved.IsBotTarget || !resolved.IsDelegated {
		return nil, ErrChannelIdentityDelegationForbidden
	}
	return resolved, nil
}

func BotManagedChannelIdentityUpdate(actor *ChannelIdentityActorContext, identityID string, input *ChannelIdentityInput) (*ChannelIdentityUpdateResult, error) {
	if input == nil {
		return nil, errors.New("参数不能为空")
	}
	resolved, err := ensureBotManagedChannelIdentityActor(actor, input.ChannelID)
	if err != nil {
		return nil, err
	}
	if err := EnsureBotChannelIdentity(resolved.TargetUserID, input.ChannelID); err != nil {
		return nil, err
	}
	if _, err := ValidateChannelIdentityActorIdentity(resolved, input.ChannelID, identityID); err != nil {
		return nil, err
	}
	return channelIdentityUpdateDetailedWithAccess(resolved.TargetUserID, resolved.OperatorUserID, identityID, input, true)
}

func BotManagedChannelIdentityVariantCreate(actor *ChannelIdentityActorContext, input *ChannelIdentityVariantInput) (*model.ChannelIdentityVariantModel, error) {
	if input == nil {
		return nil, errors.New("参数不能为空")
	}
	resolved, err := ensureBotManagedChannelIdentityActor(actor, input.ChannelID)
	if err != nil {
		return nil, err
	}
	if err := EnsureBotChannelIdentity(resolved.TargetUserID, input.ChannelID); err != nil {
		return nil, err
	}
	if _, err := ValidateChannelIdentityActorIdentity(resolved, input.ChannelID, input.IdentityID); err != nil {
		return nil, err
	}
	return ChannelIdentityVariantCreateWithAccess(resolved.TargetUserID, resolved.OperatorUserID, input)
}

func BotManagedChannelIdentityVariantUpdate(actor *ChannelIdentityActorContext, variantID string, input *ChannelIdentityVariantInput) (*model.ChannelIdentityVariantModel, error) {
	if input == nil {
		return nil, errors.New("参数不能为空")
	}
	resolved, err := ensureBotManagedChannelIdentityActor(actor, input.ChannelID)
	if err != nil {
		return nil, err
	}
	if err := EnsureBotChannelIdentity(resolved.TargetUserID, input.ChannelID); err != nil {
		return nil, err
	}
	if _, err := ValidateChannelIdentityActorIdentity(resolved, input.ChannelID, input.IdentityID); err != nil {
		return nil, err
	}
	return ChannelIdentityVariantUpdateWithAccess(resolved.TargetUserID, resolved.OperatorUserID, variantID, input)
}

func BotManagedChannelIdentityVariantDelete(actor *ChannelIdentityActorContext, channelID, variantID string) error {
	resolved, err := ensureBotManagedChannelIdentityActor(actor, channelID)
	if err != nil {
		return err
	}
	if err := EnsureBotChannelIdentity(resolved.TargetUserID, channelID); err != nil {
		return err
	}
	variant, err := ChannelIdentityVariantGetForUser(resolved.TargetUserID, channelID, variantID)
	if err != nil {
		return err
	}
	if _, err := ValidateChannelIdentityActorIdentity(resolved, channelID, variant.IdentityID); err != nil {
		return err
	}
	return ChannelIdentityVariantDeleteWithAccess(resolved.TargetUserID, resolved.OperatorUserID, channelID, variantID)
}

func BotManagedChannelIdentityVariantReorder(actor *ChannelIdentityActorContext, channelID, identityID string, ids []string) error {
	resolved, err := ensureBotManagedChannelIdentityActor(actor, channelID)
	if err != nil {
		return err
	}
	if err := EnsureBotChannelIdentity(resolved.TargetUserID, channelID); err != nil {
		return err
	}
	if _, err := ValidateChannelIdentityActorIdentity(resolved, channelID, identityID); err != nil {
		return err
	}
	return ChannelIdentityVariantReorderWithAccess(resolved.TargetUserID, resolved.OperatorUserID, channelID, identityID, ids)
}

func ensureBotSharedChannelIdentity(user *model.UserModel, channelID string) error {
	if user == nil || !user.IsBot {
		return nil
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return err
	}
	if channel == nil {
		return gorm.ErrRecordNotFound
	}
	worldID := strings.TrimSpace(channel.WorldID)
	var affectedAssetIDs []string
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var lockedUser model.UserModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", user.ID).Take(&lockedUser).Error; err != nil {
			return err
		}

		identity, err := findBotDefaultChannelIdentityTx(tx, lockedUser.ID, channelID)
		if err != nil {
			return err
		}
		if worldID == "" {
			if identity == nil {
				_, err = createBotDefaultChannelIdentityTx(tx, &lockedUser, channelID)
			}
			return nil
		}

		var template model.SharedChannelIdentityModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND world_id = ?", user.ID, worldID).
			Order("created_at ASC, id ASC").Limit(1).Find(&template).Error; err != nil {
			return err
		}

		if template.ID != "" {
			if identity == nil {
				identity, err = createBotDefaultChannelIdentityTx(tx, &lockedUser, channelID)
				if err != nil {
					return err
				}
			}
			if identity.SharedIdentityID != "" && identity.SharedIdentityID != template.ID {
				return ErrChannelIdentityAlreadyShared
			}
			return materializeBotSharedChannelIdentityCopiesTxWithOrphans(tx, identity, &template, &affectedAssetIDs)
		}

		if identity == nil {
			identity, err = findBotDefaultChannelIdentityInWorldTx(tx, user.ID, worldID)
			if err != nil {
				return err
			}
		}
		if identity == nil {
			identity, err = createBotDefaultChannelIdentityTx(tx, &lockedUser, channelID)
			if err != nil {
				return err
			}
		}
		if identity.SharedIdentityID == "" {
			result := &SharedChannelIdentitySyncResult{}
			return sharedChannelIdentityCreateFromCopyTxWithOrphans(tx, user.ID, identity.ID, result, true, &affectedAssetIDs)
		}
		return ErrChannelIdentityAlreadyShared
	})
	if err != nil {
		return err
	}
	if err := reconcileTheaterAppearanceAssetOrphans(context.Background(), affectedAssetIDs); err != nil {
		log.Printf("BOT共享角色接入后资源清理失败[user=%s channel=%s]: %v", user.ID, channelID, err)
	}
	return nil
}

func findBotDefaultChannelIdentityTx(tx *gorm.DB, userID, channelID string) (*model.ChannelIdentityModel, error) {
	var identity model.ChannelIdentityModel
	if err := tx.Where("channel_id = ? AND user_id = ? AND is_default = ?", channelID, userID, true).
		Limit(1).Find(&identity).Error; err != nil {
		return nil, err
	}
	if identity.ID != "" {
		return &identity, nil
	}
	return nil, nil
}

func findBotDefaultChannelIdentityInWorldTx(tx *gorm.DB, userID, worldID string) (*model.ChannelIdentityModel, error) {
	var channelIDs []string
	if err := tx.Model(&model.ChannelModel{}).
		Where("world_id = ? AND (status IS NULL OR status <> ?)", worldID, model.ChannelStatusDeleted).
		Pluck("id", &channelIDs).Error; err != nil {
		return nil, err
	}
	if len(channelIDs) == 0 {
		return nil, nil
	}
	var identity model.ChannelIdentityModel
	if err := tx.Where("user_id = ? AND channel_id IN ? AND is_default = ? AND (is_hidden = ? OR is_hidden IS NULL) AND (shared_identity_id = '' OR shared_identity_id IS NULL)",
		userID, channelIDs, true, false).
		Order("created_at ASC, id ASC").Limit(1).Find(&identity).Error; err != nil {
		return nil, err
	}
	if identity.ID == "" {
		return nil, nil
	}
	return &identity, nil
}

func createBotDefaultChannelIdentityTx(tx *gorm.DB, user *model.UserModel, channelID string) (*model.ChannelIdentityModel, error) {
	var maxSort int
	if err := tx.Model(&model.ChannelIdentityModel{}).Where("channel_id = ? AND user_id = ?", channelID, user.ID).
		Select("coalesce(max(sort_order), 0)").Scan(&maxSort).Error; err != nil {
		return nil, err
	}
	displayName := strings.TrimSpace(user.Nickname)
	if displayName == "" {
		displayName = strings.TrimSpace(user.Username)
	}
	if displayName == "" {
		displayName = "Bot"
	}
	identity := model.ChannelIdentityModel{
		ChannelID: channelID, UserID: user.ID, DisplayName: displayName,
		Color: model.ChannelIdentityNormalizeColor(user.NickColor), AvatarAttachmentID: strings.TrimSpace(user.Avatar),
		SortOrder: maxSort + 1, IsDefault: true, BotAppearanceMode: "inherit",
	}
	if err := tx.Create(&identity).Error; err != nil {
		return nil, err
	}
	return &identity, nil
}

func materializeBotSharedChannelIdentityCopiesTx(tx *gorm.DB, source *model.ChannelIdentityModel, template *model.SharedChannelIdentityModel) error {
	return materializeBotSharedChannelIdentityCopiesTxWithOrphans(tx, source, template, nil)
}

func materializeBotSharedChannelIdentityCopiesTxWithOrphans(tx *gorm.DB, source *model.ChannelIdentityModel, template *model.SharedChannelIdentityModel, orphanAssetIDs *[]string) error {
	if err := createSharedChannelIdentityCopiesWithOptionsTx(tx, source, template, sharedChannelIdentityCopyOptions{
		reuseDefaultProjection: true,
		repairExisting:         true,
		botManaged:              true,
		orphanAssetIDs:         orphanAssetIDs,
	}); err != nil {
		return err
	}
	copies, err := sharedIdentityCopiesTx(tx, template.ID)
	if err != nil {
		return err
	}

	var sourceIdentity model.ChannelIdentityModel
	if err := tx.Where("id = ? AND shared_identity_id = ?", template.SourceIdentityID, template.ID).Take(&sourceIdentity).Error; err != nil {
		return err
	}
	var variants []*model.ChannelIdentityVariantModel
	if err := tx.Where("identity_id = ? AND shared_variant_id <> ''", sourceIdentity.ID).
		Order("sort_order ASC, created_at ASC").Find(&variants).Error; err != nil {
		return err
	}
	for _, variant := range variants {
		if err := createSharedVariantCopiesTx(tx, &sourceIdentity, variant, copies); err != nil {
			return err
		}
	}
	return nil
}

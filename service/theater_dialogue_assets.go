package service

import (
	"sealchat/model"
	"strings"
)

// System roots have no speaking projection or user owner. Only this asset path
// recognizes their IDs; ordinary identity ownership validation remains strict.
func ResolveTheaterAppearanceActor(channelID, operatorID, targetID, identityID, variantID, purpose string) (*ChannelIdentityActorContext, error) {
	return resolveTheaterAppearanceActor(channelID, operatorID, targetID, identityID, variantID, purpose, true)
}

func resolveTheaterAppearanceActor(channelID, operatorID, targetID, identityID, variantID, purpose string, requireIdentity bool) (*ChannelIdentityActorContext, error) {
	var root model.SharedChannelIdentityModel
	err := model.GetDB().Where("id = ? AND system_kind = ?", strings.TrimSpace(identityID), TheaterDialogueControllerSystemKind).Limit(1).Find(&root).Error
	if err != nil {
		return nil, err
	}
	if root.ID != "" {
		channel, err := model.ChannelGet(channelID)
		if err != nil || channel == nil || channel.WorldID != root.WorldID || !IsWorldAdmin(root.WorldID, operatorID) || variantID != "" || (purpose != "dialogue-frame" && purpose != "portrait-decoration") {
			return nil, ErrChannelPermissionDenied
		}
		return &ChannelIdentityActorContext{OperatorUserID: operatorID, TargetUserID: operatorID, WorldID: root.WorldID}, nil
	}
	actor, err := ResolveChannelIdentityActor(channelID, operatorID, targetID)
	if err != nil {
		return nil, err
	}
	if requireIdentity {
		if _, err = ValidateChannelIdentityActorIdentity(actor, channelID, identityID); err != nil {
			return nil, newTheaterError(TheaterAppearanceAssetErrorScopeMismatch, "identity 不属于目标用户或频道", 400, nil)
		}
	}
	return actor, nil
}

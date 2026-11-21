package service

import (
	"encoding/json"
	"strings"

	"sealchat/model"
)

type MessageAppearancePatch struct {
	Name               *string `json:"name,omitempty"`
	Color              *string `json:"color,omitempty"`
	AvatarAttachmentID *string `json:"avatar_attachment_id,omitempty"`
	VariantID          *string `json:"variant_id,omitempty"`
}

func ChannelMemberRoleRanks(channel *model.ChannelModel, userIDs []string) (map[string]int, error) {
	out := map[string]int{}
	if len(userIDs) == 0 {
		return out, nil
	}
	var roles []model.UserRoleMappingModel
	if err := model.GetDB().Where("user_id IN ? AND role_type = ? AND role_id LIKE ?", userIDs, "channel", "ch-"+channel.ID+"-%").Find(&roles).Error; err != nil {
		return nil, err
	}
	for _, row := range roles {
		for suffix, rank := range map[string]int{"-owner": 4, "-admin": 3, "-member": 2, "-spectator": 1} {
			if strings.HasSuffix(row.RoleID, suffix) && out[row.UserID] < rank {
				out[row.UserID] = rank
			}
		}
	}
	if channel.WorldID != "" {
		var members []model.WorldMemberModel
		if err := model.GetDB().Where("world_id = ? AND user_id IN ?", channel.WorldID, userIDs).Find(&members).Error; err != nil {
			return nil, err
		}
		for _, m := range members {
			if m.Role == model.WorldRoleOwner {
				out[m.UserID] = 4
			} else if m.Role == model.WorldRoleAdmin && out[m.UserID] < 3 {
				out[m.UserID] = 3
			}
		}
	}
	for _, id := range userIDs {
		if id != "" && id == channel.UserID {
			out[id] = 4
		}
	}
	return out, nil
}

func ChannelMemberRoleRank(channel *model.ChannelModel, channelID, userID string) int {
	if strings.TrimSpace(userID) == "" {
		return 0
	}
	if channel != nil && channel.UserID == userID {
		return 4
	}
	roles, err := model.UserRoleMappingListByUserID(userID, channelID, "channel")
	rank := 0
	if err == nil {
		for _, role := range roles {
			for suffix, value := range map[string]int{"-owner": 4, "-admin": 3, "-member": 2, "-spectator": 1} {
				if strings.HasSuffix(role, suffix) && rank < value {
					rank = value
				}
			}
		}
	}
	if channel != nil && channel.WorldID != "" {
		if IsWorldOwner(channel.WorldID, userID) {
			return 4
		}
		if IsWorldAdmin(channel.WorldID, userID) && rank < 3 {
			rank = 3
		}
	}
	return rank
}
func CanEditHistoricalMessage(channel *model.ChannelModel, m *model.MessageModel, userID string) bool {
	if channel == nil || m.IsDeleted || m.IsRevoked || !CanReadChannelByUserId(userID, m.ChannelID) {
		return false
	}
	var visible int64
	if ApplyWhisperVisibilityFilter(model.GetDB().Model(&model.MessageModel{}).Where("id = ? AND channel_id = ?", m.ID, m.ChannelID), userID, m.ChannelID).Count(&visible).Error != nil || visible != 1 {
		return false
	}
	if m.UserID == userID {
		return true
	}
	if channel.WorldID == "" {
		return false
	}
	w, err := GetWorldByID(channel.WorldID)
	if err != nil || !w.AllowAdminEditMessages {
		return false
	}
	rank := ChannelMemberRoleRank(channel, channel.ID, userID)
	return rank >= 3 && rank > ChannelMemberRoleRank(channel, channel.ID, m.UserID)
}
func MessageAppearanceSnapshot(m *model.MessageModel) string {
	b, _ := json.Marshal(map[string]any{"name": m.SenderIdentityName, "memberName": m.SenderMemberName, "color": m.SenderIdentityColor, "avatarAttachmentId": m.SenderIdentityAvatarID, "identityId": m.SenderIdentityID, "sharedIdentityId": m.SenderSharedIdentityID, "variantId": m.SenderIdentityVariantID})
	return string(b)
}
func PrepareMessageAppearance(m *model.MessageModel, actor string, p MessageAppearancePatch) (map[string]any, error) {
	if err := ValidateMessageAppearancePatch(p); err != nil {
		return nil, err
	}
	channel, err := model.ChannelGet(m.ChannelID)
	if err != nil {
		return nil, err
	}
	if !CanEditHistoricalMessage(channel, m, actor) {
		return nil, logError("PERMISSION_DENIED", "无权编辑此消息")
	}
	if p.Name == nil && p.Color == nil && p.AvatarAttachmentID == nil && p.VariantID == nil {
		return nil, logError("INVALID_PATCH", "外观修改不能为空")
	}
	if p.VariantID != nil && (p.Name != nil || p.Color != nil || p.AvatarAttachmentID != nil) {
		return nil, logError("INVALID_PATCH", "差分和外观不可同时修改")
	}
	updates := map[string]any{}
	if p.Name != nil {
		if strings.TrimSpace(*p.Name) == "" || len([]rune(*p.Name)) > 100 {
			return nil, logError("INVALID_PATCH", "名字须为 1～100 字")
		}
		updates["sender_identity_name"] = *p.Name
		updates["sender_member_name"] = *p.Name
	}
	if p.Color != nil {
		if *p.Color != "" && normalizeAgentColor(*p.Color) == "" {
			return nil, logError("INVALID_PATCH", "颜色无效")
		}
		updates["sender_identity_color"] = *p.Color
	}
	if p.AvatarAttachmentID != nil {
		id := *p.AvatarAttachmentID
		if id != "" {
			a, err := ResolveAttachmentAccessible(m.UserID, actor, m.ChannelID, id)
			if err != nil || a == nil || !strings.HasPrefix(a.MimeType, "image/") || a.IsTTSManaged() {
				return nil, logError("INVALID_PATCH", "头像资源不可用")
			}
			id = a.ID
		}
		updates["sender_identity_avatar_id"] = id
	}
	if p.VariantID != nil {
		identity, err := ChannelIdentityValidateMessageIdentity(m.UserID, m.ChannelID, m.SenderIdentityID)
		if err != nil {
			return nil, logError("INVALID_IDENTITY", "原作者不可使用此身份")
		}
		variant, err := ChannelIdentityVariantValidateMessageVariant(m.UserID, m.ChannelID, identity, *p.VariantID)
		if err != nil {
			return nil, logError("INVALID_IDENTITY", "差分不可用")
		}
		a := ResolveChannelIdentityAppearance(identity, variant)
		if a == nil {
			return nil, logError("INVALID_IDENTITY", "身份不可用")
		}
		updates["sender_identity_variant_id"] = a.VariantID
		updates["sender_identity_name"] = a.DisplayName
		updates["sender_member_name"] = a.DisplayName
		updates["sender_identity_color"] = a.Color
		updates["sender_identity_avatar_id"] = a.AvatarAttachmentID
		updates["sender_identity_decoration"] = a.AvatarDecorations
		updates["sender_theater_presentation"] = a.TheaterPresentation
		updates["sender_chat_skin"] = a.ChatSkin
	}
	return updates, nil
}

func ValidateMessageAppearancePatch(p MessageAppearancePatch) error {
	if p.Name == nil && p.Color == nil && p.AvatarAttachmentID == nil && p.VariantID == nil {
		return logError("INVALID_PATCH", "外观修改不能为空")
	}
	if p.VariantID != nil && (p.Name != nil || p.Color != nil || p.AvatarAttachmentID != nil) {
		return logError("INVALID_PATCH", "差分和外观不可同时修改")
	}
	if p.Name != nil && (strings.TrimSpace(*p.Name) == "" || len([]rune(*p.Name)) > 100) {
		return logError("INVALID_PATCH", "名字须为 1～100 字")
	}
	if p.Color != nil && *p.Color != "" && normalizeAgentColor(*p.Color) == "" {
		return logError("INVALID_PATCH", "颜色无效")
	}
	if p.AvatarAttachmentID != nil && len(*p.AvatarAttachmentID) > 100 {
		return logError("INVALID_PATCH", "头像标识无效")
	}
	return nil
}

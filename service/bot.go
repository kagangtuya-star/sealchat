package service

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/samber/lo"
	"gorm.io/gorm"

	"sealchat/model"
)

type BotAppearanceSyncResult struct {
	UpdatedIdentities []*model.ChannelIdentityModel
}

func privateBotIDsByChannel(channel *model.ChannelModel) []string {
	if channel == nil {
		return nil
	}
	if !channel.IsPrivate && !strings.EqualFold(strings.TrimSpace(channel.PermType), "private") {
		return nil
	}
	ids := channel.GetPrivateUserIDs()
	if len(ids) == 0 {
		return nil
	}
	botIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		user := model.UserGet(id)
		if user != nil && user.IsBot {
			botIDs = append(botIDs, id)
		}
	}
	if len(botIDs) == 0 {
		return nil
	}
	botIDs = lo.Uniq(botIDs)
	sort.Strings(botIDs)
	return botIDs
}

func privateBotIDByChannel(channel *model.ChannelModel) string {
	botIDs := privateBotIDsByChannel(channel)
	if len(botIDs) == 0 {
		return ""
	}
	return botIDs[0]
}

func IsBotFeatureEffectivelyEnabled(channel *model.ChannelModel) bool {
	if channel == nil {
		return false
	}
	if channel.BotFeatureEnabled {
		return true
	}
	return privateBotIDByChannel(channel) != ""
}

func IsBuiltInDiceEffectivelyEnabled(channel *model.ChannelModel) bool {
	if channel == nil {
		return false
	}
	if IsBotFeatureEffectivelyEnabled(channel) {
		return false
	}
	return channel.BuiltInDiceEnabled
}

func SelectedBotIdByChannelId(channelId string) (string, error) {
	channelId = strings.TrimSpace(channelId)
	if channelId == "" {
		return "", errors.New("缺少频道ID")
	}
	roleId := fmt.Sprintf("ch-%s-%s", channelId, "bot")
	ids, _ := model.UserRoleMappingUserIdListByRoleId(roleId)
	if len(ids) > 0 {
		filtered := make([]string, 0, len(ids))
		for _, id := range ids {
			user := model.UserGet(id)
			if user != nil && user.IsBot {
				filtered = append(filtered, id)
			}
		}
		ids = lo.Uniq(filtered)
		if len(ids) > 0 {
			sort.Strings(ids)
			selected := ids[0]
			if len(ids) > 1 {
				log.Printf("[bot] channel %s has multiple bot bindings: %v, selecting %s", channelId, ids, selected)
			}
			return selected, nil
		}
	}

	channel, err := model.ChannelGet(channelId)
	if err == nil {
		if selected := privateBotIDByChannel(channel); selected != "" {
			return selected, nil
		}
	}
	return "", errors.New("未选择频道机器人")
}

func BotListByChannelId(curUserId, channelId string) []string {
	var ids []string
	roleId := fmt.Sprintf("ch-%s-%s", channelId, "bot")
	ids1, _ := model.UserRoleMappingUserIdListByRoleId(roleId)
	ids = append(ids, ids1...)

	ch, _ := model.ChannelGet(channelId)
	if ch.ID != "" && ch.PermType != "private" && !ch.BotFeatureEnabled {
		return []string{}
	}
	if ch.PermType == "private" {
		// 私聊时自动将对端 bot 视为频道机器人
		for _, botID := range privateBotIDsByChannel(ch) {
			if botID == curUserId {
				continue
			}
			ids = append(ids, botID)
		}
	} else {
		// 获取子频道的授权
		if ch.RootId != "" {
			roleId := fmt.Sprintf("ch-%s-%s", ch.RootId, "bot")
			ids2, _ := model.UserRoleMappingUserIdListByRoleId(roleId)
			ids = append(ids, ids2...)
		}
	}

	return lo.Uniq(ids)
}

// SyncBotUserProfile keeps the bot user's public profile aligned with the token metadata.
func SyncBotUserProfile(token *model.BotTokenModel) error {
	if token == nil || token.ID == "" {
		return nil
	}
	user := model.UserGet(token.ID)
	if user == nil {
		return fmt.Errorf("bot user not found")
	}
	updates := map[string]any{}
	if name := strings.TrimSpace(token.Name); name != "" && user.Nickname != name {
		updates["nickname"] = name
	}
	if user.Avatar != strings.TrimSpace(token.Avatar) {
		updates["avatar"] = token.Avatar
	}
	if user.NickColor != model.ChannelIdentityNormalizeColor(token.NickColor) {
		updates["nick_color"] = token.NickColor
	}
	if len(updates) == 0 {
		return nil
	}
	return model.GetDB().Model(user).Updates(updates).Error
}

// SyncBotMembers updates all channel member records to reflect the latest bot nickname.
func SyncBotMembers(token *model.BotTokenModel) error {
	if token == nil || token.ID == "" {
		return nil
	}
	name := strings.TrimSpace(token.Name)
	if name == "" {
		return nil
	}
	return model.GetDB().Model(&model.MemberModel{}).
		Where("user_id = ?", token.ID).
		Update("nickname", name).Error
}

func SyncBotChannelAppearance(token *model.BotTokenModel) (*BotAppearanceSyncResult, error) {
	if token == nil || token.ID == "" {
		return &BotAppearanceSyncResult{}, nil
	}

	displayName := strings.TrimSpace(token.Name)
	if displayName == "" {
		user := model.UserGet(token.ID)
		if user != nil {
			displayName = strings.TrimSpace(user.Nickname)
			if displayName == "" {
				displayName = strings.TrimSpace(user.Username)
			}
		}
	}
	if displayName == "" {
		displayName = "Bot"
	}

	color := model.ChannelIdentityNormalizeColor(token.NickColor)
	avatar := strings.TrimSpace(token.Avatar)
	result := &BotAppearanceSyncResult{
		UpdatedIdentities: []*model.ChannelIdentityModel{},
	}

	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var managedIdentities []*model.ChannelIdentityModel
		if err := tx.Where("user_id = ? AND (is_hidden = ? OR is_default = ?)", token.ID, true, true).Find(&managedIdentities).Error; err != nil {
			return err
		}

		for _, identity := range managedIdentities {
			if identity == nil || identity.ID == "" {
				continue
			}
			updates := map[string]any{}
			if identity.DisplayName != displayName {
				updates["display_name"] = displayName
				identity.DisplayName = displayName
			}
			if identity.Color != color {
				updates["color"] = color
				identity.Color = color
			}
			if identity.AvatarAttachmentID != avatar {
				updates["avatar_attachment_id"] = avatar
				identity.AvatarAttachmentID = avatar
			}
			if len(updates) > 0 {
				if err := tx.Model(&model.ChannelIdentityModel{}).Where("id = ?", identity.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.MessageModel{}).
				Where("channel_id = ? AND sender_identity_id = ?", identity.ChannelID, identity.ID).
				Updates(map[string]any{
					"sender_member_name":        displayName,
					"sender_identity_name":      displayName,
					"sender_identity_color":     color,
					"sender_identity_avatar_id": avatar,
				}).Error; err != nil {
				return err
			}
			result.UpdatedIdentities = append(result.UpdatedIdentities, identity)
		}

		return tx.Model(&model.MessageModel{}).
			Where("user_id = ? AND (sender_identity_id = '' OR sender_identity_id IS NULL)", token.ID).
			Updates(map[string]any{
				"sender_member_name":        displayName,
				"sender_identity_name":      "",
				"sender_identity_color":     color,
				"sender_identity_avatar_id": avatar,
			}).Error
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// EnsureBotChannelIdentity creates a default channel identity for bot users once they join a channel.
func EnsureBotChannelIdentity(userID, channelID string) error {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" {
		return nil
	}
	user := model.UserGet(userID)
	if user == nil || !user.IsBot {
		return nil
	}
	displayName := strings.TrimSpace(user.Nickname)
	if displayName == "" {
		displayName = strings.TrimSpace(user.Username)
	}
	if displayName == "" {
		displayName = "Bot"
	}
	if _, err := model.MemberGetByUserIDAndChannelIDBase(user.ID, channelID, displayName, true); err != nil {
		return err
	}
	existing, err := model.ChannelIdentityList(channelID, user.ID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	sortOrder, err := model.ChannelIdentityMaxSort(channelID, user.ID)
	if err != nil {
		return err
	}
	identity := &model.ChannelIdentityModel{
		ChannelID:          channelID,
		UserID:             user.ID,
		DisplayName:        displayName,
		Color:              model.ChannelIdentityNormalizeColor(user.NickColor),
		AvatarAttachmentID: strings.TrimSpace(user.Avatar),
		SortOrder:          sortOrder + 1,
		IsDefault:          true,
	}
	return model.ChannelIdentityUpsert(identity)
}

// EnsureBotFriendships ensures every bot account is already a confirmed friend for the given user.
func EnsureBotFriendships(userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	user := model.UserGet(userID)
	if user == nil || user.ID == "" {
		return nil
	}
	bots, err := model.UserBotList()
	if err != nil {
		return err
	}
	for _, bot := range bots {
		if bot == nil || bot.ID == "" || bot.ID == userID {
			continue
		}
		if err := ensureUserBotFriendship(userID, bot.ID); err != nil {
			return err
		}
	}
	return nil
}

func ensureUserBotFriendship(userID, botID string) error {
	if _, err := model.FriendRelationFriendApprove(userID, botID); err != nil {
		return err
	}
	ch, err := model.ChannelPrivateGet(userID, botID)
	if err != nil {
		return err
	}
	if ch.ID == "" {
		_, _ = model.ChannelPrivateNew(userID, botID)
	}
	return nil
}

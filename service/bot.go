package service

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/samber/lo"

	"sealchat/model"
)

func SelectedBotIdByChannelId(channelId string) (string, error) {
	channelId = strings.TrimSpace(channelId)
	if channelId == "" {
		return "", errors.New("缺少频道ID")
	}
	roleId := fmt.Sprintf("ch-%s-%s", channelId, "bot")
	ids, _ := model.UserRoleMappingUserIdListByRoleId(roleId)
	if len(ids) == 0 {
		return "", errors.New("未选择频道机器人")
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		user := model.UserGet(id)
		if user != nil && user.IsBot {
			filtered = append(filtered, id)
		}
	}
	ids = lo.Uniq(filtered)
	if len(ids) == 0 {
		return "", errors.New("未选择频道机器人")
	}
	sort.Strings(ids)
	selected := ids[0]
	if len(ids) > 1 {
		log.Printf("[bot] channel %s has multiple bot bindings: %v, selecting %s", channelId, ids, selected)
	}
	return selected, nil
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
		// 私聊时获取授权
		var otherId string
		id2 := ch.GetPrivateUserIDs()
		if id2[0] == curUserId {
			otherId = id2[1]
		}
		if id2[1] == curUserId {
			otherId = id2[0]
		}
		u := model.UserGet(otherId)
		if u.IsBot {
			ids = append(ids, otherId)
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
	if strings.TrimSpace(token.Avatar) != "" && user.Avatar != token.Avatar {
		updates["avatar"] = token.Avatar
	}
	if strings.TrimSpace(token.NickColor) != "" && user.NickColor != token.NickColor {
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

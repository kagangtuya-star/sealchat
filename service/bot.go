package service

import (
	"encoding/json"
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

func NormalizeBotIDList(ids []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func ParseBotIDListJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		log.Printf("[bot] parse bot id list json failed: %v", err)
		return nil
	}
	return NormalizeBotIDList(ids)
}

func EncodeBotIDListJSON(ids []string) (string, error) {
	normalized := NormalizeBotIDList(ids)
	if len(normalized) == 0 {
		return "", nil
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
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

func BoundBotIDsByChannelId(channelId string) ([]string, error) {
	channelId = strings.TrimSpace(channelId)
	if channelId == "" {
		return nil, errors.New("缺少频道ID")
	}
	channel, err := model.ChannelGet(channelId)
	if err != nil {
		return nil, err
	}
	if channel == nil || channel.ID == "" {
		return nil, errors.New("频道不存在")
	}

	if selected := privateBotIDsByChannel(channel); len(selected) > 0 {
		return selected, nil
	}

	roleIDs := botEffectiveBindingRoleIDs(channel)
	result := make([]string, 0)
	for _, roleID := range roleIDs {
		ids, _ := model.UserRoleMappingUserIdListByRoleId(roleID)
		for _, id := range ids {
			user := model.UserGet(id)
			if user != nil && user.IsBot {
				result = append(result, id)
			}
		}
	}
	result = lo.Uniq(result)
	sort.Strings(result)
	return result, nil
}

func botEffectiveBindingRoleIDs(channel *model.ChannelModel) []string {
	if channel == nil {
		return nil
	}
	channelID := strings.TrimSpace(channel.ID)
	if channelID == "" {
		return nil
	}
	roleIDs := []string{fmt.Sprintf("ch-%s-bot", channelID)}
	if rootID := strings.TrimSpace(channel.RootId); rootID != "" && rootID != channelID {
		roleIDs = append(roleIDs, fmt.Sprintf("ch-%s-bot", rootID))
	}
	return roleIDs
}

func botChannelBindingRoleSetTx(tx *gorm.DB, botID string) (map[string]struct{}, error) {
	if tx == nil {
		tx = model.GetDB()
	}
	var roleIDs []string
	if err := tx.Model(&model.UserRoleMappingModel{}).
		Where("user_id = ? AND role_type = ?", strings.TrimSpace(botID), "channel").
		Pluck("role_id", &roleIDs).Error; err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(roleIDs))
	for _, roleID := range roleIDs {
		if roleID = strings.TrimSpace(roleID); roleID != "" {
			result[roleID] = struct{}{}
		}
	}
	return result, nil
}

func botIsEffectivelyBoundToChannelWithRoleSet(botID string, channel *model.ChannelModel, roleSet map[string]struct{}) bool {
	if channel == nil {
		return false
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return false
	}
	if channel.IsPrivate || strings.EqualFold(strings.TrimSpace(channel.PermType), "private") {
		for _, id := range channel.GetPrivateUserIDs() {
			if strings.TrimSpace(id) == botID {
				return true
			}
		}
		return false
	}
	for _, roleID := range botEffectiveBindingRoleIDs(channel) {
		if _, exists := roleSet[roleID]; exists {
			return true
		}
	}
	return false
}

func botBoundChannelsInWorldTx(tx *gorm.DB, botID, worldID string) ([]model.ChannelModel, error) {
	botID = strings.TrimSpace(botID)
	worldID = strings.TrimSpace(worldID)
	if tx == nil {
		tx = model.GetDB()
	}
	if botID == "" || worldID == "" {
		return nil, nil
	}
	var user model.UserModel
	if err := tx.Select("id", "is_bot").Where("id = ?", botID).Limit(1).Find(&user).Error; err != nil {
		return nil, err
	}
	if user.ID == "" || !user.IsBot {
		return nil, nil
	}
	roleSet, err := botChannelBindingRoleSetTx(tx, botID)
	if err != nil {
		return nil, err
	}
	var channels []model.ChannelModel
	if err := tx.Where("world_id = ? AND (status IS NULL OR status <> ?)", worldID, model.ChannelStatusDeleted).
		Order("id ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	result := make([]model.ChannelModel, 0, len(channels))
	for index := range channels {
		if botIsEffectivelyBoundToChannelWithRoleSet(botID, &channels[index], roleSet) {
			result = append(result, channels[index])
		}
	}
	return result, nil
}

func botBoundChannelsInWorld(botID, worldID string) ([]model.ChannelModel, error) {
	var channels []model.ChannelModel
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var err error
		channels, err = botBoundChannelsInWorldTx(tx, botID, worldID)
		return err
	})
	return channels, err
}

func EventBotIDsByChannelId(channelId string) ([]string, error) {
	channelId = strings.TrimSpace(channelId)
	if channelId == "" {
		return nil, errors.New("缺少频道ID")
	}
	channel, err := model.ChannelGet(channelId)
	if err != nil {
		return nil, err
	}
	if channel == nil || channel.ID == "" {
		return nil, errors.New("频道不存在")
	}

	boundIDs, err := BoundBotIDsByChannelId(channelId)
	if err != nil {
		return nil, err
	}
	configured := ParseBotIDListJSON(channel.EventBotIDsJSON)
	if len(configured) == 0 {
		return boundIDs, nil
	}
	boundSet := map[string]struct{}{}
	for _, id := range boundIDs {
		boundSet[id] = struct{}{}
	}
	filtered := make([]string, 0, len(configured))
	for _, id := range configured {
		if _, ok := boundSet[id]; ok {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		return boundIDs, nil
	}
	return filtered, nil
}

func PrimaryBotIdByChannelId(channelId string) (string, error) {
	channelId = strings.TrimSpace(channelId)
	if channelId == "" {
		return "", errors.New("缺少频道ID")
	}

	channel, err := model.ChannelGet(channelId)
	if err != nil {
		return "", err
	}
	if channel == nil || channel.ID == "" {
		return "", errors.New("频道不存在")
	}
	if selected := privateBotIDByChannel(channel); selected != "" {
		return selected, nil
	}

	ids, err := BoundBotIDsByChannelId(channelId)
	if err != nil {
		return "", err
	}
	if primaryID := strings.TrimSpace(channel.PrimaryBotID); primaryID != "" {
		for _, id := range ids {
			if id == primaryID {
				return primaryID, nil
			}
		}
		return "", errors.New("主控BOT未绑定到频道")
	}
	if len(ids) > 0 {
		selected := ids[0]
		if len(ids) > 1 {
			log.Printf("[bot] channel %s has multiple bot bindings: %v, selecting %s", channelId, ids, selected)
		}
		return selected, nil
	}
	return "", errors.New("未选择频道机器人")
}

func IsBotBoundToChannel(botID, channelId string) (bool, error) {
	botID = strings.TrimSpace(botID)
	channelId = strings.TrimSpace(channelId)
	if botID == "" || channelId == "" {
		return false, errors.New("缺少BOT或频道ID")
	}
	ids, err := BoundBotIDsByChannelId(channelId)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == botID {
			return true, nil
		}
	}
	return false, nil
}

func SelectedBotIdByChannelId(channelId string) (string, error) {
	return PrimaryBotIdByChannelId(channelId)
}

func BotListByChannelId(curUserId, channelId string) []string {
	ch, _ := model.ChannelGet(channelId)
	if ch.ID != "" && ch.PermType == "private" {
		// 私聊时自动将对端 bot 视为频道机器人
		var ids []string
		for _, botID := range privateBotIDsByChannel(ch) {
			if botID == curUserId {
				continue
			}
			ids = append(ids, botID)
		}
		return lo.Uniq(ids)
	}
	ids, err := BoundBotIDsByChannelId(channelId)
	if err != nil {
		return []string{}
	}
	return ids
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
	if err := ensureBoundBotSharedChannelIdentities(token.ID); err != nil {
		return nil, err
	}
	var managedIdentities []*model.ChannelIdentityModel
	if err := model.GetDB().Where("user_id = ? AND (is_hidden = ? OR is_default = ?)", token.ID, true, true).
		Order("shared_revision DESC, created_at ASC").Find(&managedIdentities).Error; err != nil {
		return nil, err
	}
	sharedAuthorities := map[string]*model.ChannelIdentityModel{}
	localIdentities := make([]*model.ChannelIdentityModel, 0, len(managedIdentities))
	for _, identity := range managedIdentities {
		if identity == nil || identity.ID == "" || strings.EqualFold(strings.TrimSpace(identity.BotAppearanceMode), "custom") {
			continue
		}
		if identity.IsDefault && identity.SharedIdentityID != "" {
			if sharedAuthorities[identity.SharedIdentityID] == nil {
				sharedAuthorities[identity.SharedIdentityID] = identity
			}
			continue
		}
		localIdentities = append(localIdentities, identity)
	}
	for _, identity := range sharedAuthorities {
		copies, err := model.SharedChannelIdentityCopies(identity.SharedIdentityID)
		if err != nil {
			return nil, err
		}
		if identity.DisplayName != displayName || identity.Color != color || identity.AvatarAttachmentID != avatar {
			syncResult, syncErr := sharedChannelIdentitySyncFromCopy(token.ID, token.ID, identity.ID, &SharedChannelIdentitySyncInput{
				DisplayName: displayName, Color: color, AvatarAttachmentID: avatar,
				AvatarDecorations: identity.AvatarDecorations,
			}, sharedChannelIdentitySyncOptions{
				botManaged:             true,
				trustStoredAvatar:      true,
				trustStoredDecorations: true,
			})
			if syncErr != nil {
				return nil, syncErr
			}
			copies = syncResult.Copies
		}
		result.UpdatedIdentities = append(result.UpdatedIdentities, copies...)
	}

	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		for _, identity := range localIdentities {
			identity.DisplayName = displayName
			identity.Color = color
			identity.AvatarAttachmentID = avatar
			if err := tx.Model(&model.ChannelIdentityModel{}).Where("id = ?", identity.ID).Updates(map[string]any{
				"display_name": displayName, "color": color, "avatar_attachment_id": avatar,
			}).Error; err != nil {
				return err
			}
			result.UpdatedIdentities = append(result.UpdatedIdentities, identity)
		}
		for _, identity := range result.UpdatedIdentities {
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

func ensureBoundBotSharedChannelIdentities(userID string) error {
	var roleMappings []model.UserRoleMappingModel
	if err := model.GetDB().Where("user_id = ? AND role_type = ?", userID, "channel").Order("role_id ASC").Find(&roleMappings).Error; err != nil {
		return err
	}
	worldIDs := make(map[string]struct{}, len(roleMappings))
	for _, mapping := range roleMappings {
		channelID := strings.TrimSpace(model.ExtractChIdFromRoleId(mapping.RoleID))
		if channelID == "" || mapping.RoleID != fmt.Sprintf("ch-%s-bot", channelID) {
			continue
		}
		channel, err := model.ChannelGet(channelID)
		if err != nil {
			return err
		}
		if channel == nil || channel.Status == model.ChannelStatusDeleted {
			continue
		}
		if worldID := strings.TrimSpace(channel.WorldID); worldID != "" {
			worldIDs[worldID] = struct{}{}
		}
	}
	orderedWorldIDs := make([]string, 0, len(worldIDs))
	for worldID := range worldIDs {
		orderedWorldIDs = append(orderedWorldIDs, worldID)
	}
	sort.Strings(orderedWorldIDs)
	for _, worldID := range orderedWorldIDs {
		channels, err := botBoundChannelsInWorld(userID, worldID)
		if err != nil {
			return err
		}
		if len(channels) == 0 {
			continue
		}
		if err := EnsureBotChannelIdentity(userID, channels[0].ID); err != nil {
			return err
		}
	}
	return nil
}

// EnsureBotChannelIdentity creates or materializes the world's canonical default BOT identity.
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
	return ensureBotSharedChannelIdentity(user, channelID)
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

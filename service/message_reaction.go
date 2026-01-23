package service

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
)

type MessageReactionSummary struct {
	MessageID string `json:"messageId"`
	Emoji     string `json:"emoji"`
	Count     int    `json:"count"`
	MeReacted bool   `json:"meReacted"`
}

func AddMessageReaction(messageID, userID, emoji, identityID string) (*MessageReactionSummary, error) {
	messageID = strings.TrimSpace(messageID)
	userID = strings.TrimSpace(userID)
	emoji = strings.TrimSpace(emoji)
	identityID = strings.TrimSpace(identityID)
	if messageID == "" || userID == "" || emoji == "" {
		return nil, fmt.Errorf("messageId、userId 和 emoji 不能为空")
	}

	db := model.GetDB()
	var summary MessageReactionSummary
	err := db.Transaction(func(tx *gorm.DB) error {
		var existing model.MessageReactionModel
		if err := tx.Where("message_id = ? AND user_id = ? AND emoji = ?", messageID, userID, emoji).
			Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		if existing.ID != "" {
			if identityID != "" && existing.IdentityID != identityID {
				if err := tx.Model(&existing).Update("identity_id", identityID).Error; err != nil {
					return err
				}
			}
			count, err := getReactionCount(tx, messageID, emoji)
			if err != nil {
				return err
			}
			summary = MessageReactionSummary{
				MessageID: messageID,
				Emoji:     emoji,
				Count:     count,
				MeReacted: true,
			}
			return nil
		}

		reaction := model.MessageReactionModel{
			MessageID: messageID,
			UserID:    userID,
			Emoji:     emoji,
			IdentityID: identityID,
		}
		if err := tx.Create(&reaction).Error; err != nil {
			// 并发情况下如果已存在，视为幂等成功
			var retry model.MessageReactionModel
			if err := tx.Where("message_id = ? AND user_id = ? AND emoji = ?", messageID, userID, emoji).
				Limit(1).Find(&retry).Error; err != nil {
				return err
			}
			if retry.ID == "" {
				return err
			}
		}

		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "message_id"},
				{Name: "emoji"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"count": gorm.Expr("count + ?", 1),
			}),
		}).Create(&model.MessageReactionCountModel{
			MessageID: messageID,
			Emoji:     emoji,
			Count:     1,
		}).Error; err != nil {
			return err
		}

		count, err := getReactionCount(tx, messageID, emoji)
		if err != nil {
			return err
		}
		summary = MessageReactionSummary{
			MessageID: messageID,
			Emoji:     emoji,
			Count:     count,
			MeReacted: true,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func RemoveMessageReaction(messageID, userID, emoji string) (*MessageReactionSummary, error) {
	messageID = strings.TrimSpace(messageID)
	userID = strings.TrimSpace(userID)
	emoji = strings.TrimSpace(emoji)
	if messageID == "" || userID == "" || emoji == "" {
		return nil, fmt.Errorf("messageId、userId 和 emoji 不能为空")
	}

	db := model.GetDB()
	var summary MessageReactionSummary
	err := db.Transaction(func(tx *gorm.DB) error {
		var existing model.MessageReactionModel
		if err := tx.Where("message_id = ? AND user_id = ? AND emoji = ?", messageID, userID, emoji).
			Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		if existing.ID == "" {
			count, err := getReactionCount(tx, messageID, emoji)
			if err != nil {
				return err
			}
			summary = MessageReactionSummary{
				MessageID: messageID,
				Emoji:     emoji,
				Count:     count,
				MeReacted: false,
			}
			return nil
		}

		if err := tx.Delete(&existing).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.MessageReactionCountModel{}).
			Where("message_id = ? AND emoji = ?", messageID, emoji).
			Update("count", gorm.Expr("count - ?", 1)).Error; err != nil {
			return err
		}

		count, err := getReactionCount(tx, messageID, emoji)
		if err != nil {
			return err
		}
		summary = MessageReactionSummary{
			MessageID: messageID,
			Emoji:     emoji,
			Count:     count,
			MeReacted: false,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func ListMessageReactions(messageID, userID string) ([]model.MessageReactionListItem, error) {
	messageID = strings.TrimSpace(messageID)
	userID = strings.TrimSpace(userID)
	if messageID == "" || userID == "" {
		return nil, fmt.Errorf("messageId 和 userId 不能为空")
	}

	db := model.GetDB()
	var counts []model.MessageReactionCountModel
	if err := db.Where("message_id = ?", messageID).Order("count desc").Find(&counts).Error; err != nil {
		return nil, err
	}

	var userReactions []model.MessageReactionModel
	if err := db.Select("emoji").Where("message_id = ? AND user_id = ?", messageID, userID).
		Find(&userReactions).Error; err != nil {
		return nil, err
	}
	reacted := make(map[string]struct{}, len(userReactions))
	for _, item := range userReactions {
		reacted[item.Emoji] = struct{}{}
	}

	result := make([]model.MessageReactionListItem, 0, len(counts))
	for _, item := range counts {
		_, me := reacted[item.Emoji]
		result = append(result, model.MessageReactionListItem{
			Emoji:     item.Emoji,
			Count:     item.Count,
			MeReacted: me,
		})
	}
	return result, nil
}

func ListMessageReactionsForMessages(messageIDs []string, userID string) (map[string][]model.MessageReactionListItem, error) {
	userID = strings.TrimSpace(userID)
	if len(messageIDs) == 0 || userID == "" {
		return map[string][]model.MessageReactionListItem{}, nil
	}

	unique := make([]string, 0, len(messageIDs))
	seen := map[string]struct{}{}
	for _, id := range messageIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		unique = append(unique, trimmed)
	}
	if len(unique) == 0 {
		return map[string][]model.MessageReactionListItem{}, nil
	}

	db := model.GetDB()
	var counts []model.MessageReactionCountModel
	if err := db.
		Select("message_id, emoji, count").
		Where("message_id IN ?", unique).
		Order("message_id, count desc").
		Find(&counts).Error; err != nil {
		return nil, err
	}

	var userReactions []model.MessageReactionModel
	if err := db.
		Select("message_id, emoji").
		Where("message_id IN ? AND user_id = ?", unique, userID).
		Find(&userReactions).Error; err != nil {
		return nil, err
	}

	reacted := map[string]map[string]struct{}{}
	for _, item := range userReactions {
		if item.MessageID == "" || item.Emoji == "" {
			continue
		}
		set := reacted[item.MessageID]
		if set == nil {
			set = map[string]struct{}{}
			reacted[item.MessageID] = set
		}
		set[item.Emoji] = struct{}{}
	}

	result := map[string][]model.MessageReactionListItem{}
	for _, item := range counts {
		if item.MessageID == "" || item.Emoji == "" || item.Count <= 0 {
			continue
		}
		_, me := reacted[item.MessageID][item.Emoji]
		result[item.MessageID] = append(result[item.MessageID], model.MessageReactionListItem{
			Emoji:     item.Emoji,
			Count:     item.Count,
			MeReacted: me,
		})
	}
	return result, nil
}

type MessageReactionUserItem struct {
	UserID        string `json:"userId"`
	IdentityID    string `json:"identityId,omitempty"`
	DisplayName   string `json:"displayName"`
	IdentityColor string `json:"identityColor,omitempty"`
}

func ListMessageReactionUsers(messageID, channelID, emoji string, limit, offset int) ([]MessageReactionUserItem, int, error) {
	messageID = strings.TrimSpace(messageID)
	channelID = strings.TrimSpace(channelID)
	emoji = strings.TrimSpace(emoji)
	if messageID == "" || channelID == "" || emoji == "" {
		return nil, 0, fmt.Errorf("messageId、channelId 和 emoji 不能为空")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	db := model.GetDB()
	var totalCount int
	if err := db.Model(&model.MessageReactionCountModel{}).
		Select("count").
		Where("message_id = ? AND emoji = ?", messageID, emoji).
		Limit(1).
		Scan(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var reactions []model.MessageReactionModel
	if err := db.Select("user_id, identity_id, created_at").
		Where("message_id = ? AND emoji = ?", messageID, emoji).
		Order("created_at asc").
		Limit(limit).
		Offset(offset).
		Find(&reactions).Error; err != nil {
		return nil, 0, err
	}
	if len(reactions) == 0 {
		return []MessageReactionUserItem{}, totalCount, nil
	}

	identityIDs := make([]string, 0, len(reactions))
	identityIDSeen := map[string]struct{}{}
	userIDs := make([]string, 0, len(reactions))
	userIDSeen := map[string]struct{}{}
	for _, r := range reactions {
		if r.UserID != "" {
			if _, ok := userIDSeen[r.UserID]; !ok {
				userIDSeen[r.UserID] = struct{}{}
				userIDs = append(userIDs, r.UserID)
			}
		}
		if r.IdentityID != "" {
			if _, ok := identityIDSeen[r.IdentityID]; !ok {
				identityIDSeen[r.IdentityID] = struct{}{}
				identityIDs = append(identityIDs, r.IdentityID)
			}
		}
	}

	identityByID := map[string]*model.ChannelIdentityModel{}
	if len(identityIDs) > 0 {
		var identities []model.ChannelIdentityModel
		if err := db.Select("id, user_id, display_name, color").
			Where("channel_id = ? AND id IN ?", channelID, identityIDs).
			Find(&identities).Error; err != nil {
			return nil, 0, err
		}
		for i := range identities {
			item := identities[i]
			if item.ID == "" {
				continue
			}
			identityByID[item.ID] = &item
		}
	}

	missingUserIDs := make([]string, 0)
	missingSeen := map[string]struct{}{}
	for _, r := range reactions {
		if r.UserID == "" {
			continue
		}
		if r.IdentityID == "" || identityByID[r.IdentityID] == nil {
			if _, ok := missingSeen[r.UserID]; !ok {
				missingSeen[r.UserID] = struct{}{}
				missingUserIDs = append(missingUserIDs, r.UserID)
			}
		}
	}

	defaultIdentityByUser := map[string]*model.ChannelIdentityModel{}
	if len(missingUserIDs) > 0 {
		var defaults []model.ChannelIdentityModel
		if err := db.Select("id, user_id, display_name, color").
			Where("channel_id = ? AND user_id IN ? AND is_default = ?", channelID, missingUserIDs, true).
			Find(&defaults).Error; err != nil {
			return nil, 0, err
		}
		for i := range defaults {
			item := defaults[i]
			if item.UserID == "" {
				continue
			}
			defaultIdentityByUser[item.UserID] = &item
		}
	}

	displayNameByUser := map[string]string{}
	if len(userIDs) > 0 {
		type userRow struct {
			ID       string
			Nickname string
			Username string
		}
		var users []userRow
		if err := db.Table("users").
			Select("id, nickname, username").
			Where("id IN ?", userIDs).
			Find(&users).Error; err != nil {
			return nil, 0, err
		}
		for _, u := range users {
			if u.ID == "" {
				continue
			}
			displayNameByUser[u.ID] = pickDisplayName("", u.Nickname, u.Username, u.ID)
		}

		type memberRow struct {
			UserID     string
			MemberNick string
			UserNick   string
			Username   string
		}
		var members []memberRow
		if err := db.Table("members AS m").
			Select("m.user_id AS user_id, m.nickname AS member_nick, u.nickname AS user_nick, u.username AS username").
			Joins("LEFT JOIN users u ON u.id = m.user_id").
			Where("m.channel_id = ? AND m.user_id IN ?", channelID, userIDs).
			Find(&members).Error; err != nil {
			return nil, 0, err
		}
		for _, m := range members {
			if m.UserID == "" {
				continue
			}
			displayNameByUser[m.UserID] = pickDisplayName(m.MemberNick, m.UserNick, m.Username, m.UserID)
		}
	}

	items := make([]MessageReactionUserItem, 0, len(reactions))
	for _, r := range reactions {
		if r.UserID == "" {
			continue
		}
		var (
			displayName string
			identityID  string
			color       string
		)
		if r.IdentityID != "" {
			if identity := identityByID[r.IdentityID]; identity != nil {
				identityID = identity.ID
				displayName = strings.TrimSpace(identity.DisplayName)
				color = strings.TrimSpace(identity.Color)
			}
		}
		if displayName == "" {
			if identity := defaultIdentityByUser[r.UserID]; identity != nil {
				identityID = identity.ID
				displayName = strings.TrimSpace(identity.DisplayName)
				color = strings.TrimSpace(identity.Color)
			}
		}
		if displayName == "" {
			displayName = displayNameByUser[r.UserID]
		}
		if displayName == "" {
			displayName = r.UserID
		}
		item := MessageReactionUserItem{
			UserID:      r.UserID,
			DisplayName: displayName,
		}
		if identityID != "" {
			item.IdentityID = identityID
		}
		if color != "" {
			item.IdentityColor = color
		}
		items = append(items, item)
	}

	return items, totalCount, nil
}

func getReactionCount(tx *gorm.DB, messageID, emoji string) (int, error) {
	var count model.MessageReactionCountModel
	if err := tx.Where("message_id = ? AND emoji = ?", messageID, emoji).Limit(1).Find(&count).Error; err != nil {
		return 0, err
	}
	if count.ID == "" {
		return 0, nil
	}
	if count.Count <= 0 {
		_ = tx.Delete(&count).Error
		return 0, nil
	}
	return count.Count, nil
}

func pickDisplayName(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "未知成员"
}

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

func AddMessageReaction(messageID, userID, emoji string) (*MessageReactionSummary, error) {
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
		if existing.ID != "" {
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

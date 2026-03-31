package model

import (
	"strings"

	"gorm.io/gorm"
)

type MessageWhisperRecipientModel struct {
	StringPKBaseModel
	MessageID string `json:"message_id" gorm:"size:100;uniqueIndex:idx_mwr_unique,priority:1;index:idx_mwr_message"`
	UserID    string `json:"user_id" gorm:"size:100;uniqueIndex:idx_mwr_unique,priority:2;index:idx_mwr_user"`
}

func (*MessageWhisperRecipientModel) TableName() string {
	return "message_whisper_recipients"
}

// GetWhisperRecipientIDs 获取消息的所有收件人ID
func GetWhisperRecipientIDs(messageID string) []string {
	if messageID == "" {
		return nil
	}
	var recipients []MessageWhisperRecipientModel
	GetDB().Where("message_id = ?", messageID).Find(&recipients)
	ids := make([]string, len(recipients))
	for i, r := range recipients {
		ids[i] = r.UserID
	}
	return ids
}

// GetWhisperRecipientIDsBatch 批量获取消息收件人ID
func GetWhisperRecipientIDsBatch(messageIDs []string) map[string][]string {
	if len(messageIDs) == 0 {
		return nil
	}
	var recipients []MessageWhisperRecipientModel
	GetDB().Where("message_id IN ?", messageIDs).Find(&recipients)
	result := make(map[string][]string)
	for _, r := range recipients {
		result[r.MessageID] = append(result[r.MessageID], r.UserID)
	}
	return result
}

// CreateWhisperRecipients 批量创建收件人记录
func CreateWhisperRecipients(messageID string, userIDs []string) error {
	if messageID == "" || len(userIDs) == 0 {
		return nil
	}
	normalizedIDs := normalizeWhisperRecipientIDs(userIDs)
	if len(normalizedIDs) == 0 {
		return nil
	}
	recipients := make([]MessageWhisperRecipientModel, len(normalizedIDs))
	for i, uid := range normalizedIDs {
		recipients[i] = MessageWhisperRecipientModel{
			MessageID: messageID,
			UserID:    uid,
		}
		recipients[i].Init()
	}
	return GetDB().Create(&recipients).Error
}

func normalizeWhisperRecipientIDs(userIDs []string) []string {
	if len(userIDs) == 0 {
		return nil
	}
	result := make([]string, 0, len(userIDs))
	seen := make(map[string]struct{}, len(userIDs))
	for _, uid := range userIDs {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		result = append(result, uid)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// ReplaceWhisperRecipients 全量替换消息的收件人记录
func ReplaceWhisperRecipients(messageID string, userIDs []string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil
	}
	normalizedIDs := normalizeWhisperRecipientIDs(userIDs)
	return GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("message_id = ?", messageID).Delete(&MessageWhisperRecipientModel{}).Error; err != nil {
			return err
		}
		if len(normalizedIDs) == 0 {
			return nil
		}
		recipients := make([]MessageWhisperRecipientModel, len(normalizedIDs))
		for i, uid := range normalizedIDs {
			recipients[i] = MessageWhisperRecipientModel{
				MessageID: messageID,
				UserID:    uid,
			}
			recipients[i].Init()
		}
		return tx.Create(&recipients).Error
	})
}

// HasWhisperRecipient 判断用户是否为消息收件人
func HasWhisperRecipient(messageID, userID string) bool {
	if messageID == "" || userID == "" {
		return false
	}
	var count int64
	GetDB().Model(&MessageWhisperRecipientModel{}).
		Where("message_id = ? AND user_id = ?", messageID, userID).
		Count(&count)
	return count > 0
}

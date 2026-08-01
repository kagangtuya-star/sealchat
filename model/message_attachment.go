package model

import (
	"regexp"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MessageAttachmentKindImage = "image"
	messageAttachmentBatchSize = 200
)

var (
	messageAttachmentIDPattern = regexp.MustCompile(`id:([a-zA-Z0-9_-]+)`)
	messageImgSrcPattern       = regexp.MustCompile(`<img[^>]+src=["']id:([a-zA-Z0-9_-]+)["'][^>]*>`)
	messageImageSrcPattern     = regexp.MustCompile(`<image[^>]+src=["']id:([a-zA-Z0-9_-]+)["'][^>]*>`)
)

type MessageAttachmentModel struct {
	MessageID    string `json:"message_id" gorm:"primaryKey;size:100;index:idx_message_attachment_lookup,priority:1"`
	AttachmentID string `json:"attachment_id" gorm:"primaryKey;size:100"`
	Kind         string `json:"kind" gorm:"size:16;not null;index:idx_message_attachment_lookup,priority:2"`
	Position     int    `json:"position" gorm:"not null;index:idx_message_attachment_lookup,priority:3"`
}

func (*MessageAttachmentModel) TableName() string {
	return "message_attachments"
}

type ChannelMessageAttachmentBackfillState struct {
	StringPKBaseModel
	LastID      string     `json:"last_id" gorm:"size:100"`
	CompletedAt *time.Time `json:"completed_at"`
}

func (*ChannelMessageAttachmentBackfillState) TableName() string {
	return "channel_message_attachment_backfill_states"
}

var channelMessageAttachmentBackfillMu sync.Mutex

func ExtractMessageImageAttachmentIDs(content string) []string {
	if content == "" {
		return nil
	}

	ids := make([]string, 0)
	seen := map[string]struct{}{}
	appendMatches := func(pattern *regexp.Regexp) {
		for _, match := range pattern.FindAllStringSubmatch(content, -1) {
			if len(match) < 2 {
				continue
			}
			id := match[1]
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	appendMatches(messageImgSrcPattern)
	appendMatches(messageImageSrcPattern)
	if len(ids) == 0 {
		appendMatches(messageAttachmentIDPattern)
	}
	return ids
}

func ReplaceMessageImageAttachments(tx *gorm.DB, messageID, content string) error {
	if tx == nil || messageID == "" {
		return nil
	}
	return tx.Transaction(func(conn *gorm.DB) error {
		if err := conn.Where("message_id = ? AND kind = ?", messageID, MessageAttachmentKindImage).
			Delete(&MessageAttachmentModel{}).Error; err != nil {
			return err
		}

		ids := ExtractMessageImageAttachmentIDs(content)
		if len(ids) == 0 {
			return nil
		}
		items := make([]MessageAttachmentModel, 0, len(ids))
		for position, attachmentID := range ids {
			items = append(items, MessageAttachmentModel{
				MessageID:    messageID,
				AttachmentID: attachmentID,
				Kind:         MessageAttachmentKindImage,
				Position:     position,
			})
		}
		return conn.Clauses(clause.OnConflict{DoNothing: true}).Create(&items).Error
	})
}

func MessageImageAttachmentIDsByMessageIDs(conn *gorm.DB, messageIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(messageIDs))
	if conn == nil || len(messageIDs) == 0 {
		return result, nil
	}
	var rows []MessageAttachmentModel
	if err := conn.Where("message_id IN ? AND kind = ?", messageIDs, MessageAttachmentKindImage).
		Order("message_id ASC").
		Order("position ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.MessageID] = append(result[row.MessageID], row.AttachmentID)
	}
	return result, nil
}

func BackfillMessageImageAttachmentsForChannel(conn *gorm.DB, channelID string) error {
	if conn == nil || channelID == "" {
		return nil
	}
	channelMessageAttachmentBackfillMu.Lock()
	defer channelMessageAttachmentBackfillMu.Unlock()

	now := time.Now()
	if err := conn.Clauses(clause.OnConflict{DoNothing: true}).Create(&ChannelMessageAttachmentBackfillState{
		StringPKBaseModel: StringPKBaseModel{ID: channelID, CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		return err
	}

	for {
		var state ChannelMessageAttachmentBackfillState
		if err := conn.Where("id = ?", channelID).Limit(1).Find(&state).Error; err != nil {
			return err
		}
		if state.CompletedAt != nil {
			return nil
		}

		var messages []MessageModel
		query := conn.Select("id, content").Where("channel_id = ? AND content <> ''", channelID)
		if state.LastID != "" {
			query = query.Where("id > ?", state.LastID)
		}
		if err := query.Order("id ASC").Limit(messageAttachmentBatchSize).Find(&messages).Error; err != nil {
			return err
		}
		if len(messages) == 0 {
			finishedAt := time.Now()
			return conn.Model(&ChannelMessageAttachmentBackfillState{}).
				Where("id = ?", channelID).
				Updates(map[string]any{"completed_at": finishedAt, "updated_at": finishedAt}).Error
		}

		for _, message := range messages {
			if err := syncMessageImageAttachmentsByID(conn, message.ID); err != nil {
				return err
			}
		}
		lastID := messages[len(messages)-1].ID
		if err := conn.Model(&ChannelMessageAttachmentBackfillState{}).
			Where("id = ?", channelID).
			Updates(map[string]any{"last_id": lastID, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
	}
}

func syncMessageImageAttachmentsByID(conn *gorm.DB, messageID string) error {
	return conn.Transaction(func(tx *gorm.DB) error {
		var message MessageModel
		if err := tx.Select("id, content").Where("id = ?", messageID).Limit(1).Find(&message).Error; err != nil {
			return err
		}
		if message.ID == "" {
			return nil
		}
		return ReplaceMessageImageAttachments(tx, message.ID, message.Content)
	})
}

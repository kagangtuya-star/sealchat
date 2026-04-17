package model

import (
	"sealchat/pkg/contentstats"
)

const messageVisibleCharCountBatchSize = 500

func BackfillMessageVisibleCharCount() error {
	if db == nil {
		return nil
	}

	lastID := ""
	for {
		var batch []MessageModel
		query := db.
			Model(&MessageModel{}).
			Select("id", "content", "visible_char_count").
			Order("id ASC").
			Limit(messageVisibleCharCountBatchSize)
		if lastID != "" {
			query = query.Where("id > ?", lastID)
		}
		if err := query.Find(&batch).Error; err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}

		for _, item := range batch {
			nextCount := contentstats.CountVisibleTextChars(item.Content)
			if item.VisibleCharCount == nextCount {
				continue
			}
			if err := db.Model(&MessageModel{}).Where("id = ?", item.ID).Update("visible_char_count", nextCount).Error; err != nil {
				return err
			}
		}

		lastID = batch[len(batch)-1].ID
		if len(batch) < messageVisibleCharCountBatchSize {
			return nil
		}
	}
}

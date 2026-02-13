package model

import (
	"strings"

	"gorm.io/gorm/clause"
)

type ChannelAttachmentImageLayoutModel struct {
	StringPKBaseModel
	ChannelID    string `json:"channel_id" gorm:"index;uniqueIndex:idx_channel_attachment_image_layout,priority:1"`
	AttachmentID string `json:"attachment_id" gorm:"index;uniqueIndex:idx_channel_attachment_image_layout,priority:2"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	UpdatedBy    string `json:"updated_by" gorm:"index"`
}

func (*ChannelAttachmentImageLayoutModel) TableName() string {
	return "channel_attachment_image_layouts"
}

type ChannelAttachmentImageLayoutUpsertItem struct {
	AttachmentID string
	Width        int
	Height       int
}

func ChannelAttachmentImageLayoutBatchGet(channelID string, attachmentIDs []string) ([]*ChannelAttachmentImageLayoutModel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || len(attachmentIDs) == 0 {
		return []*ChannelAttachmentImageLayoutModel{}, nil
	}
	ids := make([]string, 0, len(attachmentIDs))
	seen := map[string]struct{}{}
	for _, id := range attachmentIDs {
		normalized := strings.TrimSpace(id)
		if strings.HasPrefix(normalized, "id:") {
			normalized = strings.TrimPrefix(normalized, "id:")
		}
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		ids = append(ids, normalized)
	}
	if len(ids) == 0 {
		return []*ChannelAttachmentImageLayoutModel{}, nil
	}
	var items []*ChannelAttachmentImageLayoutModel
	err := GetDB().
		Where("channel_id = ?", channelID).
		Where("attachment_id IN ?", ids).
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func ChannelAttachmentImageLayoutUpsertBatch(channelID string, updatedBy string, items []ChannelAttachmentImageLayoutUpsertItem) error {
	channelID = strings.TrimSpace(channelID)
	updatedBy = strings.TrimSpace(updatedBy)
	if channelID == "" || len(items) == 0 {
		return nil
	}
	records := make([]*ChannelAttachmentImageLayoutModel, 0, len(items))
	for _, item := range items {
		attachmentID := strings.TrimSpace(item.AttachmentID)
		if strings.HasPrefix(attachmentID, "id:") {
			attachmentID = strings.TrimPrefix(attachmentID, "id:")
		}
		if attachmentID == "" {
			continue
		}
		records = append(records, &ChannelAttachmentImageLayoutModel{
			ChannelID:    channelID,
			AttachmentID: attachmentID,
			Width:        item.Width,
			Height:       item.Height,
			UpdatedBy:    updatedBy,
		})
	}
	if len(records) == 0 {
		return nil
	}
	return GetDB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "attachment_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"width", "height", "updated_by", "updated_at"}),
	}).Create(&records).Error
}

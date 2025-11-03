package model

import (
	"errors"
	"time"

	"sealchat/protocol"
)

const displayOrderBaseGap = 1024.0

type MessageModel struct {
	StringPKBaseModel
	Content      string  `json:"content"`
	ChannelID    string  `json:"channel_id" gorm:"size:100;index:idx_msg_channel_order,priority:1"`
	GuildID      string  `json:"guild_id" gorm:"null;size:100"`
	MemberID     string  `json:"member_id" gorm:"null;size:100"`
	UserID       string  `json:"user_id" gorm:"null;size:100"`
	QuoteID      string  `json:"quote_id" gorm:"null;size:100"`
	DisplayOrder float64 `json:"display_order" gorm:"type:decimal(24,8);index:idx_msg_channel_order,priority:2"`
	IsRevoked    bool    `json:"is_revoked" gorm:"null"` // 被撤回。这样实现可能不很严肃，但是能填补窗口中空白
	IsWhisper    bool    `json:"is_whisper" gorm:"default:false"`
	WhisperTo    string  `json:"whisper_to" gorm:"size:100"`
	IsEdited     bool    `json:"is_edited" gorm:"default:false"`
	EditCount    int     `json:"edit_count" gorm:"default:0"`

	SenderMemberName string `json:"sender_member_name"` // 用户在当时的名字

	User   *UserModel    `json:"user"`           // 嵌套 User 结构体
	Member *MemberModel  `json:"member"`         // 嵌套 Member 结构体
	Quote  *MessageModel `json:"quote" gorm:"-"` // 嵌套 Message 结构体
	// WhisperTarget 为前端展示提供冗余
	WhisperTarget *UserModel `json:"whisper_target" gorm:"-"`
}

func (*MessageModel) TableName() string {
	return "messages"
}

func (m *MessageModel) ToProtocolType2(channelData *protocol.Channel) *protocol.Message {
	var updatedAt int64
	if !m.UpdatedAt.IsZero() {
		updatedAt = m.UpdatedAt.UnixMilli()
	}
	return &protocol.Message{
		ID:           m.ID,
		Content:      m.Content,
		Channel:      channelData,
		CreatedAt:    m.CreatedAt.UnixMilli(),
		UpdatedAt:    updatedAt,
		DisplayOrder: m.DisplayOrder,
		IsWhisper:    m.IsWhisper,
		IsEdited:     m.IsEdited,
		EditCount:    m.EditCount,
		WhisperTo: func() *protocol.User {
			if m.WhisperTarget != nil {
				return m.WhisperTarget.ToProtocolType()
			}
			return nil
		}(),
	}
}

func BackfillMessageDisplayOrder() error {
	const batchSize = 500
	for {
		var msgs []MessageModel
		err := db.
			Where("display_order IS NULL OR display_order = 0").
			Order("created_at asc").
			Limit(batchSize).
			Find(&msgs).Error
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			break
		}
		for _, msg := range msgs {
			order := float64(msg.CreatedAt.UnixMilli())
			if order == 0 {
				order = float64(time.Now().UnixMilli())
			}
			if err := db.Model(&MessageModel{}).
				Where("id = ?", msg.ID).
				UpdateColumn("display_order", order).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func RebalanceChannelDisplayOrder(channelID string) error {
	const batchSize = 500
	offset := 0
	for {
		var msgs []MessageModel
		err := db.Where("channel_id = ?", channelID).
			Order("display_order asc").
			Order("created_at asc").
			Order("id asc").
			Limit(batchSize).
			Offset(offset).
			Find(&msgs).Error
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			break
		}
		for i, msg := range msgs {
			order := float64(offset+i+1) * displayOrderBaseGap
			if err := db.Model(&MessageModel{}).
				Where("id = ?", msg.ID).
				UpdateColumn("display_order", order).Error; err != nil {
				return err
			}
		}
		offset += len(msgs)
	}
	return nil
}

type MessageEditHistoryModel struct {
	StringPKBaseModel
	MessageID    string `json:"message_id" gorm:"index"`
	EditorID     string `json:"editor_id" gorm:"index"`
	PrevContent  string `json:"prev_content"`
	ChannelID    string `json:"channel_id" gorm:"index"`
	EditedUserID string `json:"edited_user_id" gorm:"index"`
}

func (*MessageEditHistoryModel) TableName() string {
	return "message_edit_histories"
}

func MessagesCountByChannelIDsAfterTime(channelIDs []string, updateTimes []time.Time, userID string) (map[string]int64, error) {
	// updateTimes []int64
	if len(channelIDs) != len(updateTimes) {
		return nil, errors.New("channelIDs和updateTimes长度不匹配")
	}

	var results []struct {
		ChannelID string
		Count     int64
	}

	query := db.Model(&MessageModel{}).
		Select("channel_id, count(*) as count").
		Where("user_id <> ?", userID)

	// 使用gorm的条件构建器
	conditions := db.Where("1 = 0") // 初始为false的条件
	for i, channelID := range channelIDs {
		conditions = conditions.Or(db.Where("channel_id = ? AND created_at > ?", channelID, updateTimes[i]))
	}

	err := query.Where(conditions).
		Group("channel_id").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	// 转换为map
	countMap := make(map[string]int64)
	for _, result := range results {
		countMap[result.ChannelID] = result.Count
	}

	return countMap, nil
}

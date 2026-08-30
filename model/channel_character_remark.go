package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelCharacterRemarkModel stores the latest remark for one identity in one channel.
// Empty Content is a persisted clear marker, retaining Revision for event ordering.
type ChannelCharacterRemarkModel struct {
	ChannelID  string `json:"channelId" gorm:"primaryKey;size:100;not null"`
	IdentityID string `json:"identityId" gorm:"primaryKey;size:100;not null"`
	UserID     string `json:"userId" gorm:"size:100;not null"`
	Content    string `json:"content" gorm:"size:80;not null;default:''"`
	Revision   int64  `json:"revision" gorm:"not null;default:0"`
}

func (*ChannelCharacterRemarkModel) TableName() string {
	return "channel_character_remarks"
}

// CharacterRemarkSave creates or updates one channel-scoped remark.
// Revision stays monotonic across restarts and concurrent writers.
func CharacterRemarkSave(channelID, identityID, userID, content string) (*ChannelCharacterRemarkModel, error) {
	return CharacterRemarkSaveTx(db, channelID, identityID, userID, content)
}

func CharacterRemarkSaveTx(conn *gorm.DB, channelID, identityID, userID, content string) (*ChannelCharacterRemarkModel, error) {
	channelID = strings.TrimSpace(channelID)
	identityID = strings.TrimSpace(identityID)
	userID = strings.TrimSpace(userID)
	if conn == nil || channelID == "" || identityID == "" || userID == "" {
		return nil, errors.New("角色备注参数不能为空")
	}

	content = strings.TrimSpace(content)
	item := &ChannelCharacterRemarkModel{}
	err := conn.Transaction(func(tx *gorm.DB) error {
		locked := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("channel_id = ? AND identity_id = ?", channelID, identityID).
			Limit(1).
			Find(item)
		if locked.Error != nil {
			return locked.Error
		}

		now := time.Now().UnixMilli()
		if item.ChannelID == "" {
			candidate := &ChannelCharacterRemarkModel{
				ChannelID:  channelID,
				IdentityID: identityID,
				UserID:     userID,
				Content:    content,
				Revision:   now,
			}
			created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(candidate)
			if created.Error != nil {
				return created.Error
			}
			if created.RowsAffected > 0 {
				*item = *candidate
				return nil
			}
			// Another writer created the row while this transaction was starting.
			locked = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("channel_id = ? AND identity_id = ?", channelID, identityID).
				Limit(1).
				Find(item)
			if locked.Error != nil {
				return locked.Error
			}
		}

		if now <= item.Revision {
			now = item.Revision + 1
		}
		item.ChannelID = channelID
		item.IdentityID = identityID
		item.UserID = userID
		item.Content = content
		item.Revision = now
		return tx.Model(&ChannelCharacterRemarkModel{}).
			Where("channel_id = ? AND identity_id = ?", channelID, identityID).
			Updates(map[string]any{
				"user_id":  userID,
				"content":  content,
				"revision": now,
			}).Error
	})
	if err != nil {
		return nil, err
	}
	return item, nil
}

// CharacterRemarkListByChannel returns only visible remarks for one channel.
func CharacterRemarkListByChannel(channelID string) ([]*ChannelCharacterRemarkModel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return []*ChannelCharacterRemarkModel{}, nil
	}
	var items []*ChannelCharacterRemarkModel
	err := db.Where("channel_id = ? AND content <> ''", channelID).
		Order("identity_id ASC").
		Find(&items).Error
	return items, err
}

func CharacterRemarkDeleteByChannelTx(conn *gorm.DB, channelID string) error {
	channelID = strings.TrimSpace(channelID)
	if conn == nil || channelID == "" {
		return nil
	}
	return conn.Where("channel_id = ?", channelID).Delete(&ChannelCharacterRemarkModel{}).Error
}

func CharacterRemarkDeleteByChannel(channelID string) error {
	return CharacterRemarkDeleteByChannelTx(db, channelID)
}

func CharacterRemarkDeleteByIdentityTx(conn *gorm.DB, channelID, identityID string) error {
	channelID = strings.TrimSpace(channelID)
	identityID = strings.TrimSpace(identityID)
	if conn == nil || channelID == "" || identityID == "" {
		return nil
	}
	return conn.Where("channel_id = ? AND identity_id = ?", channelID, identityID).
		Delete(&ChannelCharacterRemarkModel{}).Error
}

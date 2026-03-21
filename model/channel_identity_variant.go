package model

import (
	"encoding/json"
	"strings"

	"gorm.io/gorm"
)

type ChannelIdentityVariantModel struct {
	StringPKBaseModel
	IdentityID         string `json:"identityId" gorm:"size:100;index:idx_identity_variant_identity,priority:1;index:idx_identity_variant_channel_user,priority:3"`
	ChannelID          string `json:"channelId" gorm:"size:100;index:idx_identity_variant_channel_user,priority:1"`
	UserID             string `json:"userId" gorm:"size:100;index:idx_identity_variant_channel_user,priority:2"`
	SelectorEmoji      string `json:"selectorEmoji" gorm:"size:64"`
	Keyword            string `json:"keyword" gorm:"size:64;index"`
	Note               string `json:"note" gorm:"size:255"`
	AvatarAttachmentID string `json:"avatarAttachmentId" gorm:"size:100"`
	DisplayName        string `json:"displayName" gorm:"size:64"`
	Color              string `json:"color" gorm:"size:16"`
	AppearanceJSON     string `json:"-" gorm:"type:text;not null;default:''"`
	SortOrder          int    `json:"sortOrder" gorm:"index"`
	Enabled            bool   `json:"enabled" gorm:"default:true"`
}

func (*ChannelIdentityVariantModel) TableName() string {
	return "channel_identity_variants"
}

func (m *ChannelIdentityVariantModel) Appearance() map[string]any {
	if strings.TrimSpace(m.AppearanceJSON) == "" {
		return map[string]any{}
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(m.AppearanceJSON), &data); err != nil {
		return map[string]any{}
	}
	if data == nil {
		return map[string]any{}
	}
	return data
}

func ChannelIdentityVariantGetByID(id string) (*ChannelIdentityVariantModel, error) {
	var item ChannelIdentityVariantModel
	if err := db.Where("id = ?", id).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &item, nil
}

func ChannelIdentityVariantList(channelID string, userID string) ([]*ChannelIdentityVariantModel, error) {
	var items []*ChannelIdentityVariantModel
	err := db.Where("channel_id = ? AND user_id = ?", channelID, userID).
		Order("identity_id ASC, sort_order ASC, created_at ASC").
		Find(&items).Error
	return items, err
}

func ChannelIdentityVariantListByIdentityID(channelID string, userID string, identityID string) ([]*ChannelIdentityVariantModel, error) {
	var items []*ChannelIdentityVariantModel
	err := db.Where("channel_id = ? AND user_id = ? AND identity_id = ?", channelID, userID, identityID).
		Order("sort_order ASC, created_at ASC").
		Find(&items).Error
	return items, err
}

func ChannelIdentityVariantListByIdentityIDs(channelID string, userID string, identityIDs []string) ([]*ChannelIdentityVariantModel, error) {
	if len(identityIDs) == 0 {
		return []*ChannelIdentityVariantModel{}, nil
	}
	var items []*ChannelIdentityVariantModel
	err := db.Where("channel_id = ? AND user_id = ?", channelID, userID).
		Where("identity_id IN ?", identityIDs).
		Order("identity_id ASC, sort_order ASC, created_at ASC").
		Find(&items).Error
	return items, err
}

func ChannelIdentityVariantMaxSort(channelID string, userID string, identityID string) (int, error) {
	var sort int
	err := db.Model(&ChannelIdentityVariantModel{}).
		Where("channel_id = ? AND user_id = ? AND identity_id = ?", channelID, userID, identityID).
		Select("coalesce(max(sort_order), 0)").
		Scan(&sort).Error
	return sort, err
}

func ChannelIdentityVariantUpsert(item *ChannelIdentityVariantModel) error {
	return db.Save(item).Error
}

func ChannelIdentityVariantUpdate(id string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	return db.Model(&ChannelIdentityVariantModel{}).Where("id = ?", id).Updates(values).Error
}

func ChannelIdentityVariantDelete(id string) error {
	return db.Where("id = ?", id).Delete(&ChannelIdentityVariantModel{}).Error
}

func ChannelIdentityVariantDeleteByIdentityIDs(identityIDs []string) error {
	if len(identityIDs) == 0 {
		return nil
	}
	return db.Where("identity_id IN ?", identityIDs).Delete(&ChannelIdentityVariantModel{}).Error
}

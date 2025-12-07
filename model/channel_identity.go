package model

import (
	"errors"
	"sealchat/protocol"
	"strings"

	"gorm.io/gorm"
)

type ChannelIdentityModel struct {
	StringPKBaseModel
	ChannelID          string   `json:"channelId" gorm:"size:100;index:idx_channel_identity_channel_user,priority:1"`
	UserID             string   `json:"userId" gorm:"size:100;index:idx_channel_identity_channel_user,priority:2"`
	DisplayName        string   `json:"displayName"`
	Color              string   `json:"color"`
	AvatarAttachmentID string   `json:"avatarAttachmentId"`
	IsDefault          bool     `json:"isDefault" gorm:"default:false"`
	IsHidden           bool     `json:"isHidden" gorm:"default:false"`
	SortOrder          int      `json:"sortOrder" gorm:"index"`
	FolderIDs          []string `json:"folderIds,omitempty" gorm:"-"`
}

func (*ChannelIdentityModel) TableName() string {
	return "channel_identities"
}

func (m *ChannelIdentityModel) ToProtocolType() *protocol.ChannelIdentity {
	return &protocol.ChannelIdentity{
		ID:                 m.ID,
		DisplayName:        m.DisplayName,
		Color:              m.Color,
		AvatarAttachmentID: m.AvatarAttachmentID,
		IsDefault:          m.IsDefault,
	}
}

func ChannelIdentityGetByID(id string) (*ChannelIdentityModel, error) {
	var item ChannelIdentityModel
	err := db.Where("id = ?", id).Limit(1).Find(&item).Error
	if err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &item, nil
}

func ChannelIdentityList(channelID string, userID string) ([]*ChannelIdentityModel, error) {
	var items []*ChannelIdentityModel
	q := db.Where("channel_id = ?", channelID).Order("sort_order asc, created_at asc")
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	err := q.Find(&items).Error
	return items, err
}

// ChannelIdentityListVisible 返回用户可见的身份列表（排除隐形身份）
func ChannelIdentityListVisible(channelID string, userID string) ([]*ChannelIdentityModel, error) {
	var items []*ChannelIdentityModel
	q := db.Where("channel_id = ? AND (is_hidden = ? OR is_hidden IS NULL)", channelID, false).
		Order("sort_order asc, created_at asc")
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	err := q.Find(&items).Error
	return items, err
}

func ChannelIdentityFindDefault(channelID string, userID string) (*ChannelIdentityModel, error) {
	var item ChannelIdentityModel
	err := db.Where("channel_id = ? AND user_id = ? AND is_default = ?", channelID, userID, true).
		Limit(1).
		Find(&item).Error
	if err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &item, nil
}

// ChannelIdentityFindHidden 查找用户在频道中的隐形默认身份
func ChannelIdentityFindHidden(channelID string, userID string) (*ChannelIdentityModel, error) {
	var item ChannelIdentityModel
	err := db.Where("channel_id = ? AND user_id = ? AND is_hidden = ?", channelID, userID, true).
		Limit(1).
		Find(&item).Error
	if err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &item, nil
}

func ChannelIdentityUpsert(item *ChannelIdentityModel) error {
	return db.Save(item).Error
}

func ChannelIdentityUpdate(id string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	return db.Model(&ChannelIdentityModel{}).Where("id = ?", id).Updates(values).Error
}

func ChannelIdentityDelete(id string) error {
	return db.Where("id = ?", id).Delete(&ChannelIdentityModel{}).Error
}

func ChannelIdentityMaxSort(channelID string, userID string) (int, error) {
	var sort int
	err := db.Model(&ChannelIdentityModel{}).
		Where("channel_id = ? AND user_id = ?", channelID, userID).
		Select("coalesce(max(sort_order), 0)").
		Scan(&sort).Error
	return sort, err
}

func ChannelIdentityEnsureSingleDefault(channelID string, userID string, identityID string) error {
	return db.Model(&ChannelIdentityModel{}).
		Where("channel_id = ? AND user_id = ? AND id <> ?", channelID, userID, identityID).
		Update("is_default", false).Error
}

func ChannelIdentityNormalizeColor(color string) string {
	if color == "" {
		return ""
	}
	color = strings.TrimSpace(strings.ToLower(color))
	if strings.HasPrefix(color, "#") {
		if len(color) == 4 || len(color) == 7 {
			return color
		}
		return ""
	}
	if len(color) == 3 || len(color) == 6 {
		return "#" + color
	}
	return ""
}

func ChannelIdentityValidateOwnership(identityID string, userID string, channelID string) (*ChannelIdentityModel, error) {
	identity, err := ChannelIdentityGetByID(identityID)
	if err != nil {
		return nil, err
	}
	if identity.UserID != userID || identity.ChannelID != channelID {
		return nil, errors.New("身份不属于该用户或频道")
	}
	return identity, nil
}

func ChannelIdentityListByIDs(channelID string, userID string, ids []string) ([]*ChannelIdentityModel, error) {
	if len(ids) == 0 {
		return []*ChannelIdentityModel{}, nil
	}
	var items []*ChannelIdentityModel
	err := db.Where("channel_id = ? AND user_id = ?", channelID, userID).
		Where("id IN ?", ids).
		Order("sort_order ASC, created_at ASC").
		Find(&items).Error
	return items, err
}

type ChannelIdentityOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color,omitempty"`
}

func ChannelIdentityOptionList(channelID string) ([]*ChannelIdentityOption, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return []*ChannelIdentityOption{}, nil
	}
	items, err := ChannelIdentityList(channelID, "")
	if err != nil {
		return nil, err
	}
	options := make([]*ChannelIdentityOption, 0, len(items))
	for _, item := range items {
		label := strings.TrimSpace(item.DisplayName)
		if label == "" {
			label = "未命名身份"
		}
		options = append(options, &ChannelIdentityOption{
			ID:    item.ID,
			Label: label,
			Color: item.Color,
		})
	}
	return options, nil
}

func ChannelIdentityOptionListActive(channelID string) ([]*ChannelIdentityOption, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return []*ChannelIdentityOption{}, nil
	}
	var identityIDs []string
	err := db.Model(&MessageModel{}).
		Distinct("sender_identity_id").
		Where("channel_id = ?", channelID).
		Where("sender_identity_id IS NOT NULL AND sender_identity_id <> ''").
		Pluck("sender_identity_id", &identityIDs).Error
	if err != nil {
		return nil, err
	}
	if len(identityIDs) == 0 {
		return []*ChannelIdentityOption{}, nil
	}
	identitySet := make(map[string]struct{}, len(identityIDs))
	for _, id := range identityIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			identitySet[trimmed] = struct{}{}
		}
	}
	if len(identitySet) == 0 {
		return []*ChannelIdentityOption{}, nil
	}
	items, err := ChannelIdentityList(channelID, "")
	if err != nil {
		return nil, err
	}
	options := make([]*ChannelIdentityOption, 0, len(identitySet))
	for _, item := range items {
		if _, ok := identitySet[item.ID]; !ok {
			continue
		}
		label := strings.TrimSpace(item.DisplayName)
		if label == "" {
			label = "未命名身份"
		}
		options = append(options, &ChannelIdentityOption{
			ID:    item.ID,
			Label: label,
			Color: item.Color,
		})
	}
	return options, nil
}

package model

import (
	"errors"
	"sealchat/protocol"
	"sort"
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
	CharacterCardID    string   `json:"characterCardId,omitempty" gorm:"size:100;index"`
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

// ChannelIdentityListAll 返回频道内所有可见身份（用于 @ 功能）
func ChannelIdentityListAll(channelID string) ([]*ChannelIdentityModel, error) {
	var items []*ChannelIdentityModel
	err := db.Where("channel_id = ? AND (is_hidden = ? OR is_hidden IS NULL)", channelID, false).
		Order("user_id asc, sort_order asc, created_at asc").
		Find(&items).Error
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

// ChannelIdentityOptionListForFilters 返回频道可用身份，并补充消息记录中存在但未在频道内定义的身份
func ChannelIdentityOptionListForFilters(channelID string) ([]*ChannelIdentityOption, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return []*ChannelIdentityOption{}, nil
	}
	options, err := ChannelIdentityOptionList(channelID)
	if err != nil {
		return nil, err
	}
	identitySet, err := fetchChannelIdentityIDsFromMessages(channelID)
	if err != nil {
		return nil, err
	}
	if len(identitySet) == 0 {
		return options, nil
	}
	existing := make(map[string]struct{}, len(options))
	for _, option := range options {
		existing[option.ID] = struct{}{}
	}
	extras, err := channelIdentityOptionsFromSet(channelID, identitySet, existing)
	if err != nil {
		return nil, err
	}
	return append(options, extras...), nil
}

func ChannelIdentityOptionListActive(channelID string) ([]*ChannelIdentityOption, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return []*ChannelIdentityOption{}, nil
	}
	identitySet, err := fetchChannelIdentityIDsFromMessages(channelID)
	if err != nil {
		return nil, err
	}
	if len(identitySet) == 0 {
		return []*ChannelIdentityOption{}, nil
	}
	items, err := ChannelIdentityList(channelID, "")
	if err != nil {
		return nil, err
	}
	options := make([]*ChannelIdentityOption, 0, len(identitySet))
	existing := make(map[string]struct{}, len(items))
	for _, item := range items {
		existing[item.ID] = struct{}{}
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
	extras, err := channelIdentityOptionsFromSet(channelID, identitySet, existing)
	if err != nil {
		return nil, err
	}
	return append(options, extras...), nil
}

func fetchChannelIdentityIDsFromMessages(channelID string) (map[string]struct{}, error) {
	var identityIDs []string
	err := db.Model(&MessageModel{}).
		Distinct("sender_identity_id").
		Where("channel_id = ?", channelID).
		Where("sender_identity_id IS NOT NULL AND sender_identity_id <> ''").
		Pluck("sender_identity_id", &identityIDs).Error
	if err != nil {
		return nil, err
	}
	identitySet := make(map[string]struct{}, len(identityIDs))
	for _, id := range identityIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			identitySet[trimmed] = struct{}{}
		}
	}
	return identitySet, nil
}

func channelIdentityOptionsFromSet(channelID string, idSet map[string]struct{}, exclude map[string]struct{}) ([]*ChannelIdentityOption, error) {
	if len(idSet) == 0 {
		return []*ChannelIdentityOption{}, nil
	}
	missing := make([]string, 0, len(idSet))
	for id := range idSet {
		if exclude != nil {
			if _, ok := exclude[id]; ok {
				continue
			}
		}
		missing = append(missing, id)
	}
	if len(missing) == 0 {
		return []*ChannelIdentityOption{}, nil
	}
	sort.Strings(missing)

	var identities []*ChannelIdentityModel
	if err := db.Where("id IN ?", missing).Find(&identities).Error; err != nil {
		return nil, err
	}
	found := make(map[string]struct{}, len(identities))
	options := make([]*ChannelIdentityOption, 0, len(missing))
	for _, identity := range identities {
		if identity == nil || strings.TrimSpace(identity.ID) == "" {
			continue
		}
		label := strings.TrimSpace(identity.DisplayName)
		if label == "" {
			label = "未命名身份"
		}
		options = append(options, &ChannelIdentityOption{
			ID:    identity.ID,
			Label: label,
			Color: identity.Color,
		})
		found[identity.ID] = struct{}{}
	}

	if len(found) == len(missing) {
		return options, nil
	}

	for _, id := range missing {
		if _, ok := found[id]; ok {
			continue
		}
		var snapshot struct {
			SenderIdentityID    string
			SenderIdentityName  string
			SenderIdentityColor string
		}
		if err := db.Model(&MessageModel{}).
			Select("sender_identity_id", "sender_identity_name", "sender_identity_color").
			Where("channel_id = ? AND sender_identity_id = ?", channelID, id).
			Order("created_at DESC").
			Limit(1).
			Scan(&snapshot).Error; err != nil {
			return nil, err
		}
		label := strings.TrimSpace(snapshot.SenderIdentityName)
		if label == "" {
			label = "未命名身份"
		}
		options = append(options, &ChannelIdentityOption{
			ID:    id,
			Label: label,
			Color: snapshot.SenderIdentityColor,
		})
	}

	return options, nil
}

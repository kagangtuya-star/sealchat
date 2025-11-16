package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelFolderModel 表示频道文件夹（系统级别，非个人）
type ChannelFolderModel struct {
	StringPKBaseModel
	Name        string `json:"name"`
	ParentID    string `json:"parentId" gorm:"size:100;index"`
	SortOrder   int    `json:"sortOrder" gorm:"index"`
	Description string `json:"description"`
	UserID      string `json:"userId" gorm:"size:100;index:idx_channel_folder_user,priority:1"`
	CreatedBy   string `json:"createdBy" gorm:"size:100"`
	UpdatedBy   string `json:"updatedBy" gorm:"size:100"`
}

func (*ChannelFolderModel) TableName() string {
	return "channel_folders"
}

// ChannelFolderMemberModel 表示文件夹与频道的关联
type ChannelFolderMemberModel struct {
	StringPKBaseModel
	FolderID  string `json:"folderId" gorm:"size:100;index:idx_channel_folder_member_folder,priority:1"`
	ChannelID string `json:"channelId" gorm:"size:100;index:idx_channel_folder_member_channel,priority:1"`
	UserID    string `json:"userId" gorm:"size:100;index:idx_channel_folder_member_user,priority:1"`
	SortOrder int    `json:"sortOrder" gorm:"index"`
}

func (*ChannelFolderMemberModel) TableName() string {
	return "channel_folder_members"
}

// ChannelFolderFavoriteModel 记录用户收藏的文件夹
type ChannelFolderFavoriteModel struct {
	StringPKBaseModel
	FolderID string `json:"folderId" gorm:"size:100;index:idx_channel_folder_favorite,priority:1"`
	UserID   string `json:"userId" gorm:"size:100;index:idx_channel_folder_favorite,priority:2"`
}

func (*ChannelFolderFavoriteModel) TableName() string {
	return "channel_folder_favorites"
}

func ChannelFolderListByUser(userID string) ([]*ChannelFolderModel, error) {
	var items []*ChannelFolderModel
	err := db.Where("user_id = ?", userID).
		Order("sort_order ASC").Order("created_at ASC").Find(&items).Error
	return items, err
}

func ChannelFolderGetByID(id string) (*ChannelFolderModel, error) {
	var folder ChannelFolderModel
	if err := db.Where("id = ?", id).Limit(1).Find(&folder).Error; err != nil {
		return nil, err
	}
	if folder.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &folder, nil
}

func ChannelFolderCreate(folder *ChannelFolderModel) error {
	if folder == nil {
		return errors.New("invalid folder payload")
	}
	return db.Create(folder).Error
}

func ChannelFolderUpdate(id string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	return db.Model(&ChannelFolderModel{}).Where("id = ?", id).Updates(values).Error
}

func ChannelFolderDelete(id string) error {
	return db.Where("id = ?", id).Delete(&ChannelFolderModel{}).Error
}

func ChannelFolderMemberListByUser(userID string) ([]*ChannelFolderMemberModel, error) {
	var items []*ChannelFolderMemberModel
	err := db.Where("user_id = ?", userID).
		Order("sort_order ASC").Order("created_at ASC").Find(&items).Error
	return items, err
}

func ChannelFolderMemberListByChannelIDs(channelIDs []string) ([]*ChannelFolderMemberModel, error) {
	if len(channelIDs) == 0 {
		return []*ChannelFolderMemberModel{}, nil
	}
	var items []*ChannelFolderMemberModel
	err := db.Where("channel_id IN ?", channelIDs).Find(&items).Error
	return items, err
}

func ChannelFolderMemberBulkInsert(records []*ChannelFolderMemberModel) error {
	if len(records) == 0 {
		return nil
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&records).Error
}

func ChannelFolderMemberDeleteByChannelIDs(userID string, channelIDs []string) error {
	if len(channelIDs) == 0 {
		return nil
	}
	return db.Where("channel_id IN ?", channelIDs).
		Where("user_id = ?", userID).
		Delete(&ChannelFolderMemberModel{}).Error
}

func ChannelFolderMemberDeleteByFolderIDs(userID string, folderIDs []string) error {
	if len(folderIDs) == 0 {
		return nil
	}
	return db.Where("folder_id IN ?", folderIDs).
		Where("user_id = ?", userID).
		Delete(&ChannelFolderMemberModel{}).Error
}

func ChannelFolderMemberDelete(userID string, folderIDs []string, channelIDs []string) error {
	if len(folderIDs) == 0 || len(channelIDs) == 0 {
		return nil
	}
	return db.Where("folder_id IN ?", folderIDs).
		Where("channel_id IN ?", channelIDs).
		Where("user_id = ?", userID).
		Delete(&ChannelFolderMemberModel{}).Error
}

func ChannelFolderFavoriteIDs(userID string) ([]string, error) {
	var items []ChannelFolderFavoriteModel
	err := db.Where("user_id = ?", userID).Find(&items).Error
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.FolderID)
	}
	return ids, nil
}

func ChannelFolderFavoriteSet(userID string, folderID string, favored bool) error {
	if favored {
		fav := &ChannelFolderFavoriteModel{FolderID: folderID, UserID: userID}
		return db.Clauses(clause.OnConflict{DoNothing: true}).Create(fav).Error
	}
	return db.Where("user_id = ? AND folder_id = ?", userID, folderID).
		Delete(&ChannelFolderFavoriteModel{}).Error
}

func ChannelFolderFavoriteDeleteByFolderIDs(userID string, folderIDs []string) error {
	if len(folderIDs) == 0 {
		return nil
	}
	return db.Where("folder_id IN ?", folderIDs).
		Where("user_id = ?", userID).
		Delete(&ChannelFolderFavoriteModel{}).Error
}

func ChannelFolderEnsureOwnership(id string, userID string) (*ChannelFolderModel, error) {
	folder, err := ChannelFolderGetByID(id)
	if err != nil {
		return nil, err
	}
	if folder.UserID != userID {
		return nil, errors.New("文件夹不存在或无权限访问")
	}
	return folder, nil
}

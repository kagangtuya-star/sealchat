package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChannelIdentityFolderModel struct {
	StringPKBaseModel
	ChannelID string `json:"channelId" gorm:"size:100;index:idx_identity_folder_channel_user,priority:1"`
	UserID    string `json:"userId" gorm:"size:100;index:idx_identity_folder_channel_user,priority:2"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder" gorm:"index"`
}

func (*ChannelIdentityFolderModel) TableName() string {
	return "channel_identity_folders"
}

type ChannelIdentityFolderMemberModel struct {
	StringPKBaseModel
	ChannelID  string `json:"channelId" gorm:"size:100;index"`
	UserID     string `json:"userId" gorm:"size:100;index"`
	FolderID   string `json:"folderId" gorm:"size:100;index:idx_identity_folder_member_folder,priority:1"`
	IdentityID string `json:"identityId" gorm:"size:100;index:idx_identity_folder_member_identity,priority:1"`
	SortOrder  int    `json:"sortOrder"`
}

func (*ChannelIdentityFolderMemberModel) TableName() string {
	return "channel_identity_folder_members"
}

type ChannelIdentityFolderFavoriteModel struct {
	StringPKBaseModel
	ChannelID string `json:"channelId" gorm:"size:100;index:idx_identity_folder_favorite_user,priority:1"`
	UserID    string `json:"userId" gorm:"size:100;index:idx_identity_folder_favorite_user,priority:2"`
	FolderID  string `json:"folderId" gorm:"size:100;index"`
}

func (*ChannelIdentityFolderFavoriteModel) TableName() string {
	return "channel_identity_folder_favorites"
}

func ChannelIdentityFolderGetByID(id string) (*ChannelIdentityFolderModel, error) {
	var item ChannelIdentityFolderModel
	if err := db.Where("id = ?", id).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &item, nil
}

func ChannelIdentityFolderList(channelID string, userID string) ([]*ChannelIdentityFolderModel, error) {
	var items []*ChannelIdentityFolderModel
	err := db.Where("channel_id = ? AND user_id = ?", channelID, userID).
		Order("sort_order ASC, created_at ASC").
		Find(&items).Error
	return items, err
}

func ChannelIdentityFolderListByIDs(channelID string, userID string, ids []string) ([]*ChannelIdentityFolderModel, error) {
	if len(ids) == 0 {
		return []*ChannelIdentityFolderModel{}, nil
	}
	var items []*ChannelIdentityFolderModel
	err := db.Where("channel_id = ? AND user_id = ?", channelID, userID).
		Where("id IN ?", ids).
		Find(&items).Error
	return items, err
}

func ChannelIdentityFolderMaxSort(channelID string, userID string) (int, error) {
	var sort int
	err := db.Model(&ChannelIdentityFolderModel{}).
		Where("channel_id = ? AND user_id = ?", channelID, userID).
		Select("coalesce(max(sort_order), 0)").
		Scan(&sort).Error
	return sort, err
}

func ChannelIdentityFolderDelete(id string) error {
	return db.Where("id = ?", id).Delete(&ChannelIdentityFolderModel{}).Error
}

func ChannelIdentityFolderUpdate(id string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	return db.Model(&ChannelIdentityFolderModel{}).Where("id = ?", id).Updates(values).Error
}

func ChannelIdentityFolderUpsert(folder *ChannelIdentityFolderModel) error {
	return db.Save(folder).Error
}

func ChannelIdentityFolderMemberList(channelID string, userID string) ([]*ChannelIdentityFolderMemberModel, error) {
	var items []*ChannelIdentityFolderMemberModel
	err := db.Where("channel_id = ? AND user_id = ?", channelID, userID).
		Order("sort_order ASC, created_at ASC").
		Find(&items).Error
	return items, err
}

func ChannelIdentityFolderMemberListByIdentityIDs(identityIDs []string) ([]*ChannelIdentityFolderMemberModel, error) {
	if len(identityIDs) == 0 {
		return []*ChannelIdentityFolderMemberModel{}, nil
	}
	var items []*ChannelIdentityFolderMemberModel
	err := db.Where("identity_id IN ?", identityIDs).
		Order("sort_order ASC, created_at ASC").
		Find(&items).Error
	return items, err
}

func ChannelIdentityFolderMemberBulkInsert(records []*ChannelIdentityFolderMemberModel) error {
	if len(records) == 0 {
		return nil
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&records).Error
}

func ChannelIdentityFolderMemberDeleteByIdentityIDs(identityIDs []string) error {
	if len(identityIDs) == 0 {
		return nil
	}
	return db.Where("identity_id IN ?", identityIDs).
		Delete(&ChannelIdentityFolderMemberModel{}).Error
}

func ChannelIdentityFolderMemberDeleteByIdentityAndFolder(identityIDs []string, folderIDs []string) error {
	if len(identityIDs) == 0 || len(folderIDs) == 0 {
		return nil
	}
	return db.Where("identity_id IN ?", identityIDs).
		Where("folder_id IN ?", folderIDs).
		Delete(&ChannelIdentityFolderMemberModel{}).Error
}

func ChannelIdentityFolderMemberDeleteByFolderIDs(folderIDs []string) error {
	if len(folderIDs) == 0 {
		return nil
	}
	return db.Where("folder_id IN ?", folderIDs).
		Delete(&ChannelIdentityFolderMemberModel{}).Error
}

func ChannelIdentityFolderFavoriteIDs(channelID string, userID string) ([]string, error) {
	var favorites []ChannelIdentityFolderFavoriteModel
	err := db.Where("channel_id = ? AND user_id = ?", channelID, userID).
		Find(&favorites).Error
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(favorites))
	for _, fav := range favorites {
		ids = append(ids, fav.FolderID)
	}
	return ids, nil
}

func ChannelIdentityFolderFavoriteSet(channelID string, userID string, folderID string, favored bool) error {
	if favored {
		fav := &ChannelIdentityFolderFavoriteModel{
			ChannelID: channelID,
			UserID:    userID,
			FolderID:  folderID,
		}
		return db.Clauses(clause.OnConflict{DoNothing: true}).Create(fav).Error
	}
	return db.Where("channel_id = ? AND user_id = ? AND folder_id = ?", channelID, userID, folderID).
		Delete(&ChannelIdentityFolderFavoriteModel{}).Error
}

func ChannelIdentityFolderFavoriteDeleteByFolderIDs(folderIDs []string) error {
	if len(folderIDs) == 0 {
		return nil
	}
	return db.Where("folder_id IN ?", folderIDs).
		Delete(&ChannelIdentityFolderFavoriteModel{}).Error
}

func ChannelIdentityFolderEnsureOwnership(folderID string, userID string, channelID string) (*ChannelIdentityFolderModel, error) {
	folder, err := ChannelIdentityFolderGetByID(folderID)
	if err != nil {
		return nil, err
	}
	if folder.UserID != userID || folder.ChannelID != channelID {
		return nil, errors.New("文件夹不存在或无权限访问")
	}
	return folder, nil
}

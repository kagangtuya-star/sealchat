package model

import (
	"errors"

	"gorm.io/gorm"

	"sealchat/utils"
)

type OwnerType string

const (
	OwnerTypeUser    OwnerType = "user"
	OwnerTypeChannel OwnerType = "channel"
)

const (
	CollectionTypeEmojiFavorites = "emoji_favorites"
	CollectionTypeEmojiReactions = "emoji_reactions"
)

type GalleryCollection struct {
	StringPKBaseModel
	OwnerType      OwnerType `json:"ownerType" gorm:"type:varchar(16);index:idx_gallery_owner"`
	OwnerID        string    `json:"ownerId" gorm:"index:idx_gallery_owner"`
	CollectionType *string   `json:"collectionType,omitempty" gorm:"type:varchar(32)"`
	Name           string    `json:"name"`
	Order          int       `json:"order"`
	QuotaUsed      int64     `json:"quotaUsed"`
	CreatedBy      string    `json:"createdBy"`
	UpdatedBy      string    `json:"updatedBy"`
}

func (*GalleryCollection) TableName() string { return "gallery_collections" }

type GalleryItem struct {
	StringPKBaseModel
	CollectionID string `json:"collectionId" gorm:"index"`
	AttachmentID string `json:"attachmentId"`
	ThumbURL     string `json:"thumbUrl"`
	Remark       string `json:"remark" gorm:"index"`
	Tags         string `json:"tags"`
	Order        int    `json:"order"`
	CreatedBy    string `json:"createdBy"`
	Size         int64  `json:"size"`
}

func (*GalleryItem) TableName() string { return "gallery_items" }

func CreateGalleryCollection(ownerType OwnerType, ownerID, name, createdBy string, order int) (*GalleryCollection, error) {
	if ownerType != OwnerTypeUser && ownerType != OwnerTypeChannel {
		return nil, errors.New("invalid owner type")
	}
	col := &GalleryCollection{
		OwnerType: ownerType,
		OwnerID:   ownerID,
		Name:      name,
		Order:     order,
		CreatedBy: createdBy,
		UpdatedBy: createdBy,
	}
	col.StringPKBaseModel.Init()
	return col, db.Create(col).Error
}

func UpdateGalleryCollection(col *GalleryCollection, fields map[string]interface{}) error {
	return db.Model(col).Updates(fields).Error
}

func DeleteGalleryCollection(id string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Delete(&GalleryItem{}, "collection_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(&GalleryCollection{}, "id = ?", id).Error
	})
}

func ListGalleryCollections(ownerType OwnerType, ownerID string) ([]*GalleryCollection, error) {
	var cols []*GalleryCollection
	err := db.Where("owner_type = ? AND owner_id = ?", ownerType, ownerID).Order("`order`").Find(&cols).Error
	return cols, err
}

func GetGalleryCollection(id string) (*GalleryCollection, error) {
	var col GalleryCollection
	if err := db.Where("id = ?", id).First(&col).Error; err != nil {
		return nil, err
	}
	return &col, nil
}

func GetGalleryItem(id string) (*GalleryItem, error) {
	var item GalleryItem
	if err := db.Where("id = ?", id).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func ListGalleryItemsByIDs(ids []string) ([]*GalleryItem, error) {
	if len(ids) == 0 {
		return []*GalleryItem{}, nil
	}
	var items []*GalleryItem
	if err := db.Where("id IN ?", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func ListGalleryItems(collectionID string, keyword string, page, pageSize int) ([]*GalleryItem, int64, error) {
	return utils.QueryPaginatedList(db, page, pageSize, &GalleryItem{}, func(q *gorm.DB) *gorm.DB {
		q = q.Where("collection_id = ?", collectionID)
		if keyword != "" {
			q = q.Where("remark LIKE ?", "%"+keyword+"%")
		}
		return q.Order("`order`, created_at DESC")
	})
}

func SearchGalleryItems(ownerType OwnerType, ownerID, keyword string, limit int) ([]*GalleryItem, error) {
	var items []*GalleryItem
	query := db.Table((&GalleryItem{}).TableName()).Select("gallery_items.*")
	query = query.Joins("JOIN gallery_collections ON gallery_collections.id = gallery_items.collection_id")
	query = query.Where("gallery_collections.owner_type = ? AND gallery_collections.owner_id = ?", ownerType, ownerID)
	if keyword != "" {
		query = query.Where("gallery_items.remark LIKE ?", "%"+keyword+"%")
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Order("gallery_items.`order`, gallery_items.created_at DESC").Find(&items).Error
	return items, err
}

func CreateGalleryItems(items []*GalleryItem) error {
	for _, item := range items {
		if item.ID == "" {
			item.StringPKBaseModel.Init()
		}
	}
	return db.Create(items).Error
}

func DeleteGalleryItems(ids []string) error {
	return db.Unscoped().Delete(&GalleryItem{}, "id IN ?", ids).Error
}

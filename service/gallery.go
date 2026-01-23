package service

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
)

var (
	galleryRemarkPattern    = regexp.MustCompile(`^[\p{L}\p{N}_]{1,64}$`)
	ErrGalleryRemarkInvalid = errors.New("备注仅支持字母、数字和下划线，长度不超过64")
	ErrGalleryPermission    = errors.New("缺少快捷表情资源操作权限")
	ErrGalleryQuotaExceeded = errors.New("快捷表情容量不足")
)

const defaultCollectionName = "默认分类"
const emojiCollectionName = "表情收藏"
const emojiReactionCollectionName = "表情反应"

func GalleryValidateRemark(remark string) bool {
	if remark == "" {
		return false
	}
	return galleryRemarkPattern.MatchString(remark)
}

func GalleryEnsureDefaultCollection(ownerType model.OwnerType, ownerID, creatorID string) (*model.GalleryCollection, error) {
	db := model.GetDB()
	var col model.GalleryCollection
	err := db.Where("owner_type = ? AND owner_id = ? AND collection_type IS NULL", ownerType, ownerID).
		Order("`order`, created_at").
		Limit(1).
		Take(&col).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		col = model.GalleryCollection{
			OwnerType: ownerType,
			OwnerID:   ownerID,
			Name:      defaultCollectionName,
			Order:     0,
			CreatedBy: creatorID,
			UpdatedBy: creatorID,
		}
		col.StringPKBaseModel.Init()
		if err = db.Create(&col).Error; err != nil {
			return nil, err
		}
		return &col, nil
	}
	if err != nil {
		return nil, err
	}
	return &col, nil
}

func GalleryListCollections(ownerType model.OwnerType, ownerID, creatorID string) ([]*model.GalleryCollection, error) {
	cols, err := model.ListGalleryCollections(ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		col, err := GalleryEnsureDefaultCollection(ownerType, ownerID, creatorID)
		if err != nil {
			return nil, err
		}
		cols = []*model.GalleryCollection{col}
	}
	if ownerType == model.OwnerTypeUser {
		if _, err := GalleryEnsureEmojiReactionCollection(ownerType, ownerID, creatorID); err != nil {
			return nil, err
		}
		cols, err = model.ListGalleryCollections(ownerType, ownerID)
		if err != nil {
			return nil, err
		}
	}
	return cols, nil
}

func GalleryEnsureCanRead(userID string, ownerType model.OwnerType, ownerID string) bool {
	switch ownerType {
	case model.OwnerTypeUser:
		return ownerID == userID
	case model.OwnerTypeChannel:
		return pm.Can(userID, ownerID, pm.PermFuncChannelRead)
	default:
		return false
	}
}

func GalleryEnsureCanManage(userID string, ownerType model.OwnerType, ownerID string) bool {
	switch ownerType {
	case model.OwnerTypeUser:
		return ownerID == userID
	case model.OwnerTypeChannel:
		return pm.CanWithChannelRole(userID, ownerID, pm.PermFuncChannelManageGallery)
	default:
		return false
	}
}

func GalleryUserUsageBytes(userID string) (int64, error) {
	var total int64
	err := model.GetDB().Model(&model.GalleryItem{}).
		Where("created_by = ?", userID).
		Select("COALESCE(SUM(size),0)").
		Scan(&total).Error
	return total, err
}

func GalleryEnsureQuota(userID string, additional int64, limitBytes int64) error {
	used, err := GalleryUserUsageBytes(userID)
	if err != nil {
		return err
	}
	if used+additional > limitBytes {
		return ErrGalleryQuotaExceeded
	}
	return nil
}

func GalleryUpdateCollectionQuota(collectionID string) error {
	var total int64
	db := model.GetDB()
	if err := db.Model(&model.GalleryItem{}).
		Where("collection_id = ?", collectionID).
		Select("COALESCE(SUM(size),0)").
		Scan(&total).Error; err != nil {
		return err
	}

	return db.Model(&model.GalleryCollection{}).
		Where("id = ?", collectionID).
		Update("quota_used", total).Error
}

func GalleryBatchUpdateCollectionQuota(collectionIDs []string) error {
	if len(collectionIDs) == 0 {
		return nil
	}
	unique := map[string]struct{}{}
	for _, id := range collectionIDs {
		if id == "" {
			continue
		}
		if _, ok := unique[id]; ok {
			continue
		}
		unique[id] = struct{}{}
		if err := GalleryUpdateCollectionQuota(id); err != nil {
			return err
		}
	}
	return nil
}

func GalleryThumbFilename(itemID string, ext string) string {
	return filepath.Join("./data/gallery/thumbs", itemID+ext)
}

// GalleryIsSystemCollection 判断是否系统分类
func GalleryIsSystemCollection(col *model.GalleryCollection) bool {
	return col != nil && col.CollectionType != nil && *col.CollectionType != ""
}

// GalleryIsEmojiCollection 判断是否表情收藏分类
func GalleryIsEmojiCollection(col *model.GalleryCollection) bool {
	return col != nil && col.CollectionType != nil && *col.CollectionType == model.CollectionTypeEmojiFavorites
}

// GalleryIsEmojiReactionCollection 判断是否表情反应分类
func GalleryIsEmojiReactionCollection(col *model.GalleryCollection) bool {
	return col != nil && col.CollectionType != nil && *col.CollectionType == model.CollectionTypeEmojiReactions
}

// GalleryEnsureEmojiCollection 确保表情收藏分类存在
func GalleryEnsureEmojiCollection(ownerType model.OwnerType, ownerID, creatorID string) (*model.GalleryCollection, error) {
	if ownerType != model.OwnerTypeUser {
		return nil, errors.New("emoji favorites only supported for user")
	}
	db := model.GetDB()
	ct := model.CollectionTypeEmojiFavorites
	var col model.GalleryCollection
	err := db.
		Where("owner_type = ? AND owner_id = ? AND collection_type = ?", ownerType, ownerID, ct).
		Attrs(model.GalleryCollection{
			OwnerType:      ownerType,
			OwnerID:        ownerID,
			CollectionType: &ct,
			Name:           emojiCollectionName,
			Order:          -1,
			CreatedBy:      creatorID,
			UpdatedBy:      creatorID,
		}).
		FirstOrCreate(&col).Error
	if err != nil {
		return nil, err
	}
	if col.ID == "" {
		col.StringPKBaseModel.Init()
	}
	return &col, nil
}

// GalleryEnsureEmojiReactionCollection 确保表情反应分类存在
func GalleryEnsureEmojiReactionCollection(ownerType model.OwnerType, ownerID, creatorID string) (*model.GalleryCollection, error) {
	if ownerType != model.OwnerTypeUser {
		return nil, errors.New("emoji reactions only supported for user")
	}
	db := model.GetDB()
	ct := model.CollectionTypeEmojiReactions
	var col model.GalleryCollection
	err := db.
		Where("owner_type = ? AND owner_id = ? AND collection_type = ?", ownerType, ownerID, ct).
		Attrs(model.GalleryCollection{
			OwnerType:      ownerType,
			OwnerID:        ownerID,
			CollectionType: &ct,
			Name:           emojiReactionCollectionName,
			Order:          -2,
			CreatedBy:      creatorID,
			UpdatedBy:      creatorID,
		}).
		FirstOrCreate(&col).Error
	if err != nil {
		return nil, err
	}
	if col.ID == "" {
		col.StringPKBaseModel.Init()
	}
	return &col, nil
}

// GalleryListEmojiFavorites 列出用户表情收藏
func GalleryListEmojiFavorites(userID string) ([]*model.GalleryItem, error) {
	col, err := GalleryEnsureEmojiCollection(model.OwnerTypeUser, userID, userID)
	if err != nil {
		return nil, err
	}
	items, _, err := model.ListGalleryItems(col.ID, "", 1, -1)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// GalleryAddEmojiFavorite 添加表情到收藏（复制附件记录）
func GalleryAddEmojiFavorite(userID, attachmentID, remark string) (*model.GalleryItem, error) {
	if strings.TrimSpace(attachmentID) == "" {
		return nil, errors.New("附件ID不能为空")
	}
	col, err := GalleryEnsureEmojiCollection(model.OwnerTypeUser, userID, userID)
	if err != nil {
		return nil, err
	}

	var srcAtt model.AttachmentModel
	if err := model.GetDB().Where("id = ?", attachmentID).First(&srcAtt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("附件不存在或已删除")
		}
		return nil, err
	}

	// 复制附件记录（共享存储文件，新ID）
	newAtt := model.AttachmentModel{
		Hash:        srcAtt.Hash,
		Filename:    srcAtt.Filename,
		Size:        srcAtt.Size,
		MimeType:    srcAtt.MimeType,
		IsAnimated:  srcAtt.IsAnimated,
		UserID:      userID,
		StorageType: srcAtt.StorageType,
		ObjectKey:   srcAtt.ObjectKey,
		ExternalURL: srcAtt.ExternalURL,
		RootID:      col.ID,
		RootIDType:  "gallery_collection",
		IsTemp:      false,
	}
	if _, item := model.AttachmentCreate(&newAtt); item == nil {
		return nil, errors.New("创建附件记录失败")
	}

	normalized := strings.TrimSpace(remark)
	if normalized != "" && !GalleryValidateRemark(normalized) {
		normalized = NormalizeRemark(normalized, srcAtt.Filename)
	}
	if normalized == "" {
		normalized = NormalizeRemark("", srcAtt.Filename)
	}

	item := &model.GalleryItem{
		CollectionID: col.ID,
		AttachmentID: newAtt.ID,
		ThumbURL:     "",
		Remark:       normalized,
		Order:        int(time.Now().Unix()),
		CreatedBy:    userID,
		Size:         newAtt.Size,
	}
	item.StringPKBaseModel.Init()
	if err := model.GetDB().Create(item).Error; err != nil {
		return nil, err
	}
	_ = GalleryUpdateCollectionQuota(col.ID)
	return item, nil
}

// GalleryAddEmojiReaction 添加表情到表情反应分类（复制附件记录）
func GalleryAddEmojiReaction(userID, attachmentID, remark string) (*model.GalleryItem, error) {
	if strings.TrimSpace(attachmentID) == "" {
		return nil, errors.New("附件ID不能为空")
	}
	col, err := GalleryEnsureEmojiReactionCollection(model.OwnerTypeUser, userID, userID)
	if err != nil {
		return nil, err
	}

	var srcAtt model.AttachmentModel
	if err := model.GetDB().Where("id = ?", attachmentID).First(&srcAtt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("附件不存在或已删除")
		}
		return nil, err
	}

	newAtt := model.AttachmentModel{
		Hash:        srcAtt.Hash,
		Filename:    srcAtt.Filename,
		Size:        srcAtt.Size,
		MimeType:    srcAtt.MimeType,
		IsAnimated:  srcAtt.IsAnimated,
		UserID:      userID,
		StorageType: srcAtt.StorageType,
		ObjectKey:   srcAtt.ObjectKey,
		ExternalURL: srcAtt.ExternalURL,
		RootID:      col.ID,
		RootIDType:  "gallery_collection",
		IsTemp:      false,
	}
	if _, item := model.AttachmentCreate(&newAtt); item == nil {
		return nil, errors.New("创建附件记录失败")
	}

	normalized := strings.TrimSpace(remark)
	if normalized != "" && !GalleryValidateRemark(normalized) {
		normalized = NormalizeRemark(normalized, srcAtt.Filename)
	}
	if normalized == "" {
		normalized = NormalizeRemark("", srcAtt.Filename)
	}

	item := &model.GalleryItem{
		CollectionID: col.ID,
		AttachmentID: newAtt.ID,
		ThumbURL:     "",
		Remark:       normalized,
		Order:        int(time.Now().Unix()),
		CreatedBy:    userID,
		Size:         newAtt.Size,
	}
	item.StringPKBaseModel.Init()
	if err := model.GetDB().Create(item).Error; err != nil {
		return nil, err
	}
	_ = GalleryUpdateCollectionQuota(col.ID)
	return item, nil
}

// GalleryUpdateEmojiFavoriteRemark 更新表情备注
func GalleryUpdateEmojiFavoriteRemark(userID, itemID, remark string) (*model.GalleryItem, error) {
	item, err := model.GetGalleryItem(itemID)
	if err != nil {
		return nil, err
	}
	col, err := model.GetGalleryCollection(item.CollectionID)
	if err != nil {
		return nil, err
	}
	if col.OwnerType != model.OwnerTypeUser || col.OwnerID != userID || !GalleryIsEmojiCollection(col) {
		return nil, ErrGalleryPermission
	}
	normalized := strings.TrimSpace(remark)
	if normalized != "" && !GalleryValidateRemark(normalized) {
		return nil, ErrGalleryRemarkInvalid
	}
	if err := model.GetDB().Model(item).Updates(map[string]interface{}{"remark": normalized}).Error; err != nil {
		return nil, err
	}
	item.Remark = normalized
	return item, nil
}

// GalleryDeleteEmojiFavorites 删除表情收藏（只断开链接，不删附件）
func GalleryDeleteEmojiFavorites(userID string, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	col, err := GalleryEnsureEmojiCollection(model.OwnerTypeUser, userID, userID)
	if err != nil {
		return 0, err
	}

	var items []*model.GalleryItem
	if err := model.GetDB().Where("id IN ?", ids).Find(&items).Error; err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	for _, item := range items {
		if item.CollectionID != col.ID {
			return 0, ErrGalleryPermission
		}
	}
	if err := model.DeleteGalleryItems(ids); err != nil {
		return 0, err
	}
	_ = GalleryUpdateCollectionQuota(col.ID)
	return int64(len(items)), nil
}

func NormalizeRemark(input, filename string) string {
	remark := strings.TrimSpace(input)
	if remark == "" {
		remark = strings.TrimSuffix(filename, filepath.Ext(filename))
	}

	var builder strings.Builder
	builder.Grow(len(remark))
	lastUnderscore := false
	for _, r := range remark {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastUnderscore = false
		case r == '_':
			if !lastUnderscore {
				builder.WriteRune('_')
				lastUnderscore = true
			}
		default:
			if !lastUnderscore {
				builder.WriteRune('_')
				lastUnderscore = true
			}
		}
	}

	out := strings.Trim(builder.String(), "_")
	if out == "" {
		out = "img"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

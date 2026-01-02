package api

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/samber/lo"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

const galleryThumbMaxSize = 256 * 1024

var supportedThumbMime = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/jpg":  ".jpg",
	"image/gif":  ".gif",
}

type galleryUploadItem struct {
	AttachmentID string `json:"attachmentId"`
	ThumbData    string `json:"thumbData"`
	Remark       string `json:"remark"`
	Order        int    `json:"order"`
}

type galleryUploadRequest struct {
	CollectionID string              `json:"collectionId"`
	Items        []galleryUploadItem `json:"items"`
}

type galleryItemUpdateRequest struct {
	Remark       *string `json:"remark"`
	CollectionID *string `json:"collectionId"`
	Order        *int    `json:"order"`
}

type gallerySearchResponse struct {
	Items       []*model.GalleryItem                `json:"items"`
	Collections map[string]*model.GalleryCollection `json:"collections"`
}

func GalleryCollectionsList(c *fiber.Ctx) error {
	user := getCurUser(c)
	cols, err := service.GalleryListCollections(model.OwnerTypeUser, user.ID, user.ID)
	if err != nil {
		return wrapError(c, err, "获取资源分类失败")
	}
	return c.JSON(fiber.Map{"items": cols})
}

type createCollectionRequest struct {
	Name  string `json:"name"`
	Order int    `json:"order"`
}

func GalleryCollectionCreate(c *fiber.Ctx) error {
	user := getCurUser(c)
	var req createCollectionRequest
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求参数无效")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "分类名称不能为空")
	}
	col, err := model.CreateGalleryCollection(model.OwnerTypeUser, user.ID, name, user.ID, req.Order)
	if err != nil {
		return wrapError(c, err, "创建分类失败")
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": col})
}

func GalleryCollectionUpdate(c *fiber.Ctx) error {
	colID := c.Params("id")
	if colID == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "分类ID不能为空")
	}
	user := getCurUser(c)
	col, err := model.GetGalleryCollection(colID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "分类不存在")
		}
		return wrapError(c, err, "读取分类失败")
	}
	if col.OwnerType != model.OwnerTypeUser || col.OwnerID != user.ID {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "无法操作他人的分类")
	}
	var req createCollectionRequest
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求参数错误")
	}
	updates := map[string]interface{}{}
	name := strings.TrimSpace(req.Name)
	if name != "" {
		updates["name"] = name
		col.Name = name
	}
	updates["order"] = req.Order
	col.Order = req.Order
	updates["updated_by"] = user.ID
	col.UpdatedBy = user.ID
	if err := model.UpdateGalleryCollection(col, updates); err != nil {
		return wrapError(c, err, "更新分类失败")
	}
	return c.JSON(fiber.Map{"item": col})
}

func GalleryCollectionDelete(c *fiber.Ctx) error {
	colID := c.Params("id")
	if colID == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "分类ID不能为空")
	}
	user := getCurUser(c)
	col, err := model.GetGalleryCollection(colID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "分类不存在")
		}
		return wrapError(c, err, "读取分类失败")
	}
	if col.OwnerType != model.OwnerTypeUser || col.OwnerID != user.ID {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "无法删除他人的分类")
	}

	var items []*model.GalleryItem
	if err := model.GetDB().Where("collection_id = ?", colID).Find(&items).Error; err != nil {
		return wrapError(c, err, "读取分类资源失败")
	}
	if err := model.DeleteGalleryCollection(colID); err != nil {
		return wrapError(c, err, "删除分类失败")
	}
	for _, item := range items {
		removeGalleryThumbFile(item.ThumbURL)
	}
	return c.JSON(fiber.Map{"message": "删除成功"})
}

func GalleryItemsList(c *fiber.Ctx) error {
	collectionID := c.Query("collectionId")
	if collectionID == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少分类ID")
	}
	col, err := model.GetGalleryCollection(collectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "分类不存在")
		}
		return wrapError(c, err, "读取分类失败")
	}
	user := getCurUser(c)
	if col.OwnerType != model.OwnerTypeUser || col.OwnerID != user.ID {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "无法访问他人的分类")
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	return utils.APIPaginatedList(c, func(page, pageSize int) ([]*model.GalleryItem, int64, error) {
		return model.ListGalleryItems(collectionID, keyword, page, pageSize)
	})
}

func GalleryItemsUpload(c *fiber.Ctx) error {
	user := getCurUser(c)
	var req galleryUploadRequest
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体格式错误")
	}
	if req.CollectionID == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少分类ID")
	}
	col, err := model.GetGalleryCollection(req.CollectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "分类不存在")
		}
		return wrapError(c, err, "读取分类失败")
	}
	if col.OwnerType != model.OwnerTypeUser || col.OwnerID != user.ID {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "无法向他人分类上传")
	}
	if len(req.Items) == 0 {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "上传列表不能为空")
	}

	attachmentIDs := lo.Map(req.Items, func(item galleryUploadItem, _ int) string { return item.AttachmentID })
	var attachments []model.AttachmentModel
	if err := model.GetDB().Where("id IN ?", attachmentIDs).Find(&attachments).Error; err != nil {
		return wrapError(c, err, "读取附件失败")
	}
	attMap := lo.Associate(attachments, func(att model.AttachmentModel) (string, model.AttachmentModel) {
		return att.ID, att
	})

	uniqueAttachmentIDs := lo.Uniq(attachmentIDs)
	existingAttachments := map[string]struct{}{}
	if len(uniqueAttachmentIDs) > 0 {
		var existingItems []*model.GalleryItem
		if err := model.GetDB().
			Where("collection_id = ? AND attachment_id IN ?", col.ID, uniqueAttachmentIDs).
			Find(&existingItems).Error; err != nil {
			return wrapError(c, err, "校验重复资源失败")
		}
		for _, item := range existingItems {
			existingAttachments[item.AttachmentID] = struct{}{}
		}
	}

	var totalSize int64
	items := make([]*model.GalleryItem, 0, len(req.Items))
	requestSeen := map[string]struct{}{}
	for _, payload := range req.Items {
		att, ok := attMap[payload.AttachmentID]
		if !ok {
			return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "附件不存在或已删除")
		}
		if att.UserID != user.ID {
			return wrapErrorStatus(c, fiber.StatusForbidden, nil, "无法使用他人的附件")
		}
		if _, dup := requestSeen[att.ID]; dup {
			continue
		}
		if _, exists := existingAttachments[att.ID]; exists {
			continue
		}
		requestSeen[att.ID] = struct{}{}

		remark := strings.TrimSpace(payload.Remark)
		if !service.GalleryValidateRemark(remark) {
			remark = service.NormalizeRemark(remark, att.Filename)
		}
		thumbURL, err := saveGalleryThumb(user.ID, payload.AttachmentID, payload.ThumbData)
		if err != nil {
			return wrapError(c, err, "保存缩略图失败")
		}
		items = append(items, &model.GalleryItem{
			CollectionID: col.ID,
			AttachmentID: att.ID,
			ThumbURL:     thumbURL,
			Remark:       remark,
			Order:        payload.Order,
			CreatedBy:    user.ID,
			Size:         att.Size,
		})
		totalSize += att.Size
	}

	if len(items) == 0 {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "所选资源已存在，无需重复上传")
	}

	limitBytes := int64(appConfig.GalleryQuotaMB) * 1024 * 1024
	if limitBytes > 0 {
		if err := service.GalleryEnsureQuota(user.ID, totalSize, limitBytes); err != nil {
			if errors.Is(err, service.ErrGalleryQuotaExceeded) {
				return wrapErrorStatus(c, fiber.StatusForbidden, err, "已超过图库容量限制")
			}
			return wrapError(c, err, "校验容量失败")
		}
	}

	if err := model.CreateGalleryItems(items); err != nil {
		for _, item := range items {
			removeGalleryThumbFile(item.ThumbURL)
		}
		return wrapError(c, err, "保存资源失败")
	}
	if err := service.GalleryUpdateCollectionQuota(col.ID); err != nil {
		return wrapError(c, err, "更新容量信息失败")
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{"items": items})
}

func GalleryItemUpdate(c *fiber.Ctx) error {
	itemID := c.Params("id")
	if itemID == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少资源ID")
	}
	var req galleryItemUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体格式错误")
	}
	item, err := model.GetGalleryItem(itemID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "资源不存在")
		}
		return wrapError(c, err, "读取资源失败")
	}
	col, err := model.GetGalleryCollection(item.CollectionID)
	if err != nil {
		return wrapError(c, err, "读取分类失败")
	}
	user := getCurUser(c)
	if col.OwnerType != model.OwnerTypeUser || col.OwnerID != user.ID {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "无法编辑他人的分类资源")
	}

	updates := map[string]interface{}{}
	if req.Remark != nil {
		remark := strings.TrimSpace(*req.Remark)
		if !service.GalleryValidateRemark(remark) {
			remark = service.NormalizeRemark(remark, "")
		}
		updates["remark"] = remark
		item.Remark = remark
	}
	if req.Order != nil {
		updates["order"] = *req.Order
		item.Order = *req.Order
	}
	if req.CollectionID != nil && *req.CollectionID != item.CollectionID {
		targetCol, err := model.GetGalleryCollection(*req.CollectionID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return wrapErrorStatus(c, fiber.StatusNotFound, err, "目标分类不存在")
			}
			return wrapError(c, err, "读取目标分类失败")
		}
		if targetCol.OwnerType != model.OwnerTypeUser || targetCol.OwnerID != user.ID {
			return wrapErrorStatus(c, fiber.StatusForbidden, nil, "无法移动到他人分类")
		}
		updates["collection_id"] = targetCol.ID
		item.CollectionID = targetCol.ID
	}
	if len(updates) == 0 {
		return c.JSON(fiber.Map{"item": item})
	}
	if err := model.GetDB().Model(item).Updates(updates).Error; err != nil {
		return wrapError(c, err, "更新资源失败")
	}
	if updates["collection_id"] != nil {
		_ = service.GalleryUpdateCollectionQuota(col.ID)
		_ = service.GalleryUpdateCollectionQuota(item.CollectionID)
	}
	return c.JSON(fiber.Map{"item": item})
}

func GalleryItemsDelete(c *fiber.Ctx) error {
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体格式错误")
	}
	if len(payload.IDs) == 0 {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "ID 列表不能为空")
	}

	var items []*model.GalleryItem
	if err := model.GetDB().Where("id IN ?", payload.IDs).Find(&items).Error; err != nil {
		return wrapError(c, err, "读取资源失败")
	}
	if len(items) == 0 {
		return c.JSON(fiber.Map{"message": "已删除"})
	}

	user := getCurUser(c)
	collectionIDs := make([]string, 0)
	for _, item := range items {
		col, err := model.GetGalleryCollection(item.CollectionID)
		if err != nil {
			return wrapError(c, err, "读取分类失败")
		}
		if col.OwnerType != model.OwnerTypeUser || col.OwnerID != user.ID {
			return wrapErrorStatus(c, fiber.StatusForbidden, nil, "无法删除他人资源")
		}
		collectionIDs = append(collectionIDs, col.ID)
	}

	if err := model.DeleteGalleryItems(payload.IDs); err != nil {
		return wrapError(c, err, "删除资源失败")
	}
	for _, item := range items {
		removeGalleryThumbFile(item.ThumbURL)
	}
	_ = service.GalleryBatchUpdateCollectionQuota(collectionIDs)

	return c.JSON(fiber.Map{"message": "删除成功"})
}

func GallerySearch(c *fiber.Ctx) error {
	keyword := strings.TrimSpace(c.Query("keyword"))
	ownerID := strings.TrimSpace(c.Query("ownerId"))
	ownerTypeStr := strings.TrimSpace(c.Query("ownerType"))
	user := getCurUser(c)

	ownerType := model.OwnerType(ownerTypeStr)
	switch ownerType {
	case "":
		ownerType = model.OwnerTypeUser
	case model.OwnerTypeUser, model.OwnerTypeChannel:
		// 合法类型
	default:
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "不支持的资源类型")
	}

	if ownerType == model.OwnerTypeUser {
		if ownerID == "" || ownerID != user.ID {
			ownerID = user.ID
		}
	} else {
		if ownerID == "" {
			return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少频道ID")
		}
		if !service.GalleryEnsureCanRead(user.ID, ownerType, ownerID) {
			return wrapErrorStatus(c, fiber.StatusForbidden, nil, "没有权限读取该资源")
		}
	}
	results := make([]*model.GalleryItem, 0)
	collections := map[string]*model.GalleryCollection{}

	items, err := model.SearchGalleryItems(ownerType, ownerID, keyword, 50)
	if err != nil {
		return wrapError(c, err, "搜索表情失败")
	}
	results = append(results, items...)

	if len(results) > 0 {
		ids := lo.Uniq(lo.Map(results, func(item *model.GalleryItem, _ int) string { return item.CollectionID }))
		var cols []*model.GalleryCollection
		if err := model.GetDB().Where("id IN ?", ids).Find(&cols).Error; err != nil {
			return wrapError(c, err, "读取分类信息失败")
		}
		for _, col := range cols {
			collections[col.ID] = col
		}
	}

	return c.JSON(gallerySearchResponse{Items: results, Collections: collections})
}

func saveGalleryThumb(userID, attachmentID, dataURI string) (string, error) {
	if dataURI == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "缩略图不能为空")
	}
	parts := strings.SplitN(dataURI, ",", 2)
	if len(parts) != 2 {
		return "", fiber.NewError(fiber.StatusBadRequest, "缩略图格式错误")
	}
	meta := parts[0]
	raw := parts[1]
	if !strings.HasPrefix(meta, "data:") {
		return "", fiber.NewError(fiber.StatusBadRequest, "缩略图格式错误")
	}
	meta = strings.TrimPrefix(meta, "data:")
	semi := strings.Index(meta, ";")
	if semi == -1 || !strings.Contains(meta[semi:], "base64") {
		return "", fiber.NewError(fiber.StatusBadRequest, "缩略图编码格式错误")
	}
	mime := meta[:semi]
	ext, ok := supportedThumbMime[strings.ToLower(mime)]
	if !ok {
		return "", fiber.NewError(fiber.StatusBadRequest, "不支持的缩略图格式")
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, "缩略图解码失败")
	}
	if len(decoded) > galleryThumbMaxSize {
		return "", fiber.NewError(fiber.StatusBadRequest, "缩略图大小超过限制")
	}

	dir := "./data/gallery/thumbs"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	// For non-GIF images, convert to WebP
	if ext != ".gif" {
		img, _, decErr := image.Decode(bytes.NewReader(decoded))
		if decErr == nil {
			webpData, encErr := utils.EncodeImageToWebPWithCWebP(img, galleryThumbWebpQuality)
			if encErr == nil {
				filename := attachmentID + ".webp"
				fullPath := filepath.Join(dir, filename)
				if err := os.WriteFile(fullPath, webpData, 0o644); err != nil {
					return "", err
				}
				return "/api/v1/gallery/thumbs/" + filename, nil
			}
		}
	}

	// Fallback: save original format (GIF or if WebP conversion failed)
	filename := attachmentID + ext
	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, decoded, 0o644); err != nil {
		return "", err
	}
	return "/api/v1/gallery/thumbs/" + filename, nil
}

func removeGalleryThumbFile(url string) {
	if url == "" {
		return
	}
	idx := strings.LastIndex(url, "/")
	if idx == -1 {
		return
	}
	filename := url[idx+1:]
	if filename == "" {
		return
	}
	_ = os.Remove(filepath.Join("./data/gallery/thumbs", filename))
}

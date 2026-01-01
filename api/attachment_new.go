package api

import (
	"encoding/hex"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"modernc.org/libc/limits"

	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

func uploadFiles(
	files []*multipart.FileHeader,
	uid string,
	modelSolve func(item *model.AttachmentModel),
	mimeMatcher []string,
	mimeCheckResult func(file *multipart.FileHeader, contentType string, allowed bool) int, // 返回0继续上传 返回-1跳过，返回-2中止
) (err error, ids []string, filenames []string) {
	tmpDir := appConfig.Storage.Local.TempDir
	if strings.TrimSpace(tmpDir) == "" {
		tmpDir = "./data/temp/"
	}
	_ = appFs.MkdirAll(tmpDir, 0755)

	// 遍历每个文件
	for _, file := range files {
		if len(mimeMatcher) > 0 {
			contentType := file.Header.Get("Content-Type")
			isAllowed := mimetype.EqualsAny(
				contentType,
				mimeMatcher...,
			)
			if mimeCheckResult != nil {
				checkResult := mimeCheckResult(file, contentType, isAllowed)
				if checkResult == -2 {
					return nil, ids, filenames
				} else if checkResult == -1 {
					continue
				}
			} else {
				continue
			}
		}

		tempFile, err := afero.TempFile(appFs, tmpDir, "*.upload")
		if err != nil {
			return err, nil, nil
		}

		limit := appConfig.ImageSizeLimit * 1024
		if limit == 0 {
			limit = limits.INT_MAX
		}
		saveResult, err := SaveMultipartFile(file, tempFile, limit)
		if err != nil {
			return err, nil, nil
		}
		hexString := hex.EncodeToString(saveResult.Hash)
		fn := fmt.Sprintf("%s_%d", hexString, saveResult.Size)

		_ = tempFile.Close()
		location, err := service.PersistAttachmentFile(saveResult.Hash, saveResult.Size, tempFile.Name(), saveResult.MimeType)
		if err != nil {
			return err, nil, nil
		}

		attachment := &model.AttachmentModel{
			Filename:    file.Filename,
			Size:        saveResult.Size,
			Hash:        saveResult.Hash,
			MimeType:    saveResult.MimeType,
			IsAnimated:  saveResult.IsAnimated,
			UserID:      uid,
			StorageType: location.StorageType,
			ObjectKey:   location.ObjectKey,
			ExternalURL: location.ExternalURL,
		}

		attachment.ID = utils.NewID()
		if modelSolve != nil {
			modelSolve(attachment)
		}
		model.AttachmentCreate(attachment)

		filenames = append(filenames, fn)
		ids = append(ids, attachment.ID)
	}

	return nil, ids, filenames
}

func UploadRaw(c *fiber.Ctx, uploadCallback func(item *model.AttachmentModel)) (fiber.Map, error) {
	// 解析表单中的文件
	form, err := c.MultipartForm()
	if err != nil {
		return nil, err
	}

	// 获取上传的文件切片
	files := form.File["file"]

	var ui = getCurUser(c)
	getFromForm := func(key string) string {
		if v, exists := form.Value[key]; exists {
			if len(v) > 0 {
				return v[0]
			}
		}
		return ""
	}

	parentId := getFromForm("parentId")
	parentIdType := getFromForm("parentIdType")
	rootId := getFromForm("rootId")
	rootIdType := getFromForm("rootIdType")
	extra := getFromForm("extra")

	// 遍历每个文件
	err, ids, filenames := uploadFiles(files, ui.ID, func(item *model.AttachmentModel) {
		item.ParentID = parentId
		item.ParentIDType = parentIdType
		item.RootID = rootId
		item.RootIDType = rootIdType
		item.Extra = extra

		item.UserID = ui.ID
		item.CreatorName = ui.Nickname
		item.CreatorAvatar = ui.Avatar

		if uploadCallback != nil {
			uploadCallback(item)
		}
	}, nil, nil)

	// 特殊值处理
	// for _, fn := range filenames {
	// 	if extra == "user-avatar" {
	// 		u := getCurUser(c)
	// 		u.Avatar = "id:" + fn
	// 		u.SaveAvatar()
	// 	}
	// }

	return fiber.Map{
		"message": "上传成功",
		"files":   filenames,
		"ids":     ids,
		"extra":   extra,
	}, nil
}

func AttachmentUploadTempFile(c *fiber.Ctx) error {
	// 使用 UploadRaw 重构函数
	result, err := UploadRaw(c, func(item *model.AttachmentModel) {
		item.IsTemp = true
	})
	if err != nil {
		return wrapError(c, err, "")
	}
	return c.JSON(result)
}

// AttachmentUploadQuick 上传前检查哈希，如果文件已存在，则使用快速上传
func AttachmentUploadQuick(c *fiber.Ctx) error {
	ui := getCurUser(c)
	var body struct {
		Hash string `json:"hash"`
		Size int64  `json:"size"`

		Extra  string `json:"extra"`
		Note   string `json:"note"`
		IsTemp bool   `json:"isTemp"` // 临时文件标记，先上传上来，无问题转正，有问题自动删除

		RootIdType   string `json:"rootIdType"`
		RootId       string `json:"rootId"`
		ParentIdType string `json:"parentIdType"`
		ParentId     string `json:"parentId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return wrapError(c, err, "提交的数据存在问题")
	}

	hashBytes, err := hex.DecodeString(body.Hash)
	if err != nil {
		return wrapError(c, err, "提交的数据存在问题")
	}

	db := model.GetDB()
	var item model.AttachmentModel
	db.Where("hash = ? and size = ?", hashBytes, body.Size).Limit(1).Find(&item)
	if item.ID == "" {
		return wrapError(c, nil, "此项数据无法进行快速上传")
	}

	_, newItem := model.AttachmentCreate(&model.AttachmentModel{
		Filename:    item.Filename,
		Size:        item.Size,
		Hash:        hashBytes,
		MimeType:    item.MimeType,
		IsAnimated:  item.IsAnimated,
		StorageType: item.StorageType,
		ObjectKey:   item.ObjectKey,
		ExternalURL: item.ExternalURL,

		ParentID:     body.ParentId,
		ParentIDType: body.ParentIdType,
		RootID:       body.RootId,
		RootIDType:   body.RootIdType,

		Extra: body.Extra,
		Note:  body.Note,

		UserID:        ui.ID,
		CreatorName:   ui.Nickname,
		CreatorAvatar: ui.Avatar,
	})

	return c.JSON(fiber.Map{
		"message": "上传成功",
		"file":    newItem,
	})
}

// AttachmentSetConfirm 转为正式附件
func AttachmentSetConfirm(c *fiber.Ctx) error {
	data := &struct {
		Ids          []string `json:"ids"` // 获取Id列表
		RootIdType   string   `json:"rootIdType"`
		RootId       string   `json:"rootId"`
		ParentIdType string   `json:"parentIdType"`
		ParentId     string   `json:"parentId"`
		Extra        string   `json:"extra"`
		Note         string   `json:"note"`
		Note2        string   `json:"note2"`
		IsTemp       bool     `json:"isTemp"`
	}{}

	if err := c.BodyParser(data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	if len(data.Ids) > 0 {
		q := model.AttachmentSetConfirm(data.Ids, map[string]any{
			"rootIdType":   data.RootIdType,
			"rootId":       data.RootId,
			"parentIdType": data.ParentIdType,
			"parentId":     data.ParentId,
			"extra":        data.Extra,
			"note":         data.Note,
			"isTemp":       data.IsTemp,
		})
		return c.JSON(fiber.Map{
			"rowsAffected": q.RowsAffected,
		})
	}

	return c.JSON(fiber.Map{})
}

// AttachmentDelete 转为正式附件
func AttachmentDelete(c *fiber.Ctx) error {
	data := &struct {
		Ids []string `json:"ids"` // 获取Id列表
	}{}
	if err := c.BodyParser(data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	ret := model.AttachmentsSetDelete(data.Ids)
	return c.JSON(fiber.Map{
		"rowsAffected": ret,
	})
}

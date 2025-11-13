package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"modernc.org/libc/limits"

	"sealchat/model"
	"sealchat/service"
)

// UploadQuick 上传前检查哈希，如果文件已存在，则使用快速上传
func UploadQuick(c *fiber.Ctx) error {
	var body struct {
		Hash      string `json:"hash"`
		Size      int64  `json:"size"`
		ChannelID string `json:"channelId"`
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

	tx, newItem := model.AttachmentCreate(&model.AttachmentModel{
		Filename:    item.Filename,
		Size:        item.Size,
		Hash:        hashBytes,
		ChannelID:   body.ChannelID,
		UserID:      getCurUser(c).ID,
		StorageType: item.StorageType,
		ObjectKey:   item.ObjectKey,
		ExternalURL: item.ExternalURL,
	})
	if tx.Error != nil {
		return wrapError(c, tx.Error, "上传失败，请重试")
	}

	// 特殊值处理
	if body.ChannelID == "user-avatar" {
		user := getCurUser(c)
		user.Avatar = "id:" + newItem.ID
		user.SaveAvatar()
	}

	return c.JSON(fiber.Map{
		"message": "上传成功",
		"file":    newItem,
		"id":      newItem.ID,
	})
}

func Upload(c *fiber.Ctx) error {
	// 解析表单中的文件
	form, err := c.MultipartForm()
	if err != nil {
		return wrapError(c, err, "上传失败，请重试")
	}
	channelId := getHeader(c, "Channelid") // header中只能首字大写

	// 获取上传的文件切片
	files := form.File["file"]
	filenames := []string{}
	ids := []string{}

	tmpDir := appConfig.Storage.Local.TempDir
	if strings.TrimSpace(tmpDir) == "" {
		tmpDir = "./data/temp/"
	}

	// 遍历每个文件
	for _, file := range files {
		_ = appFs.MkdirAll(tmpDir, 0755)

		tempFile, err := afero.TempFile(appFs, tmpDir, "*.upload")
		if err != nil {
			return wrapError(c, err, "上传失败，请重试")
		}

		limit := appConfig.ImageSizeLimit * 1024
		if limit == 0 {
			limit = limits.INT_MAX
		}
		hashCode, savedSize, err := SaveMultipartFile(file, tempFile, limit)
		if err != nil {
			return err
		}
		hexString := hex.EncodeToString(hashCode)
		fn := fmt.Sprintf("%s_%d", hexString, savedSize)
		_ = tempFile.Close()

		location, err := service.PersistAttachmentFile(hashCode, savedSize, tempFile.Name(), file.Header.Get("Content-Type"))
		if err != nil {
			return wrapError(c, err, "上传失败，请重试")
		}

		tx, newItem := model.AttachmentCreate(&model.AttachmentModel{
			Filename:    file.Filename,
			Size:        savedSize,
			Hash:        hashCode,
			ChannelID:   channelId,
			UserID:      getCurUser(c).ID,
			StorageType: location.StorageType,
			ObjectKey:   location.ObjectKey,
			ExternalURL: location.ExternalURL,
		})
		if tx.Error != nil {
			return wrapError(c, tx.Error, "上传失败，请重试")
		}

		filenames = append(filenames, fn)
		ids = append(ids, newItem.ID)

		// 特殊值处理
		if channelId == "user-avatar" {
			user := getCurUser(c)
			user.Avatar = "id:" + newItem.ID
			user.SaveAvatar()
		}
	}

	return c.JSON(fiber.Map{
		"message": "上传成功",
		"files":   filenames,
		"ids":     ids,
	})
}

func AttachmentList(c *fiber.Ctx) error {
	var items []*model.AttachmentModel
	user := getCurUser(c)
	model.GetDB().Where("user_id = ?", user.ID).Select("id, created_at, hash").Find(&items)

	return c.JSON(fiber.Map{
		"message": "ok",
		"data":    items,
	})
}

func AttachmentGet(c *fiber.Ctx) error {
	attachmentID := c.Params("id")
	if attachmentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "无效的附件ID",
		})
	}
	var att model.AttachmentModel
	if err := model.GetDB().Where("id = ?", attachmentID).Limit(1).Find(&att).Error; err != nil {
		return wrapError(c, err, "读取附件失败")
	}
	if att.ID == "" {
		if served, err := trySendUploadFile(c, attachmentID); served {
			return err
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "附件不存在",
		})
	}
	if att.StorageType == model.StorageS3 {
		if redirected := redirectAttachmentToRemote(c, &att); redirected {
			return nil
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "附件文件不存在",
		})
	}

	if strings.TrimSpace(att.ObjectKey) != "" {
		if path, err := service.ResolveLocalAttachmentPath(att.ObjectKey); err == nil {
			if _, err := os.Stat(path); err == nil {
				return c.SendFile(path)
			}
		}
	}

	filename := fmt.Sprintf("%s_%d", hex.EncodeToString([]byte(att.Hash)), att.Size)
	uploadRoot := appConfig.Storage.Local.UploadDir
	if strings.TrimSpace(uploadRoot) == "" {
		uploadRoot = "./data/upload"
	}
	fullPath := filepath.Join(uploadRoot, filename)
	if _, err := os.Stat(fullPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "附件文件不存在",
			})
		}
		return wrapError(c, err, "读取附件失败")
	}
	return c.SendFile(fullPath)
}

func AttachmentMeta(c *fiber.Ctx) error {
	attachmentID := c.Params("id")
	if attachmentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "无效的附件ID",
		})
	}

	var att model.AttachmentModel
	if err := model.GetDB().Where("id = ?", attachmentID).Limit(1).Find(&att).Error; err != nil {
		return wrapError(c, err, "读取附件失败")
	}
	if att.ID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "附件不存在",
		})
	}

	publicURL := service.AttachmentPublicURL(&att)
	return c.JSON(fiber.Map{
		"message": "ok",
		"item": fiber.Map{
			"id":          att.ID,
			"filename":    att.Filename,
			"size":        att.Size,
			"hash":        att.Hash,
			"storageType": att.StorageType,
			"objectKey":   att.ObjectKey,
			"externalUrl": att.ExternalURL,
			"publicUrl":   publicURL,
		},
	})
}

func wrapErrorStatus(c *fiber.Ctx, status int, err error, s string) error {
	m := fiber.Map{
		"message": s,
	}
	if err != nil {
		m["error"] = err.Error()
	}
	return c.Status(status).JSON(m)
}

func wrapError(c *fiber.Ctx, err error, s string) error {
	return wrapErrorStatus(c, fiber.StatusBadRequest, err, s)
}

var attachmentFileTokenPattern = regexp.MustCompile(`^[0-9a-fA-F]{32,}_[0-9]+$`)

func trySendUploadFile(c *fiber.Ctx, token string) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, nil
	}
	if strings.ContainsAny(token, "/\\") {
		return false, nil
	}
	if !attachmentFileTokenPattern.MatchString(token) {
		return false, nil
	}
	uploadRoot := appConfig.Storage.Local.UploadDir
	if strings.TrimSpace(uploadRoot) == "" {
		uploadRoot = "./data/upload"
	}
	fullPath := filepath.Join(uploadRoot, token)
	if _, err := os.Stat(fullPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return true, wrapError(c, err, "读取附件失败")
	}
	return true, c.SendFile(fullPath)
}

func getHeader(c *fiber.Ctx, name string) string {
	var value string
	if len(name) > 1 {
		newName := strings.ToLower(name)
		name = name[:1] + newName[1:]
	}

	items := c.GetReqHeaders()[name] // header中只能首字大写
	if len(items) > 0 {
		value = items[0]
	}
	return value
}

func redirectAttachmentToRemote(c *fiber.Ctx, att *model.AttachmentModel) bool {
	if att == nil {
		return false
	}
	target := service.AttachmentPublicURL(att)
	if target == "" {
		return false
	}
	_ = c.Redirect(target, fiber.StatusTemporaryRedirect)
	return true
}

package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"modernc.org/libc/limits"

	"sealchat/model"
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

	_, newItem := model.AttachmentCreate(&model.AttachmentModel{
		Filename:  item.Filename,
		Size:      item.Size,
		Hash:      hashBytes,
		ChannelID: body.ChannelID,
		UserID:    getCurUser(c).ID,
	})

	// 特殊值处理
	if body.ChannelID == "user-avatar" {
		fn := fmt.Sprintf("%s_%d", body.Hash, item.Size)
		user := getCurUser(c)
		user.Avatar = "id:" + fn
		user.SaveAvatar()
	}

	return c.JSON(fiber.Map{
		"message": "上传成功",
		"file":    newItem,
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

	tmpDir := "./data/temp/"
	uploadDir := "./data/upload/"

	// 遍历每个文件
	for _, file := range files {
		// f, err := appFs.Open("./assets/" + file.Filename + ".upload")
		// if err != nil {
		//	return err
		// }
		_ = appFs.MkdirAll(tmpDir, 0755)
		_ = appFs.MkdirAll(uploadDir, 0755)

		tempFile, err := afero.TempFile(appFs, tmpDir, "*.upload")
		if err != nil {
			return wrapError(c, err, "上传失败，请重试")
		}

		limit := appConfig.ImageSizeLimit * 1024
		if limit == 0 {
			limit = limits.INT_MAX
		}
		hashCode, err := SaveMultipartFile(file, tempFile, limit)
		if err != nil {
			return err
		}
		hexString := hex.EncodeToString(hashCode)
		fn := fmt.Sprintf("%s_%d", hexString, file.Size)
		_ = tempFile.Close()

		if _, err := os.Stat(fn); errors.Is(err, os.ErrNotExist) {
			if err = appFs.Rename(tempFile.Name(), uploadDir+fn); err != nil {
				return wrapError(c, err, "上传失败，请重试")
			}
		} else {
			// 文件已存在，复用并删除临时文件
			_ = appFs.Remove(tempFile.Name())
		}

		model.AttachmentCreate(&model.AttachmentModel{
			Filename:  file.Filename,
			Size:      file.Size,
			Hash:      hashCode,
			ChannelID: channelId,
			UserID:    getCurUser(c).ID,
		})

		filenames = append(filenames, fn)

		// 特殊值处理
		if channelId == "user-avatar" {
			user := getCurUser(c)
			user.Avatar = "id:" + fn
			user.SaveAvatar()
		}
	}

	return c.JSON(fiber.Map{
		"message": "上传成功",
		"files":   filenames,
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "附件不存在",
		})
	}
	filename := fmt.Sprintf("%s_%d", hex.EncodeToString([]byte(att.Hash)), att.Size)
	fullPath := filepath.Join("./data/upload", filename)
	file, err := appFs.Open(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "附件文件不存在",
			})
		}
		return wrapError(c, err, "读取附件失败")
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return wrapError(c, err, "读取附件失败")
	}

	c.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	return c.SendStream(file)
}

func wrapError(c *fiber.Ctx, err error, s string) error {
	m := fiber.Map{
		"message": s,
	}
	if err != nil {
		m["error"] = err.Error()
	}
	return c.Status(fiber.StatusBadRequest).JSON(m)
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

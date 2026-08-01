package api

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

const cursorAssetMaxDimension = 128

func normalizeCursorAssetSize(size int) int {
	if size == 0 {
		return utils.DefaultCursorSize
	}
	return max(utils.MinCursorSize, min(size, cursorAssetMaxDimension))
}

func resizeCursorDimensions(width, height, targetSize int) (int, int) {
	targetSize = normalizeCursorAssetSize(targetSize)
	if width >= height {
		return targetSize, max(1, height*targetSize/width)
	}
	return max(1, width*targetSize/height), targetSize
}

func resizeAnimatedWebP(data []byte, width, height, quality int) ([]byte, error) {
	toolchain := service.ResolveMediaToolchain(&appConfig.Audio)
	return service.ResizeAnimatedWebPCursor(data, width, height, quality, toolchain)
}

func encodeCursorAsset(data []byte, mimeType string, quality, targetSize int) ([]byte, int, int, bool, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, false, fmt.Errorf("无法读取图片")
	}
	width, height := resizeCursorDimensions(config.Width, config.Height, targetSize)
	if mimeType == "image/gif" {
		decoded, decodeErr := gif.DecodeAll(bytes.NewReader(data))
		if decodeErr != nil {
			return nil, 0, 0, false, fmt.Errorf("无法读取 GIF")
		}
		if len(decoded.Image) > 1 {
			encoded, encodeErr := utils.EncodeGIFToWebPWithGIF2WebPResize(data, quality, width, height)
			return encoded, width, height, true, encodeErr
		}
	}
	if mimeType == "image/webp" && bytes.Contains(data, []byte("ANIM")) {
		if config.Width == width && config.Height == height {
			return data, width, height, true, nil
		}
		encoded, resizeErr := resizeAnimatedWebP(data, width, height, quality)
		return encoded, width, height, true, resizeErr
	}
	decoded, _, decodeErr := image.Decode(bytes.NewReader(data))
	if decodeErr != nil {
		return nil, 0, 0, false, fmt.Errorf("无法解码图片")
	}
	if width != config.Width || height != config.Height {
		resized := image.NewNRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(resized, resized.Bounds(), decoded, decoded.Bounds(), draw.Over, nil)
		decoded = resized
	}
	encoded, encodeErr := utils.EncodeImageToWebPWithCWebP(decoded, quality)
	return encoded, width, height, false, encodeErr
}

func CursorAssetUploadHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	form, err := c.MultipartForm()
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "上传内容无效")
	}
	files := form.File["file"]
	if len(files) != 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请选择一张鼠标图片"})
	}
	value := func(key string) string {
		items := form.Value[key]
		if len(items) == 0 {
			return ""
		}
		return strings.TrimSpace(items[0])
	}
	scope := value("scope")
	worldID := value("worldId")
	targetSize, parseErr := strconv.Atoi(value("size"))
	if parseErr != nil && value("size") != "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "鼠标尺寸无效"})
	}
	if targetSize != 0 && (targetSize < utils.MinCursorSize || targetSize > cursorAssetMaxDimension) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": fmt.Sprintf("鼠标尺寸必须在 %d-%d 之间", utils.MinCursorSize, cursorAssetMaxDimension)})
	}
	targetSize = normalizeCursorAssetSize(targetSize)
	rootID := "platform"
	rootIDType := "platform_cursor"
	switch scope {
	case "platform":
		if !pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "仅平台管理员可上传平台鼠标样式"})
		}
	case "world":
		if worldID == "" || !service.IsWorldAdmin(worldID, user.ID) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "仅世界管理员可上传世界鼠标样式"})
		}
		rootID = worldID
		rootIDType = "world_cursor"
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "鼠标样式作用域无效"})
	}

	file, err := files[0].Open()
	if err != nil {
		return wrapError(c, err, "读取图片失败")
	}
	defer file.Close()
	limit := appConfig.ImageSizeLimit * 1024
	if limit <= 0 {
		limit = 20 * 1024 * 1024
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return wrapError(c, err, "读取图片失败")
	}
	if int64(len(data)) > limit {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"message": "图片超过上传限制"})
	}
	mimeType := strings.ToLower(strings.TrimSpace(mimetype.Detect(data).String()))
	if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/gif" && mimeType != "image/webp" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "仅支持 PNG、JPEG、GIF、WebP"})
	}
	encoded, width, height, animated, err := encodeCursorAsset(data, mimeType, appConfig.ImageCompressQuality, targetSize)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "图片转换为 WebP 失败")
	}
	if int64(len(encoded)) > limit {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"message": "转换后的 WebP 超过上传限制"})
	}

	tmpDir := appConfig.Storage.Local.TempDir
	if strings.TrimSpace(tmpDir) == "" {
		tmpDir = "./data/temp/"
	}
	_ = appFs.MkdirAll(tmpDir, 0o755)
	tempFile, err := afero.TempFile(appFs, tmpDir, "cursor-*.webp")
	if err != nil {
		return wrapError(c, err, "创建临时文件失败")
	}
	tempPath := tempFile.Name()
	hash, size, writeErr := copyWithHash(tempFile, bytes.NewReader(encoded))
	closeErr := tempFile.Close()
	if writeErr != nil {
		return wrapError(c, writeErr, "写入鼠标图片失败")
	}
	if closeErr != nil {
		return wrapError(c, closeErr, "写入鼠标图片失败")
	}
	location, err := service.PersistAttachmentFile(hash, size, tempPath, "image/webp")
	if err != nil {
		return wrapError(c, err, "保存鼠标图片失败")
	}
	attachment := &model.AttachmentModel{
		Filename:      strings.TrimSuffix(filepath.Base(files[0].Filename), filepath.Ext(files[0].Filename)) + ".webp",
		Size:          size,
		Hash:          hash,
		MimeType:      "image/webp",
		IsAnimated:    animated,
		UserID:        user.ID,
		StorageType:   location.StorageType,
		ObjectKey:     location.ObjectKey,
		ExternalURL:   location.ExternalURL,
		RootID:        rootID,
		RootIDType:    rootIDType,
		Extra:         "cursor_asset",
		IsTemp:        true,
		CreatorName:   user.Nickname,
		CreatorAvatar: user.Avatar,
	}
	_, attachment = model.AttachmentCreate(attachment)
	return c.JSON(fiber.Map{
		"attachmentId": attachment.ID,
		"width":        width,
		"height":       height,
		"animated":     animated,
		"mimeType":     attachment.MimeType,
	})
}

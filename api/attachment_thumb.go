package api

import (
	"encoding/hex"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"

	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

const (
	// Threshold for generating thumbnails (30KB)
	thumbnailSizeThreshold = 30 * 1024
	// Default thumbnail cache directory
	defaultThumbDir = "./data/thumbs"
	// WebP quality for thumbnails
	thumbWebpQuality = 65
)

// AttachmentThumb returns a thumbnail version of an image attachment
// GET /api/v1/attachment/:id/thumb?size=150
func AttachmentThumb(c *fiber.Ctx) error {
	attachmentID := c.Params("id")
	if attachmentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "无效的附件ID",
		})
	}

	// Parse size parameter (default 150, max 400)
	size := c.QueryInt("size", 150)
	if size < 50 {
		size = 50
	}
	if size > 400 {
		size = 400
	}

	// Get attachment metadata
	var att model.AttachmentModel
	if err := model.GetDB().Where("id = ?", attachmentID).Limit(1).Find(&att).Error; err != nil {
		return wrapError(c, err, "读取附件失败")
	}
	if att.ID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "附件不存在",
		})
	}

	// If original is small enough, just serve it directly
	if att.Size < thumbnailSizeThreshold {
		return AttachmentGet(c)
	}

	// For animated images (GIF/animated WebP), serve original to preserve animation
	if att.IsAnimated {
		return AttachmentGet(c)
	}

	// Check if thumbnail is already cached
	thumbPath := getThumbCachePath(attachmentID, size)
	if _, err := os.Stat(thumbPath); err == nil {
		// Thumbnail exists, serve it
		setThumbCacheHeaders(c)
		return c.SendFile(thumbPath)
	}

	// Need to generate thumbnail
	originalPath, err := getAttachmentPath(&att)
	if err != nil {
		// Can't access original, fall back to serving it directly
		return AttachmentGet(c)
	}

	// Generate thumbnail
	if err := generateThumbnail(originalPath, thumbPath, size); err != nil {
		// Generation failed, serve original
		return AttachmentGet(c)
	}

	// Serve the generated thumbnail
	setThumbCacheHeaders(c)
	return c.SendFile(thumbPath)
}

// getThumbCachePath returns the cache path for a thumbnail (.webp)
func getThumbCachePath(attachmentID string, size int) string {
	thumbDir := defaultThumbDir
	_ = os.MkdirAll(thumbDir, 0755)
	return filepath.Join(thumbDir, fmt.Sprintf("%s_%d.webp", attachmentID, size))
}

// getAttachmentPath resolves the local file path for an attachment
func getAttachmentPath(att *model.AttachmentModel) (string, error) {
	// S3 storage not supported for thumbnails
	if att.StorageType == model.StorageS3 {
		return "", fmt.Errorf("S3 attachments don't support local thumbnail generation")
	}

	// Try ObjectKey first
	if strings.TrimSpace(att.ObjectKey) != "" {
		if path, err := service.ResolveLocalAttachmentPath(att.ObjectKey); err == nil {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	// Fallback to hash-based path
	uploadRoot := "./data/upload"
	if appConfig != nil && appConfig.Storage.Local.UploadDir != "" {
		uploadRoot = appConfig.Storage.Local.UploadDir
	}
	filename := fmt.Sprintf("%s_%d", hex.EncodeToString([]byte(att.Hash)), att.Size)
	fullPath := filepath.Join(uploadRoot, filename)
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath, nil
	}

	return "", fmt.Errorf("attachment file not found")
}

// generateThumbnail creates a WebP thumbnail from the original image
func generateThumbnail(srcPath, dstPath string, maxSize int) error {
	// Open source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Detect and decode image format
	var srcImg image.Image

	// Try to detect format first
	_, format, err := image.DecodeConfig(srcFile)
	if err != nil {
		return fmt.Errorf("unable to decode image config: %w", err)
	}

	// Seek back to start
	if _, err := srcFile.Seek(0, 0); err != nil {
		return err
	}

	// Decode based on format
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		srcImg, err = jpeg.Decode(srcFile)
	case "png":
		srcImg, err = png.Decode(srcFile)
	case "gif":
		srcImg, err = gif.Decode(srcFile)
	case "webp":
		srcImg, err = webp.Decode(srcFile)
	default:
		// Try generic decode
		if _, err := srcFile.Seek(0, 0); err != nil {
			return err
		}
		srcImg, _, err = image.Decode(srcFile)
	}
	if err != nil {
		return fmt.Errorf("unable to decode image: %w", err)
	}

	// Calculate new dimensions maintaining aspect ratio
	bounds := srcImg.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	var dstWidth, dstHeight int
	if srcWidth > srcHeight {
		dstWidth = maxSize
		dstHeight = int(float64(srcHeight) * float64(maxSize) / float64(srcWidth))
	} else {
		dstHeight = maxSize
		dstWidth = int(float64(srcWidth) * float64(maxSize) / float64(srcHeight))
	}

	// Ensure minimum size
	if dstWidth < 1 {
		dstWidth = 1
	}
	if dstHeight < 1 {
		dstHeight = 1
	}

	// Create destination image
	dstImg := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))

	// Resize using high-quality interpolation
	draw.CatmullRom.Scale(dstImg, dstImg.Bounds(), srcImg, bounds, draw.Over, nil)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}

	// Encode as WebP using existing utility
	webpData, encodeErr := utils.EncodeImageToWebPWithCWebP(dstImg, thumbWebpQuality)
	if encodeErr != nil {
		return fmt.Errorf("unable to encode WebP: %w", encodeErr)
	}

	// Write to file
	return os.WriteFile(dstPath, webpData, 0644)
}

// setThumbCacheHeaders sets appropriate cache headers for thumbnails
func setThumbCacheHeaders(c *fiber.Ctx) {
	c.Set("Cache-Control", "public, max-age=31536000, immutable")
	c.Set("Content-Type", "image/webp")
}

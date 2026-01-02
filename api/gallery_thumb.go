package api

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	_ "golang.org/x/image/webp"

	"sealchat/utils"
)

const (
	galleryThumbDir         = "./data/gallery/thumbs"
	galleryThumbWebpQuality = 65
)

// GalleryThumbServe serves gallery thumbnails, converting PNG/JPG to WebP on demand.
// GET /api/v1/gallery/thumbs/:filename
func GalleryThumbServe(c *fiber.Ctx) error {
	filename := strings.TrimSpace(c.Params("filename"))
	if filename == "" || filename == "." || filename == ".." ||
		filepath.Base(filename) != filename ||
		strings.Contains(filename, "\\") || strings.Contains(filename, "/") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "无效的缩略图文件名",
		})
	}

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
	default:
		return c.SendStatus(fiber.StatusNotFound)
	}

	originalPath := filepath.Join(galleryThumbDir, filename)

	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		return c.SendStatus(fiber.StatusNotFound)
	} else if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "读取缩略图失败",
		})
	}

	// GIF and WebP: serve directly without conversion
	if ext == ".gif" || ext == ".webp" {
		if ext == ".webp" {
			setGalleryThumbWebpHeaders(c)
		}
		return c.SendFile(originalPath)
	}

	// PNG/JPG: check if WebP exists, generate if needed, then redirect
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return c.SendFile(originalPath)
	}

	baseName := strings.TrimSuffix(filename, ext)
	webpFilename := baseName + ".webp"
	webpPath := filepath.Join(galleryThumbDir, webpFilename)

	// Generate WebP if not exists
	if _, err := os.Stat(webpPath); os.IsNotExist(err) {
		file, err := os.Open(originalPath)
		if err != nil {
			return c.SendFile(originalPath)
		}
		defer file.Close()

		img, _, err := image.Decode(file)
		if err != nil {
			return c.SendFile(originalPath)
		}

		webpData, err := utils.EncodeImageToWebPWithCWebP(img, galleryThumbWebpQuality)
		if err != nil {
			return c.SendFile(originalPath)
		}

		// Atomic write
		_ = os.MkdirAll(galleryThumbDir, 0o755)
		tmpFile, err := os.CreateTemp(galleryThumbDir, baseName+".*.webp")
		if err == nil {
			tmpName := tmpFile.Name()
			if _, err := tmpFile.Write(webpData); err != nil {
				_ = tmpFile.Close()
				_ = os.Remove(tmpName)
			} else if err := tmpFile.Close(); err != nil {
				_ = os.Remove(tmpName)
			} else if err := os.Rename(tmpName, webpPath); err != nil {
				_ = os.Remove(tmpName)
			}
		}
	}

	// Redirect to WebP URL
	return c.Redirect("/api/v1/gallery/thumbs/"+webpFilename, fiber.StatusMovedPermanently)
}

func setGalleryThumbWebpHeaders(c *fiber.Ctx) {
	c.Set("Cache-Control", "public, max-age=31536000, immutable")
	c.Set("Content-Type", "image/webp")
}

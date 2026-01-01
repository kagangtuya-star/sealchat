package api

import (
	"bufio"
	"bytes"
	"errors"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"sealchat/pm/gen"
	"sealchat/utils"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/mikespook/gorbac"
	"github.com/samber/lo"
	"github.com/spf13/afero"
	"golang.org/x/crypto/blake2s"
	_ "golang.org/x/image/webp"

	"sealchat/pm"
)

var copyBufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 4096)
	},
}

func copyZeroAlloc(w io.Writer, r io.Reader) (int64, error) {
	vbuf := copyBufPool.Get()
	buf := vbuf.([]byte)
	n, err := io.CopyBuffer(w, r, buf)
	copyBufPool.Put(vbuf)
	return n, err
}

// ErrFileTooLarge is returned when uploaded file exceeds size limit
var ErrFileTooLarge = errors.New("文件大小超过限制")

// SaveMultipartFileResult contains the result of saving a multipart file
type SaveMultipartFileResult struct {
	Hash       []byte
	Size       int64
	MimeType   string // Final MIME type after conversion (e.g., image/webp)
	IsAnimated bool   // Whether the image is animated (e.g., animated WebP from GIF)
}

func SaveMultipartFile(fh *multipart.FileHeader, fOut afero.File, limit int64) (result SaveMultipartFileResult, err error) {
	file, err := fh.Open()
	if err != nil {
		return SaveMultipartFileResult{}, err
	}
	defer func() {
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
	}()

	peek := make([]byte, 512)
	n, _ := io.ReadFull(file, peek)
	peek = peek[:n]
	mimeType := detectUploadMime(fh, peek)

	// Reset file position after peeking
	if seeker, ok := file.(io.Seeker); ok {
		_, _ = seeker.Seek(0, io.SeekStart)
	} else {
		// If can't seek, close and reopen
		_ = file.Close()
		file, err = fh.Open()
		if err != nil {
			return SaveMultipartFileResult{}, err
		}
	}

	if shouldCompressUpload(mimeType) {
		// Read with limit + 1 to detect oversized files
		limitedReader := io.LimitReader(file, limit+1)
		data, readErr := io.ReadAll(limitedReader)
		if readErr != nil {
			return SaveMultipartFileResult{}, readErr
		}

		// Check if file exceeds limit
		if int64(len(data)) > limit {
			return SaveMultipartFileResult{}, ErrFileTooLarge
		}

		if len(data) == 0 {
			hash, size, err := copyWithHash(fOut, bytes.NewReader(data))
			return SaveMultipartFileResult{Hash: hash, Size: size, MimeType: mimeType}, err
		}

		compressed, finalMime, ok, isAnimated, compErr := tryCompressImage(data, mimeType, appConfig.ImageCompressQuality)
		if compErr != nil {
			return SaveMultipartFileResult{}, compErr
		}
		if ok && len(compressed) > 0 {
			hash, size, err := copyWithHash(fOut, bytes.NewReader(compressed))
			return SaveMultipartFileResult{Hash: hash, Size: size, MimeType: finalMime, IsAnimated: isAnimated}, err
		}
		hash, size, err := copyWithHash(fOut, bytes.NewReader(data))
		return SaveMultipartFileResult{Hash: hash, Size: size, MimeType: mimeType, IsAnimated: isAnimated}, err
	}

	// For non-image files, also check size limit
	reader := bufio.NewReader(io.LimitReader(file, limit+1))
	data, readErr := io.ReadAll(reader)
	if readErr != nil {
		return SaveMultipartFileResult{}, readErr
	}
	if int64(len(data)) > limit {
		return SaveMultipartFileResult{}, ErrFileTooLarge
	}
	hash, size, err := copyWithHash(fOut, bytes.NewReader(data))
	return SaveMultipartFileResult{Hash: hash, Size: size, MimeType: mimeType}, err
}

func copyWithHash(dst io.Writer, src io.Reader) ([]byte, int64, error) {
	hash := lo.Must(blake2s.New256(nil))
	teeReader := io.TeeReader(src, hash)
	written, err := copyZeroAlloc(dst, teeReader)
	if err != nil {
		return nil, written, err
	}
	return hash.Sum(nil), written, nil
}

func detectUploadMime(fh *multipart.FileHeader, peek []byte) string {
	contentType := strings.ToLower(strings.TrimSpace(fh.Header.Get("Content-Type")))
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	if contentType == "" || contentType == "application/octet-stream" {
		if len(peek) == 0 {
			return ""
		}
		contentType = strings.ToLower(http.DetectContentType(peek))
	}
	return contentType
}

func shouldCompressUpload(mimeType string) bool {
	if appConfig == nil || !appConfig.ImageCompress {
		return false
	}
	switch mimeType {
	// Skip webp - already compressed by frontend, avoid quality degradation
	case "image/jpeg", "image/jpg", "image/png", "image/gif":
		return true
	default:
		return false
	}
}

func tryCompressImage(data []byte, mimeType string, quality int) ([]byte, string, bool, bool, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, mimeType, false, false, nil
	}

	quality = clampImageQuality(quality)

	// For GIF, check if animated (multiple frames)
	if format == "gif" {
		gifImg, gifErr := gif.DecodeAll(bytes.NewReader(data))
		if gifErr != nil {
			return nil, mimeType, false, false, nil
		}

		// Multi-frame GIF: use gif2webp to preserve animation
		if len(gifImg.Image) > 1 {
			result, encodeErr := utils.EncodeGIFToWebPWithGIF2WebP(data, quality)
			if encodeErr != nil {
				return nil, mimeType, false, false, encodeErr
			}
			// If animated WebP is significantly larger, keep original GIF
			if len(result) > len(data)*3/2 {
				return nil, mimeType, false, true, nil // Still mark as animated even if keeping GIF
			}
			return result, "image/webp", true, true, nil // isAnimated = true
		}

		// Single-frame GIF: extract first frame for static WebP
		if len(gifImg.Image) > 0 {
			img = gifImg.Image[0]
		}
	}

	result, encodeErr := utils.EncodeImageToWebPWithCWebP(img, quality)
	if encodeErr != nil {
		return nil, mimeType, false, false, encodeErr
	}
	// Always use WebP result even if larger, for format consistency
	// If WebP result is significantly larger (>150%), fall back to original
	if len(result) > len(data)*3/2 {
		return nil, mimeType, false, false, nil
	}
	return result, "image/webp", true, false, nil
}

func clampImageQuality(val int) int {
	switch {
	case val < 1:
		return 85
	case val > 100:
		return 100
	default:
		return val
	}
}

// Can 检查当前用户是否拥有指定项目的指定权限
func Can(c *fiber.Ctx, chId string, relations ...gorbac.Permission) bool {
	ok := pm.Can(getCurUser(c).ID, chId, relations...)
	if !ok {
		_ = c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "无权限访问"})
	}
	return ok
}

// CanWithSystemRole 检查当前用户是否拥有指定权限
func CanWithSystemRole(c *fiber.Ctx, relations ...gorbac.Permission) bool {
	ok := pm.CanWithSystemRole(getCurUser(c).ID, relations...)
	if !ok {
		_ = c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "无权限访问"})
	}
	return ok
}

// CanWithSystemRole2 检查当前用户是否拥有指定权限
func CanWithSystemRole2(c *fiber.Ctx, userId string, relations ...gorbac.Permission) bool {
	ok := pm.CanWithSystemRole(userId, relations...)
	if !ok {
		_ = c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "无权限访问"})
	}
	return ok
}

// CanWithChannelRole 检查当前用户是否拥有指定项目的指定权限
func CanWithChannelRole(c *fiber.Ctx, chId string, relations ...gorbac.Permission) bool {
	ok := pm.CanWithChannelRole(getCurUser(c).ID, chId, relations...)

	if !ok {
		// 额外检查用户的系统级别权限
		var rootPerm []gorbac.Permission
		for _, i := range relations {
			p := i.ID()
			for key, _ := range gen.PermSystemMap {
				if p == key {
					rootPerm = append(rootPerm, gorbac.NewStdPermission(key))
					break
				}
			}
		}

		userId := getCurUser(c).ID
		ok = pm.CanWithSystemRole(userId, rootPerm...)
	}

	if !ok {
		_ = c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "无权限访问"})
	}
	return ok
}

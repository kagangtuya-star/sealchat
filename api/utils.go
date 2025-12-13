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
	"strings"
	"sync"

	"github.com/gen2brain/webp"
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

func SaveMultipartFile(fh *multipart.FileHeader, fOut afero.File, limit int64) (hashOut []byte, size int64, err error) {
	file, err := fh.Open()
	if err != nil {
		return nil, 0, err
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
			return nil, 0, err
		}
	}

	if shouldCompressUpload(mimeType) {
		// Read with limit + 1 to detect oversized files
		limitedReader := io.LimitReader(file, limit+1)
		data, readErr := io.ReadAll(limitedReader)
		if readErr != nil {
			return nil, 0, readErr
		}

		// Check if file exceeds limit
		if int64(len(data)) > limit {
			return nil, 0, ErrFileTooLarge
		}

		if len(data) == 0 {
			return copyWithHash(fOut, bytes.NewReader(data))
		}

		compressed, ok, compErr := tryCompressImage(data, mimeType, appConfig.ImageCompressQuality)
		if compErr != nil {
			return nil, 0, compErr
		}
		if ok && len(compressed) > 0 {
			return copyWithHash(fOut, bytes.NewReader(compressed))
		}
		return copyWithHash(fOut, bytes.NewReader(data))
	}

	// For non-image files, also check size limit
	reader := bufio.NewReader(io.LimitReader(file, limit+1))
	data, readErr := io.ReadAll(reader)
	if readErr != nil {
		return nil, 0, readErr
	}
	if int64(len(data)) > limit {
		return nil, 0, ErrFileTooLarge
	}
	return copyWithHash(fOut, bytes.NewReader(data))
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
	case "image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func tryCompressImage(data []byte, mimeType string, quality int) ([]byte, bool, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false, nil
	}

	quality = clampImageQuality(quality)
	buf := bytes.NewBuffer(make([]byte, 0, len(data)/2))

	// For GIF, extract first frame
	// (Note: this doesn't preserve animation, just the first frame)
	if format == "gif" {
		if gifImg, gifErr := gif.DecodeAll(bytes.NewReader(data)); gifErr == nil && len(gifImg.Image) > 0 {
			// Use first frame for static WebP
			img = gifImg.Image[0]
		}
	}

	// Encode to WebP format with lossy compression
	// WebP lossy mode supports alpha channel, so transparency is preserved
	encodeErr := webp.Encode(buf, img, webp.Options{
		Lossless: false,
		Quality:  quality,
	})

	if encodeErr != nil {
		return nil, false, encodeErr
	}

	result := buf.Bytes()
	// Always use WebP result even if larger, for format consistency
	// If WebP result is significantly larger (>150%), fall back to original
	if len(result) > len(data)*3/2 {
		return nil, false, nil
	}
	return result, true, nil
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

package service

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/crypto/blake2s"
	_ "golang.org/x/image/webp"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

// SatoriAttachmentConfig holds configuration for Satori content normalization
type SatoriAttachmentConfig struct {
	ImageSizeLimit       int64 // Max image size in bytes
	ImageCompress        bool
	ImageCompressQuality int
	TempDir              string
	MaxImagesPerMessage  int // Max number of images to process per message
}

// SatoriAttachmentResult contains the result of normalizing Satori content
type SatoriAttachmentResult struct {
	Content        string   // Normalized content with id:xxx references
	AttachmentIDs  []string // Created attachment IDs
	ProcessedCount int      // Number of images processed
	SkippedCount   int      // Number of images skipped (errors, limits)
	Errors         []error  // Non-fatal errors encountered
}

const (
	satoriAssetPrefix = "sealchat://asset/"
	assetIDLength     = 64
)

// NormalizeSatoriContent processes Satori content, extracting Base64 images
// and converting them to attachments. Returns normalized content with id:xxx references.
func NormalizeSatoriContent(content, userID, channelID string, cfg SatoriAttachmentConfig) (*SatoriAttachmentResult, error) {
	result := &SatoriAttachmentResult{
		Content: content,
	}

	if content == "" {
		return result, nil
	}

	content = protocol.EscapeSatoriText(content)
	result.Content = content

	// Parse Satori content
	root := protocol.ElementParse(content)
	if root == nil {
		return result, nil
	}

	// Track modifications
	modified := false
	appFs := afero.NewOsFs()

	// Ensure temp dir
	tmpDir := cfg.TempDir
	if strings.TrimSpace(tmpDir) == "" {
		tmpDir = "./data/temp/"
	}
	_ = appFs.MkdirAll(tmpDir, 0755)

	// Traverse and process img elements
	root.Traverse(func(el *protocol.Element) {
		if el.Type != "img" && el.Type != "file" {
			return
		}

		// Check max images limit (count both successful and failed attempts to prevent DoS)
		totalAttempts := result.ProcessedCount + result.SkippedCount
		if cfg.MaxImagesPerMessage > 0 && totalAttempts >= cfg.MaxImagesPerMessage {
			result.SkippedCount++
			return
		}

		srcRaw, ok := el.Attrs["src"]
		if !ok {
			return
		}
		src, ok := srcRaw.(string)
		if !ok || src == "" {
			return
		}

		if strings.HasPrefix(src, satoriAssetPrefix) {
			assetID := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(src, satoriAssetPrefix)))
			if assetID == "" || !isHexString(assetID) {
				result.SkippedCount++
				return
			}
			el.Attrs["src"] = "id:" + assetID
			result.AttachmentIDs = append(result.AttachmentIDs, assetID)
			result.ProcessedCount++
			modified = true
			return
		}

		// Skip if already an id: reference or http(s): URL
		if strings.HasPrefix(src, "id:") {
			return
		}
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			// Keep external URLs as-is per design decision
			return
		}

		// Process data: URLs
		if !strings.HasPrefix(src, "data:") {
			return
		}

		expectImage := el.Type == "img"
		attachmentID, err := processDataURLToAttachment(src, userID, channelID, cfg, appFs, tmpDir, expectImage)
		if err != nil {
			result.Errors = append(result.Errors, err)
			result.SkippedCount++
			return
		}

		// Replace src with id:xxx
		el.Attrs["src"] = "id:" + attachmentID
		result.AttachmentIDs = append(result.AttachmentIDs, attachmentID)
		result.ProcessedCount++
		modified = true
	})

	if modified {
		result.Content = root.ToString()
	}

	return result, nil
}

// processDataURLToAttachment converts a data URL to an attachment
func processDataURLToAttachment(dataURL, userID, channelID string, cfg SatoriAttachmentConfig, appFs afero.Fs, tmpDir string, expectImage bool) (string, error) {
	// Parse data URL: data:[<mediatype>][;base64],<data>
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid data URL format")
	}

	meta := parts[0]
	rawData := parts[1]

	if !strings.HasPrefix(meta, "data:") {
		return "", fmt.Errorf("invalid data URL prefix")
	}
	meta = strings.TrimPrefix(meta, "data:")

	semi := strings.Index(meta, ";")
	if semi == -1 || !strings.Contains(meta[semi:], "base64") {
		return "", fmt.Errorf("data URL must be base64 encoded")
	}

	mimeType := strings.ToLower(meta[:semi])

	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if expectImage && !isSupportedImageMime(mimeType) {
		return "", fmt.Errorf("unsupported image type: %s", mimeType)
	}

	// Pre-decode size check to prevent Base64 bomb attacks
	// Base64 encoding increases size by ~33%, so decoded size ≈ len(rawData) * 3/4
	estimatedSize := int64(base64.StdEncoding.DecodedLen(len(rawData)))
	if cfg.ImageSizeLimit > 0 && estimatedSize > cfg.ImageSizeLimit {
		return "", fmt.Errorf("estimated image size %d exceeds limit %d", estimatedSize, cfg.ImageSizeLimit)
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(rawData)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	// Check actual size limit (double-check after decode)
	if cfg.ImageSizeLimit > 0 && int64(len(decoded)) > cfg.ImageSizeLimit {
		return "", fmt.Errorf("image size %d exceeds limit %d", len(decoded), cfg.ImageSizeLimit)
	}

	// Verify actual content type from magic bytes
	detectedMime := http.DetectContentType(decoded)
	if expectImage && !isSupportedImageMime(detectedMime) {
		return "", fmt.Errorf("detected mime type not supported: %s", detectedMime)
	}
	if !expectImage && (mimeType == "application/octet-stream" || mimeType == "") {
		mimeType = strings.ToLower(detectedMime)
	}

	// Apply compression if enabled (matches normal upload flow)
	var finalData []byte
	var finalMime string
	var isAnimated bool

	if expectImage && cfg.ImageCompress && shouldCompressImage(mimeType) {
		compressed, compMime, ok, animated, compErr := tryCompressImageData(decoded, mimeType, cfg.ImageCompressQuality)
		if compErr != nil {
			return "", fmt.Errorf("compression failed: %w", compErr)
		}
		if ok && len(compressed) > 0 {
			finalData = compressed
			finalMime = compMime
			isAnimated = animated
		} else {
			finalData = decoded
			finalMime = mimeType
			isAnimated = animated
		}
	} else {
		finalData = decoded
		finalMime = mimeType
	}

	// Calculate hash
	hash, err := blake2sHash(finalData)
	if err != nil {
		return "", fmt.Errorf("hash calculation failed: %w", err)
	}
	size := int64(len(finalData))
	if existing, err := model.AttachmentFindByHashAndSize(hash, size); err != nil {
		return "", fmt.Errorf("attachment lookup failed: %w", err)
	} else if existing != nil {
		return existing.ID, nil
	}

	// Write to temp file
	tempFile, err := afero.TempFile(appFs, tmpDir, "*.upload")
	if err != nil {
		return "", fmt.Errorf("temp file creation failed: %w", err)
	}
	tempPath := tempFile.Name()

	_, err = io.Copy(tempFile, bytes.NewReader(finalData))
	_ = tempFile.Close()
	if err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("temp file write failed: %w", err)
	}

	// Persist to storage
	location, err := PersistAttachmentFile(hash, size, tempPath, finalMime)
	if err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("storage persist failed: %w", err)
	}

	// Create attachment record
	filename := generateFilename(hash, size, finalMime)
	_, newItem := model.AttachmentCreate(&model.AttachmentModel{
		Filename:    filename,
		Size:        size,
		Hash:        hash,
		MimeType:    finalMime,
		IsAnimated:  isAnimated,
		ChannelID:   channelID,
		UserID:      userID,
		StorageType: location.StorageType,
		ObjectKey:   location.ObjectKey,
		ExternalURL: location.ExternalURL,
	})

	return newItem.ID, nil
}

func isSupportedImageMime(mime string) bool {
	switch strings.ToLower(mime) {
	case "image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func shouldCompressImage(mimeType string) bool {
	switch mimeType {
	// Skip webp - already compressed, avoid quality degradation
	case "image/jpeg", "image/jpg", "image/png", "image/gif":
		return true
	default:
		return false
	}
}

// Image decompression bomb limits
const (
	maxImagePixels    = 100 * 1024 * 1024 // 100 megapixels max
	maxGIFFrames      = 500               // Max GIF frames
	maxImageDimension = 16384             // Max width or height
)

func tryCompressImageData(data []byte, mimeType string, quality int) ([]byte, string, bool, bool, error) {
	// First, decode config to check dimensions without full decode
	cfg, format, cfgErr := image.DecodeConfig(bytes.NewReader(data))
	if cfgErr != nil {
		return nil, mimeType, false, false, nil
	}

	// Check for decompression bomb: dimension and pixel limits
	if cfg.Width > maxImageDimension || cfg.Height > maxImageDimension {
		return nil, mimeType, false, false, fmt.Errorf("image dimensions %dx%d exceed limit %d", cfg.Width, cfg.Height, maxImageDimension)
	}
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if pixels > maxImagePixels {
		return nil, mimeType, false, false, fmt.Errorf("image pixels %d exceed limit %d", pixels, maxImagePixels)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, mimeType, false, false, nil
	}

	quality = clampQuality(quality)

	// For GIF, check if animated (multiple frames)
	if format == "gif" {
		gifImg, gifErr := gif.DecodeAll(bytes.NewReader(data))
		if gifErr != nil {
			return nil, mimeType, false, false, nil
		}

		// Check GIF frame limit
		if len(gifImg.Image) > maxGIFFrames {
			return nil, mimeType, false, false, fmt.Errorf("GIF frames %d exceed limit %d", len(gifImg.Image), maxGIFFrames)
		}

		// Multi-frame GIF: use gif2webp to preserve animation
		if len(gifImg.Image) > 1 {
			result, encodeErr := utils.EncodeGIFToWebPWithGIF2WebP(data, quality)
			if encodeErr != nil {
				return nil, mimeType, false, false, encodeErr
			}
			// If animated WebP is significantly larger, keep original GIF
			if len(result) > len(data)*3/2 {
				return nil, mimeType, false, true, nil
			}
			return result, "image/webp", true, true, nil
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
	// If WebP result is significantly larger (>150%), fall back to original
	if len(result) > len(data)*3/2 {
		return nil, mimeType, false, false, nil
	}
	return result, "image/webp", true, false, nil
}

func blake2sHash(data []byte) ([]byte, error) {
	h, err := blake2s.New256(nil)
	if err != nil {
		return nil, err
	}
	h.Write(data)
	return h.Sum(nil), nil
}

func generateFilename(hash []byte, size int64, mimeType string) string {
	hexHash := hex.EncodeToString(hash)
	ext := ".bin"
	switch mimeType {
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}
	return fmt.Sprintf("%s_%d%s", hexHash[:16], time.Now().UnixMilli(), ext)
}

func isHexString(value string) bool {
	if len(value) != assetIDLength {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	"golang.org/x/crypto/blake2s"
	_ "golang.org/x/image/webp"

	"sealchat/model"
	"sealchat/service/storage"
	"sealchat/utils"
)

// MigrationStats holds statistics about the image migration process
type MigrationStats struct {
	Total       int64 `json:"total"`       // Total images scanned
	Pending     int64 `json:"pending"`     // Images pending migration
	Completed   int64 `json:"completed"`   // Successfully migrated
	Failed      int64 `json:"failed"`      // Failed migrations
	Skipped     int64 `json:"skipped"`     // Skipped (GIF, S3, already webp, etc.)
	SpaceSaved  int64 `json:"spaceSaved"`  // Bytes saved
	OriginalSum int64 `json:"originalSum"` // Original total size
	NewSum      int64 `json:"newSum"`      // New total size
}

// MigrationItemResult represents the result of migrating a single image
type MigrationItemResult struct {
	ID           string `json:"id"`
	Filename     string `json:"filename"`
	OriginalSize int64  `json:"originalSize"`
	NewSize      int64  `json:"newSize"`
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
	Skipped      bool   `json:"skipped"`
	SkipReason   string `json:"skipReason,omitempty"`
}

// imageMigrator handles the image migration process
type imageMigrator struct {
	quality int
	mu      sync.Mutex
	stats   MigrationStats
}

var defaultMigrator = &imageMigrator{
	quality: 85,
}

// GetMigrationPreview returns statistics about images pending migration
func GetMigrationPreview() (*MigrationStats, error) {
	cfg := utils.GetConfig()
	if cfg != nil && cfg.ImageCompressQuality > 0 {
		defaultMigrator.quality = cfg.ImageCompressQuality
	}

	db := model.GetDB()
	stats := &MigrationStats{}

	// Count total attachments that are images
	var total int64
	if err := db.Model(&model.AttachmentModel{}).
		Where("filename LIKE ? OR filename LIKE ? OR filename LIKE ? OR filename LIKE ?",
			"%.jpg", "%.jpeg", "%.png", "%.webp").
		Count(&total).Error; err != nil {
		return nil, err
	}
	stats.Total = total

	// Count pending (non-webp, local storage, not GIF)
	var pending int64
	if err := db.Model(&model.AttachmentModel{}).
		Where("storage_type = ? OR storage_type = ?", "local", "").
		Where("filename NOT LIKE ?", "%.webp").
		Where("filename NOT LIKE ?", "%.gif").
		Where("filename LIKE ? OR filename LIKE ? OR filename LIKE ?",
			"%.jpg", "%.jpeg", "%.png").
		Count(&pending).Error; err != nil {
		return nil, err
	}
	stats.Pending = pending

	return stats, nil
}

// MigrateImages performs the actual image migration
// batchSize: number of images to process in this batch (0 = all)
// dryRun: if true, only simulate without making changes
func MigrateImages(batchSize int, dryRun bool) (*MigrationStats, []MigrationItemResult, error) {
	cfg := utils.GetConfig()
	if cfg != nil && cfg.ImageCompressQuality > 0 {
		defaultMigrator.quality = cfg.ImageCompressQuality
	}

	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}

	db := model.GetDB()

	// Find pending images
	var attachments []*model.AttachmentModel
	if err := db.
		Where("storage_type = ? OR storage_type = ?", "local", "").
		Where("filename NOT LIKE ?", "%.webp").
		Where("filename NOT LIKE ?", "%.gif").
		Where("filename LIKE ? OR filename LIKE ? OR filename LIKE ?",
			"%.jpg", "%.jpeg", "%.png").
		Limit(batchSize).
		Find(&attachments).Error; err != nil {
		return nil, nil, err
	}

	stats := &MigrationStats{
		Total:   int64(len(attachments)),
		Pending: int64(len(attachments)),
	}

	results := make([]MigrationItemResult, 0, len(attachments))

	for _, att := range attachments {
		result := migrateOneImage(att, dryRun, cfg)
		results = append(results, result)

		if result.Skipped {
			stats.Skipped++
		} else if result.Success {
			stats.Completed++
			stats.OriginalSum += result.OriginalSize
			stats.NewSum += result.NewSize
			stats.SpaceSaved += result.OriginalSize - result.NewSize
		} else {
			stats.Failed++
		}
	}

	return stats, results, nil
}

// migrateOneImage migrates a single image to WebP format
func migrateOneImage(att *model.AttachmentModel, dryRun bool, cfg *utils.AppConfig) MigrationItemResult {
	result := MigrationItemResult{
		ID:           att.ID,
		Filename:     att.Filename,
		OriginalSize: att.Size,
	}

	// Skip S3 storage
	if att.StorageType == model.StorageS3 {
		result.Skipped = true
		result.SkipReason = "S3 storage not supported"
		return result
	}

	// Skip GIF (already filtered in query, but double-check)
	lower := strings.ToLower(att.Filename)
	if strings.HasSuffix(lower, ".gif") {
		result.Skipped = true
		result.SkipReason = "GIF files are skipped"
		return result
	}

	// Skip already WebP
	if strings.HasSuffix(lower, ".webp") {
		result.Skipped = true
		result.SkipReason = "Already WebP format"
		return result
	}

	// Resolve file path
	filePath, err := resolveAttachmentFilePath(att, cfg)
	if err != nil {
		result.Error = fmt.Sprintf("Cannot resolve path: %v", err)
		return result
	}

	// Read original file
	data, err := os.ReadFile(filePath)
	if err != nil {
		result.Error = fmt.Sprintf("Cannot read file: %v", err)
		return result
	}

	// Detect MIME type
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		result.Skipped = true
		result.SkipReason = "Not an image file"
		return result
	}

	// Skip GIF by MIME
	if mimeType == "image/gif" {
		result.Skipped = true
		result.SkipReason = "GIF files are skipped"
		return result
	}

	// Compress to WebP
	quality := 85
	if cfg != nil && cfg.ImageCompressQuality > 0 {
		quality = cfg.ImageCompressQuality
	}

	compressed, ok, compErr := compressImageToWebP(data, quality)
	if compErr != nil {
		result.Error = fmt.Sprintf("Compression failed: %v", compErr)
		return result
	}

	if !ok || len(compressed) == 0 {
		result.Skipped = true
		result.SkipReason = "Compression did not reduce size"
		return result
	}

	result.NewSize = int64(len(compressed))

	// If dry run, return early
	if dryRun {
		result.Success = true
		return result
	}

	// Compute new hash
	hasher := lo.Must(blake2s.New256(nil))
	hasher.Write(compressed)
	newHash := hasher.Sum(nil)

	// Save new file
	manager := GetStorageManager()
	if manager == nil {
		result.Error = "Storage manager not initialized"
		return result
	}

	// Create temp file for new content
	tempDir := "./data/temp"
	if cfg != nil && cfg.Storage.Local.TempDir != "" {
		tempDir = cfg.Storage.Local.TempDir
	}
	_ = os.MkdirAll(tempDir, 0755)

	tempPath := filepath.Join(tempDir, fmt.Sprintf("migrate_%s_%d", att.ID, time.Now().UnixNano()))
	if err := os.WriteFile(tempPath, compressed, 0644); err != nil {
		result.Error = fmt.Sprintf("Cannot write temp file: %v", err)
		return result
	}
	defer os.Remove(tempPath)

	// Build new object key
	newObjectKey := storage.BuildAttachmentObjectKey(hex.EncodeToString(newHash), int64(len(compressed)), time.Now())

	// Upload new file
	ctx := context.Background()
	uploadResult, err := manager.UploadAttachment(ctx, storage.UploadInput{
		ObjectKey:   newObjectKey,
		LocalPath:   tempPath,
		ContentType: "image/webp",
	})
	if err != nil {
		result.Error = fmt.Sprintf("Cannot upload new file: %v", err)
		return result
	}

	// Update database record
	oldObjectKey := att.ObjectKey
	newFilename := replaceExtension(att.Filename, ".webp")

	updates := map[string]interface{}{
		"hash":       newHash,
		"size":       int64(len(compressed)),
		"object_key": uploadResult.ObjectKey,
		"filename":   newFilename,
	}

	if err := model.GetDB().Model(att).Updates(updates).Error; err != nil {
		result.Error = fmt.Sprintf("Cannot update database: %v", err)
		return result
	}

	// Delete old file
	if oldObjectKey != "" && oldObjectKey != newObjectKey {
		oldPath, _ := manager.ResolveLocalPath(oldObjectKey)
		if oldPath != "" {
			_ = os.Remove(oldPath)
		}
	}

	result.Success = true
	return result
}

// compressImageToWebP compresses image data to WebP format
func compressImageToWebP(data []byte, quality int) ([]byte, bool, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false, nil
	}

	// Skip GIF format
	if format == "gif" {
		return nil, false, nil
	}

	quality = clampQuality(quality)
	result, encodeErr := utils.EncodeImageToWebPWithCWebP(img, quality)
	if encodeErr != nil {
		return nil, false, encodeErr
	}
	// Fall back if WebP is significantly larger (>150%)
	if len(result) > len(data)*3/2 {
		return nil, false, nil
	}

	return result, true, nil
}

// clampQuality ensures quality is within valid range
func clampQuality(val int) int {
	switch {
	case val < 1:
		return 85
	case val > 100:
		return 100
	default:
		return val
	}
}

// resolveAttachmentFilePath resolves the local file path for an attachment
func resolveAttachmentFilePath(att *model.AttachmentModel, cfg *utils.AppConfig) (string, error) {
	if att == nil {
		return "", errors.New("nil attachment")
	}

	// Try resolving via object key first
	if strings.TrimSpace(att.ObjectKey) != "" {
		if path, err := ResolveLocalAttachmentPath(att.ObjectKey); err == nil {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	// Fall back to legacy hash_size naming
	if len(att.Hash) > 0 {
		uploadRoot := "./data/upload"
		if cfg != nil && cfg.Storage.Local.UploadDir != "" {
			uploadRoot = cfg.Storage.Local.UploadDir
		}

		fileName := fmt.Sprintf("%s_%d", hex.EncodeToString(att.Hash), att.Size)
		fullPath := filepath.Join(uploadRoot, fileName)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	return "", errors.New("file not found")
}

// replaceExtension replaces the file extension with a new one
func replaceExtension(filename, newExt string) string {
	ext := filepath.Ext(filename)
	if ext == "" {
		return filename + newExt
	}
	return filename[:len(filename)-len(ext)] + newExt
}

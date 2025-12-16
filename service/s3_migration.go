package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/service/storage"
	"sealchat/utils"
)

type S3MigrationKind string

const (
	S3MigrationKindImages S3MigrationKind = "images"
	S3MigrationKindAudio  S3MigrationKind = "audio"
)

var (
	ErrS3MigrationBadRequest = errors.New("s3 migration bad request")
	ErrS3MigrationS3NotReady = errors.New("s3 not ready")
)

type S3MigrationStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
	Skipped   int64 `json:"skipped"`
}

type S3MigrationItemResult struct {
	Kind        S3MigrationKind `json:"kind"`
	PrimaryID   string          `json:"primaryId"`
	RecordCount int             `json:"recordCount"`
	ObjectKey   string          `json:"objectKey"`
	Success     bool            `json:"success"`
	Skipped     bool            `json:"skipped"`
	SkipReason  string          `json:"skipReason,omitempty"`
	Error       string          `json:"error,omitempty"`
}

func GetS3MigrationPreview(kind S3MigrationKind) (*S3MigrationStats, error) {
	db := model.GetDB()
	if db == nil {
		return nil, errors.New("数据库未初始化")
	}
	stats := &S3MigrationStats{}
	var pending int64

	switch kind {
	case S3MigrationKindImages:
		if err := db.Model(&model.AttachmentModel{}).
			Where("storage_type = ? OR storage_type = ?", "local", "").
			Where("filename NOT LIKE ?", "%.gif").
			Where("filename LIKE ? OR filename LIKE ? OR filename LIKE ? OR filename LIKE ?",
				"%.jpg", "%.jpeg", "%.png", "%.webp").
			Count(&pending).Error; err != nil {
			return nil, err
		}
		stats.Pending = pending
		stats.Total = pending
	case S3MigrationKindAudio:
		if err := db.Model(&model.AudioAsset{}).
			Where("storage_type = ? OR storage_type = ?", "local", "").
			Count(&pending).Error; err != nil {
			return nil, err
		}
		stats.Pending = pending
		stats.Total = pending
	default:
		return nil, fmt.Errorf("%w: unsupported type %q", ErrS3MigrationBadRequest, kind)
	}

	return stats, nil
}

func ExecuteS3Migration(kind S3MigrationKind, batchSize int, dryRun bool, deleteSource bool) (*S3MigrationStats, []S3MigrationItemResult, error) {
	db := model.GetDB()
	if db == nil {
		return nil, nil, errors.New("数据库未初始化")
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if batchSize > 1000 {
		batchSize = 1000
	}
	manager := GetStorageManager()
	if manager == nil {
		return nil, nil, fmt.Errorf("%w: storage manager not initialized", ErrS3MigrationS3NotReady)
	}
	if !manager.HasRemote() {
		if initErr := manager.RemoteInitError(); initErr != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrS3MigrationS3NotReady, initErr)
		}
		return nil, nil, fmt.Errorf("%w: S3 未启用或初始化失败", ErrS3MigrationS3NotReady)
	}

	switch kind {
	case S3MigrationKindImages:
		return executeS3ImageMigration(db, manager, batchSize, dryRun, deleteSource)
	case S3MigrationKindAudio:
		return executeS3AudioMigration(db, manager, batchSize, dryRun, deleteSource)
	default:
		return nil, nil, fmt.Errorf("%w: unsupported type %q", ErrS3MigrationBadRequest, kind)
	}
}

func executeS3ImageMigration(db *gorm.DB, manager *storage.Manager, batchSize int, dryRun bool, deleteSource bool) (*S3MigrationStats, []S3MigrationItemResult, error) {
	var attachments []*model.AttachmentModel
	if err := db.
		Where("storage_type = ? OR storage_type = ?", "local", "").
		Where("filename NOT LIKE ?", "%.gif").
		Where("filename LIKE ? OR filename LIKE ? OR filename LIKE ? OR filename LIKE ?",
			"%.jpg", "%.jpeg", "%.png", "%.webp").
		Order("created_at ASC").
		Limit(batchSize).
		Find(&attachments).Error; err != nil {
		return nil, nil, err
	}

	stats := &S3MigrationStats{
		Total:   int64(len(attachments)),
		Pending: int64(len(attachments)),
	}
	results := make([]S3MigrationItemResult, 0, len(attachments))
	processed := map[string]struct{}{}

	cfg := utils.GetConfig()
	ctx := context.Background()

	for _, att := range attachments {
		groupKey := attachmentGroupKey(att)
		if groupKey == "" {
			results = append(results, S3MigrationItemResult{
				Kind:       S3MigrationKindImages,
				PrimaryID:  att.ID,
				Skipped:    true,
				SkipReason: "无法确定分组键",
			})
			stats.Skipped++
			continue
		}
		if _, ok := processed[groupKey]; ok {
			continue
		}
		processed[groupKey] = struct{}{}

		group, err := loadAttachmentGroup(db, att)
		if err != nil {
			results = append(results, S3MigrationItemResult{
				Kind:      S3MigrationKindImages,
				PrimaryID: att.ID,
				Error:     err.Error(),
			})
			stats.Failed++
			continue
		}
		result := migrateAttachmentGroupToS3(ctx, db, manager, group, cfg, dryRun, deleteSource)
		results = append(results, result)
		switch {
		case result.Skipped:
			stats.Skipped++
		case result.Success:
			stats.Completed++
		default:
			stats.Failed++
		}
	}

	return stats, results, nil
}

func attachmentGroupKey(att *model.AttachmentModel) string {
	if att == nil {
		return ""
	}
	if strings.TrimSpace(att.ObjectKey) != "" {
		return "ok:" + strings.TrimSpace(att.ObjectKey)
	}
	if len(att.Hash) > 0 && att.Size > 0 {
		return "hs:" + hex.EncodeToString(att.Hash) + fmt.Sprintf("_%d", att.Size)
	}
	return ""
}

func loadAttachmentGroup(db *gorm.DB, att *model.AttachmentModel) ([]*model.AttachmentModel, error) {
	if db == nil || att == nil {
		return nil, errors.New("invalid input")
	}
	var group []*model.AttachmentModel
	if strings.TrimSpace(att.ObjectKey) != "" {
		if err := db.
			Where("storage_type = ? OR storage_type = ?", "local", "").
			Where("object_key = ?", att.ObjectKey).
			Order("created_at ASC").
			Find(&group).Error; err != nil {
			return nil, err
		}
		return group, nil
	}
	if len(att.Hash) == 0 || att.Size <= 0 {
		return nil, errors.New("missing hash/size")
	}
	if err := db.
		Where("storage_type = ? OR storage_type = ?", "local", "").
		Where("hash = ? AND size = ?", []byte(att.Hash), att.Size).
		Order("created_at ASC").
		Find(&group).Error; err != nil {
		return nil, err
	}
	return group, nil
}

func migrateAttachmentGroupToS3(ctx context.Context, db *gorm.DB, manager *storage.Manager, group []*model.AttachmentModel, cfg *utils.AppConfig, dryRun bool, deleteSource bool) S3MigrationItemResult {
	primary := firstNonNilAttachment(group)
	if primary == nil {
		return S3MigrationItemResult{Kind: S3MigrationKindImages, Skipped: true, SkipReason: "empty group"}
	}
	result := S3MigrationItemResult{
		Kind:        S3MigrationKindImages,
		PrimaryID:   primary.ID,
		RecordCount: len(group),
	}
	if primary.StorageType == model.StorageS3 {
		result.Skipped = true
		result.SkipReason = "already s3"
		return result
	}
	lower := strings.ToLower(strings.TrimSpace(primary.Filename))
	if strings.HasSuffix(lower, ".gif") {
		result.Skipped = true
		result.SkipReason = "gif skipped"
		return result
	}
	if !isImageFilename(lower) {
		result.Skipped = true
		result.SkipReason = "not image"
		return result
	}

	localPath, err := resolveLocalAttachmentPathForS3Migration(primary, cfg)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	objectKey := chooseAttachmentObjectKey(primary)
	if objectKey == "" {
		result.Error = "cannot determine objectKey"
		return result
	}
	result.ObjectKey = objectKey

	if dryRun {
		result.Success = true
		return result
	}

	uploadResult, err := manager.UploadToS3(ctx, storage.UploadInput{
		ObjectKey:   objectKey,
		LocalPath:   localPath,
		ContentType: guessContentTypeFromFilename(primary.Filename),
	})
	if err != nil {
		result.Error = fmt.Sprintf("upload failed: %v", err)
		return result
	}
	ok, err := manager.Exists(ctx, storage.BackendS3, objectKey)
	if err != nil || !ok {
		if err == nil {
			err = errors.New("stat object failed")
		}
		result.Error = fmt.Sprintf("verify failed: %v", err)
		return result
	}
	if err := verifyHTTPAccessible(uploadResult.PublicURL); err != nil {
		result.Error = fmt.Sprintf("not accessible: %v", err)
		return result
	}

	updates := map[string]interface{}{
		"storage_type": model.StorageS3,
		"object_key":   objectKey,
		"external_url": strings.TrimSpace(uploadResult.PublicURL),
	}
	ids := make([]string, 0, len(group))
	for _, item := range group {
		if item == nil || item.ID == "" {
			continue
		}
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		result.Error = "no records to update"
		return result
	}
	if err := db.Model(&model.AttachmentModel{}).Where("id IN (?)", ids).Updates(updates).Error; err != nil {
		result.Error = fmt.Sprintf("db update failed: %v", err)
		return result
	}

	if deleteSource {
		_ = os.Remove(localPath)
	}
	result.Success = true
	return result
}

func chooseAttachmentObjectKey(att *model.AttachmentModel) string {
	if att == nil {
		return ""
	}
	if key := strings.TrimSpace(att.ObjectKey); key != "" && strings.HasPrefix(key, "attachments/") {
		return key
	}
	if len(att.Hash) == 0 {
		return ""
	}
	t := time.Now()
	if !att.CreatedAt.IsZero() {
		t = att.CreatedAt
	}
	return storage.BuildAttachmentObjectKey(hex.EncodeToString(att.Hash), att.Size, t)
}

func resolveLocalAttachmentPathForS3Migration(att *model.AttachmentModel, cfg *utils.AppConfig) (string, error) {
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

func firstNonNilAttachment(list []*model.AttachmentModel) *model.AttachmentModel {
	for _, item := range list {
		if item != nil {
			return item
		}
	}
	return nil
}

func isImageFilename(lower string) bool {
	return strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".png") ||
		strings.HasSuffix(lower, ".webp")
}

func executeS3AudioMigration(db *gorm.DB, manager *storage.Manager, batchSize int, dryRun bool, deleteSource bool) (*S3MigrationStats, []S3MigrationItemResult, error) {
	var assets []*model.AudioAsset
	if err := db.
		Where("storage_type = ? OR storage_type = ?", "local", "").
		Order("created_at ASC").
		Limit(batchSize).
		Find(&assets).Error; err != nil {
		return nil, nil, err
	}
	stats := &S3MigrationStats{
		Total:   int64(len(assets)),
		Pending: int64(len(assets)),
	}
	results := make([]S3MigrationItemResult, 0, len(assets))
	cfg := utils.GetConfig()
	ctx := context.Background()

	for _, asset := range assets {
		r := migrateOneAudioAssetToS3(ctx, db, manager, asset, cfg, dryRun, deleteSource)
		results = append(results, r)
		switch {
		case r.Skipped:
			stats.Skipped++
		case r.Success:
			stats.Completed++
		default:
			stats.Failed++
		}
	}
	return stats, results, nil
}

func migrateOneAudioAssetToS3(ctx context.Context, db *gorm.DB, manager *storage.Manager, asset *model.AudioAsset, cfg *utils.AppConfig, dryRun bool, deleteSource bool) S3MigrationItemResult {
	if asset == nil {
		return S3MigrationItemResult{Kind: S3MigrationKindAudio, Skipped: true, SkipReason: "nil asset"}
	}
	result := S3MigrationItemResult{
		Kind:        S3MigrationKindAudio,
		PrimaryID:   asset.ID,
		RecordCount: 1,
	}
	if asset.StorageType == model.StorageS3 {
		result.Skipped = true
		result.SkipReason = "already s3"
		return result
	}
	localPath, err := resolveLocalAudioPath(cfg, asset.ObjectKey)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	destKey := buildAudioDestObjectKey(asset.ID, asset.ObjectKey)
	if destKey == "" {
		result.Error = "cannot determine objectKey"
		return result
	}
	result.ObjectKey = destKey

	if dryRun {
		result.Success = true
		return result
	}

	uploadResult, err := manager.UploadToS3(ctx, storage.UploadInput{
		ObjectKey:   destKey,
		LocalPath:   localPath,
		ContentType: guessContentTypeFromFilename(asset.ObjectKey),
	})
	if err != nil {
		result.Error = fmt.Sprintf("upload failed: %v", err)
		return result
	}
	ok, err := manager.Exists(ctx, storage.BackendS3, destKey)
	if err != nil || !ok {
		if err == nil {
			err = errors.New("stat object failed")
		}
		result.Error = fmt.Sprintf("verify failed: %v", err)
		return result
	}
	if err := verifyHTTPAccessible(uploadResult.PublicURL); err != nil {
		result.Error = fmt.Sprintf("not accessible: %v", err)
		return result
	}

	updates := map[string]interface{}{
		"storage_type": model.StorageS3,
		"object_key":   destKey,
		"variants":     model.JSONList[model.AudioAssetVariant]([]model.AudioAssetVariant{}),
	}
	if err := db.Model(&model.AudioAsset{}).Where("id = ?", asset.ID).Updates(updates).Error; err != nil {
		result.Error = fmt.Sprintf("db update failed: %v", err)
		return result
	}

	if deleteSource {
		deleteLocalAudioFiles(cfg, asset)
	}
	result.Success = true
	return result
}

func resolveLocalAudioPath(cfg *utils.AppConfig, objectKey string) (string, error) {
	root := "./static/audio"
	if cfg != nil && strings.TrimSpace(cfg.Audio.StorageDir) != "" {
		root = cfg.Audio.StorageDir
	}
	clean := filepath.Clean(strings.TrimSpace(objectKey))
	if clean == "" || strings.HasPrefix(clean, "..") {
		return "", errors.New("invalid objectKey")
	}
	full := filepath.Join(filepath.Clean(root), clean)
	if _, err := os.Stat(full); err != nil {
		return "", err
	}
	return full, nil
}

func deleteLocalAudioFiles(cfg *utils.AppConfig, asset *model.AudioAsset) {
	if asset == nil {
		return
	}
	paths := []string{asset.ObjectKey}
	for _, v := range asset.Variants {
		if v.StorageType == model.StorageLocal && strings.TrimSpace(v.ObjectKey) != "" {
			paths = append(paths, v.ObjectKey)
		}
	}
	for _, p := range paths {
		if full, err := resolveLocalAudioPath(cfg, p); err == nil {
			_ = os.Remove(full)
		}
	}
}

func buildAudioDestObjectKey(assetID string, existingObjectKey string) string {
	id := strings.TrimSpace(assetID)
	if id == "" {
		return ""
	}
	name := filepath.Base(strings.TrimSpace(existingObjectKey))
	if name == "." || name == "/" || name == "" {
		name = "audio.ogg"
	}
	name = strings.TrimSpace(name)
	if ext := filepath.Ext(name); ext == "" {
		name = name + ".ogg"
	}
	return storage.BuildAudioObjectKey(id, name)
}

func guessContentTypeFromFilename(name string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	if ext == "" {
		return ""
	}
	return mime.TypeByExtension(ext)
}

func verifyHTTPAccessible(target string) error {
	url := strings.TrimSpace(target)
	if !strings.HasPrefix(strings.ToLower(url), "http://") && !strings.HasPrefix(strings.ToLower(url), "https://") {
		return fmt.Errorf("missing url")
	}

	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode < 400 {
			return nil
		}
		if resp.StatusCode != http.StatusMethodNotAllowed {
			return fmt.Errorf("status %s", resp.Status)
		}
	}

	req, err = http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err = client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 400 || resp.StatusCode == http.StatusPartialContent {
		return nil
	}
	return fmt.Errorf("status %s", resp.Status)
}

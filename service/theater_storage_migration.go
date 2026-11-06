package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/service/storage"
)

type StorageMigrationTarget string

const (
	StorageMigrationTargetLocal StorageMigrationTarget = "local"
	StorageMigrationTargetS3    StorageMigrationTarget = "s3"
)

var storageMigrationMu sync.Mutex
var ErrStorageMigrationRunning = errors.New("已有存储迁移正在执行，请稍后重试")

func beginStorageMigration() (func(), error) {
	if !storageMigrationMu.TryLock() {
		return nil, ErrStorageMigrationRunning
	}
	if err := storageMigrationConfigCheck(); err != nil {
		storageMigrationMu.Unlock()
		return nil, err
	}
	return storageMigrationMu.Unlock, nil
}

func GetStorageMigrationPreview(kind S3MigrationKind, target StorageMigrationTarget) (*S3MigrationStats, error) {
	if err := storageMigrationConfigCheck(); err != nil {
		return nil, err
	}
	if target == "" {
		target = StorageMigrationTargetS3
	}
	if kind == S3MigrationKindFonts {
		return getPlatformFontStorageMigrationPreview(target)
	}
	if kind == S3MigrationKindAudio {
		backend, err := storageMigrationBackend(target)
		if err != nil {
			return nil, err
		}
		return audioStorageMigrationPreview(model.GetDB(), backend)
	}
	if kind != S3MigrationKindTheater && kind != S3MigrationKindTTS {
		if target != StorageMigrationTargetS3 {
			return nil, fmt.Errorf("%w: %s 暂不支持迁移到本地存储", ErrS3MigrationBadRequest, kind)
		}
		return GetS3MigrationPreview(kind)
	}
	source, err := storageMigrationSource(target)
	if err != nil {
		return nil, err
	}
	stats := &S3MigrationStats{}
	query := applyStorageSourceFilter(theaterAttachmentScope(model.GetDB()), source)
	if kind == S3MigrationKindTTS {
		query = applyStorageSourceFilter(ttsAttachmentScope(model.GetDB()), source)
	}
	if err := query.Count(&stats.Pending).Error; err != nil {
		return nil, err
	}
	stats.Total = stats.Pending
	return stats, nil
}

func ExecuteStorageMigration(kind S3MigrationKind, target StorageMigrationTarget, batchSize int, dryRun bool, deleteSource bool) (*S3MigrationStats, []S3MigrationItemResult, error) {
	unlock, err := beginStorageMigration()
	if err != nil {
		return nil, nil, err
	}
	defer unlock()
	return executeStorageMigration(kind, target, batchSize, dryRun, deleteSource)
}

func executeStorageMigration(kind S3MigrationKind, target StorageMigrationTarget, batchSize int, dryRun bool, deleteSource bool) (*S3MigrationStats, []S3MigrationItemResult, error) {
	if target == "" {
		target = StorageMigrationTargetS3
	}
	if kind == S3MigrationKindFonts {
		return executePlatformFontStorageMigration(target, batchSize, dryRun, deleteSource)
	}
	if kind == S3MigrationKindAudio {
		return executeAudioStorageMigration(target, batchSize, dryRun, deleteSource)
	}
	if kind != S3MigrationKindTheater && kind != S3MigrationKindTTS {
		if target != StorageMigrationTargetS3 {
			return nil, nil, fmt.Errorf("%w: %s 暂不支持迁移到本地存储", ErrS3MigrationBadRequest, kind)
		}
		return executeS3Migration(kind, batchSize, dryRun, deleteSource)
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if batchSize > 1000 {
		batchSize = 1000
	}
	manager := GetStorageManager()
	if manager == nil {
		return nil, nil, errors.New("存储服务未初始化")
	}
	targetBackend, err := storageMigrationBackend(target)
	if err != nil {
		return nil, nil, err
	}
	if targetBackend == storage.BackendS3 && !manager.HasRemote() {
		return nil, nil, fmt.Errorf("%w: S3 未启用或初始化失败", ErrS3MigrationS3NotReady)
	}
	sourceType, err := storageMigrationSource(target)
	if err != nil {
		return nil, nil, err
	}
	var candidates []*model.AttachmentModel
	query := applyStorageSourceFilter(theaterAttachmentScope(model.GetDB()), sourceType)
	if kind == S3MigrationKindTTS {
		query = applyStorageSourceFilter(ttsAttachmentScope(model.GetDB()), sourceType)
	}
	err = query.Order("created_at ASC").Limit(batchSize).Find(&candidates).Error
	if err != nil {
		return nil, nil, err
	}
	stats := &S3MigrationStats{Total: int64(len(candidates)), Pending: int64(len(candidates))}
	results := make([]S3MigrationItemResult, 0, len(candidates))
	processed := map[string]struct{}{}
	for _, candidate := range candidates {
		sourceBackend := convertModelToBackend(candidate.StorageType)
		key := string(sourceBackend) + "\x00" + attachmentGroupKey(candidate)
		if _, ok := processed[key]; ok {
			continue
		}
		processed[key] = struct{}{}
		groupDB := model.GetDB()
		if kind == S3MigrationKindTTS {
			groupDB = ttsAttachmentScope(groupDB)
		}
		group, loadErr := loadStorageAttachmentGroup(groupDB, candidate)
		if loadErr != nil {
			results = append(results, S3MigrationItemResult{Kind: kind, PrimaryID: candidate.ID, Error: loadErr.Error()})
			stats.Failed++
			continue
		}
		result := migrateAttachmentGroup(context.Background(), model.GetDB(), manager, group, targetBackend, dryRun, deleteSource, kind)
		result.Kind = kind
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

func storageMigrationSource(target StorageMigrationTarget) (model.StorageType, error) {
	switch target {
	case StorageMigrationTargetS3:
		return model.StorageLocal, nil
	case StorageMigrationTargetLocal:
		return model.StorageS3, nil
	default:
		return "", fmt.Errorf("%w: unsupported target %q", ErrS3MigrationBadRequest, target)
	}
}

func storageMigrationBackend(target StorageMigrationTarget) (storage.BackendType, error) {
	switch target {
	case StorageMigrationTargetS3:
		return storage.BackendS3, nil
	case StorageMigrationTargetLocal:
		return storage.BackendLocal, nil
	default:
		return "", fmt.Errorf("%w: unsupported target %q", ErrS3MigrationBadRequest, target)
	}
}

func applyStorageSourceFilter(query *gorm.DB, source model.StorageType) *gorm.DB {
	if source == model.StorageLocal {
		return query.Where("storage_type = ? OR storage_type = ?", model.StorageLocal, "")
	}
	return query.Where("storage_type = ?", source)
}

func theaterAttachmentScope(db *gorm.DB) *gorm.DB {
	return db.Model(&model.AttachmentModel{}).Where(
		"root_id_type LIKE ? OR root_id_type LIKE ? OR id IN (?) OR id IN (?) OR id IN (?) OR id IN (?) OR id IN (?)",
		"theater_%",
		"theater-%",
		db.Model(&model.TheaterResourceModel{}).Select("attachment_id"),
		db.Model(&model.TheaterResourceVariantModel{}).Select("attachment_id"),
		db.Model(&model.TheaterAppearanceAssetModel{}).Select("source_attachment_id"),
		db.Model(&model.TheaterAppearanceAssetModel{}).Select("display_attachment_id"),
		db.Model(&model.TheaterAppearanceAssetModel{}).Select("fallback_attachment_id"),
	)
}

func ttsAttachmentScope(db *gorm.DB) *gorm.DB {
	// The private resource prefix implies IsTTSManaged; samples and spool are excluded.
	return db.Model(&model.AttachmentModel{}).
		Where("parent_id_type = ? AND object_key LIKE ? AND deleted_at IS NULL", "job_audio", "tts-private/resources/%")
}

func loadStorageAttachmentGroup(db *gorm.DB, attachment *model.AttachmentModel) ([]*model.AttachmentModel, error) {
	if db == nil || attachment == nil {
		return nil, errors.New("invalid input")
	}
	sourceType := attachment.StorageType
	if sourceType == "" {
		sourceType = model.StorageLocal
	}
	query := applyStorageSourceFilter(db, sourceType)
	if strings.TrimSpace(attachment.ObjectKey) != "" {
		query = query.Where("object_key = ?", attachment.ObjectKey)
	} else if len(attachment.Hash) > 0 && attachment.Size > 0 {
		query = query.Where("hash = ? AND size = ?", []byte(attachment.Hash), attachment.Size)
	} else {
		return nil, errors.New("附件缺少 objectKey 和 hash")
	}
	var group []*model.AttachmentModel
	return group, query.Order("created_at ASC").Find(&group).Error
}

func migrateAttachmentGroup(ctx context.Context, db *gorm.DB, manager *storage.Manager, group []*model.AttachmentModel, target storage.BackendType, dryRun bool, deleteSource bool, kind S3MigrationKind) S3MigrationItemResult {
	primary := firstNonNilAttachment(group)
	result := S3MigrationItemResult{PrimaryID: "", RecordCount: len(group)}
	if primary == nil {
		result.Skipped = true
		result.SkipReason = "empty group"
		return result
	}
	result.PrimaryID = primary.ID
	isTTS := kind == S3MigrationKindTTS
	if isTTS && (!primary.IsTTSManaged() || primary.ParentIDType != "job_audio" || primary.DeletedAt != nil || !strings.HasPrefix(primary.ObjectKey, "tts-private/resources/")) {
		result.Skipped = true
		result.SkipReason = "not a final TTS result"
		return result
	}
	source := convertModelToBackend(primary.StorageType)
	if source == target {
		result.Skipped = true
		result.SkipReason = "already on target storage"
		return result
	}
	objectKey := strings.TrimSpace(primary.ObjectKey)
	if !isTTS && (objectKey == "" || !strings.HasPrefix(objectKey, "attachments/")) {
		createdAt := primary.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		objectKey = storage.BuildAttachmentObjectKey(hex.EncodeToString(primary.Hash), primary.Size, createdAt)
	}
	result.ObjectKey = objectKey
	if dryRun {
		result.Success = true
		return result
	}
	var tempPath string
	var err error
	if isTTS {
		// Keep the public attachment materializer's private-resource guard intact.
		var temp *os.File
		temp, err = os.CreateTemp("", "sealchat-tts-migration-*")
		if err == nil {
			tempPath = temp.Name()
			err = temp.Close()
			if err == nil {
				err = manager.DownloadToPath(ctx, source, primary.ObjectKey, tempPath)
			}
			if err != nil {
				_ = os.Remove(tempPath)
			}
		}
	} else {
		tempPath, err = MaterializeAttachmentToTempFile(primary)
	}
	if err != nil {
		result.Error = fmt.Sprintf("读取源文件失败: %v", err)
		return result
	}
	defer os.Remove(tempPath)
	targetExisted, _ := manager.Exists(ctx, target, objectKey)
	uploaded, err := manager.UploadWithBackend(ctx, target, storage.UploadInput{ObjectKey: objectKey, LocalPath: tempPath, ContentType: primary.MimeType})
	if err != nil {
		result.Error = fmt.Sprintf("写入目标存储失败: %v", err)
		return result
	}
	if err := verifyMigratedAttachment(ctx, manager, target, objectKey, primary); err != nil {
		if !targetExisted {
			_ = manager.Delete(ctx, target, objectKey)
		}
		result.Error = fmt.Sprintf("校验目标文件失败: %v", err)
		return result
	}
	ids := make([]string, 0, len(group))
	for _, attachment := range group {
		if attachment != nil && attachment.ID != "" {
			ids = append(ids, attachment.ID)
		}
	}
	externalURL := ""
	if target == storage.BackendS3 {
		externalURL = uploaded.PublicURL
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&model.AttachmentModel{}).Where("id IN ?", ids).Updates(map[string]any{
			"storage_type": convertBackendToModel(target), "object_key": objectKey, "external_url": externalURL,
		}).Error
	}); err != nil {
		if !targetExisted {
			_ = manager.Delete(ctx, target, objectKey)
		}
		result.Error = fmt.Sprintf("更新数据库失败: %v", err)
		return result
	}
	if deleteSource && strings.TrimSpace(primary.ObjectKey) != "" {
		var remaining int64
		remainingQuery := db.Model(&model.AttachmentModel{}).Where("object_key = ?", primary.ObjectKey)
		if isTTS {
			remainingQuery = applyStorageSourceFilter(remainingQuery, convertBackendToModel(source))
		} else {
			remainingQuery = remainingQuery.Where("storage_type = ?", primary.StorageType)
		}
		if err := remainingQuery.Count(&remaining).Error; err == nil && remaining == 0 {
			_ = manager.Delete(ctx, source, primary.ObjectKey)
		}
	}
	result.Success = true
	return result
}

func verifyMigratedAttachment(ctx context.Context, manager *storage.Manager, backend storage.BackendType, objectKey string, attachment *model.AttachmentModel) error {
	temp, err := os.CreateTemp("", "sealchat-theater-verify-*")
	if err != nil {
		return err
	}
	path := temp.Name()
	_ = temp.Close()
	defer os.Remove(path)
	if err := manager.DownloadToPath(ctx, backend, objectKey, path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if attachment.Size > 0 && int64(len(data)) != attachment.Size {
		return fmt.Errorf("大小不一致: got=%d want=%d", len(data), attachment.Size)
	}
	if len(attachment.Hash) == sha256.Size {
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), hex.EncodeToString(attachment.Hash)) {
			return errors.New("SHA-256 不一致")
		}
	}
	return nil
}

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/service/storage"
)

func platformFontMigrationTypes(target StorageMigrationTarget) (model.StorageType, model.StorageType, error) {
	backend, err := storageMigrationBackend(target)
	if err != nil {
		return "", "", err
	}
	if backend == storage.BackendS3 {
		return model.StorageFontLocal, model.StorageFontS3, nil
	}
	return model.StorageFontS3, model.StorageFontLocal, nil
}

func platformFontMigrationScope(db *gorm.DB, source model.StorageType) *gorm.DB {
	return db.Model(&model.PlatformFontAsset{}).Where("deleted_at IS NULL").Where(
		"(original_storage_type = ? AND original_object_key <> '') OR (subset_storage_type = ? AND subset_object_key <> '') OR (manifest_storage_type = ? AND manifest_object_key <> '')",
		source, source, source,
	)
}

// A subset key can name either one file or a directory containing the whole package.
func platformFontMigrationSubsetObjects(ctx context.Context, manager *storage.Manager, source model.StorageType, key string) ([]storage.ObjectInfo, error) {
	backend := convertFontModelToBackend(source)
	if backend == storage.BackendS3 {
		objects, err := manager.ListS3Prefix(ctx, strings.TrimRight(key, "/")+"/")
		if err != nil {
			return nil, err
		}
		files := make([]storage.ObjectInfo, 0, len(objects))
		for _, object := range objects {
			if !strings.HasSuffix(object.ObjectKey, "/") {
				files = append(files, object)
			}
		}
		if len(files) > 0 {
			return files, nil
		}
		exists, err := manager.Exists(ctx, backend, key)
		if err != nil {
			return nil, err
		}
		if exists && !strings.HasSuffix(key, "/") {
			return []storage.ObjectInfo{{ObjectKey: key}}, nil
		}
		return nil, nil
	}
	root, err := manager.ResolveLocalPath(key)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []storage.ObjectInfo{{ObjectKey: key, Size: info.Size()}}, nil
	}
	var objects []storage.ObjectInfo
	err = filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("字体分片不是普通文件: %s", filePath)
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		objects = append(objects, storage.ObjectInfo{ObjectKey: strings.TrimRight(key, "/") + "/" + filepath.ToSlash(relative)})
		return nil
	})
	return objects, err
}

// Only fields backed by existing source files are updated. Missing/empty resources
// do not turn into references to nonexistent target objects.
func platformFontMigrationObjects(ctx context.Context, manager *storage.Manager, asset *model.PlatformFontAsset, source, target model.StorageType) ([]storage.ObjectInfo, map[string]any, error) {
	objects := make([]storage.ObjectInfo, 0)
	updates := map[string]any{}
	seen := map[string]bool{}
	for _, resource := range []struct {
		column      string
		storageType model.StorageType
		key         string
		prefix      bool
	}{
		{"original_storage_type", asset.OriginalStorageType, asset.OriginalObjectKey, false},
		{"subset_storage_type", asset.SubsetStorageType, asset.SubsetObjectKey, true},
		{"manifest_storage_type", asset.ManifestStorageType, asset.ManifestObjectKey, false},
	} {
		if resource.storageType != source || strings.TrimSpace(resource.key) == "" {
			continue
		}
		var files []storage.ObjectInfo
		if resource.prefix {
			var err error
			files, err = platformFontMigrationSubsetObjects(ctx, manager, source, resource.key)
			if err != nil {
				return nil, nil, err
			}
		} else {
			exists, err := manager.Exists(ctx, convertFontModelToBackend(source), resource.key)
			if err != nil {
				return nil, nil, err
			}
			if exists {
				files = []storage.ObjectInfo{{ObjectKey: resource.key}}
			}
		}
		if len(files) == 0 {
			continue
		}
		updates[resource.column] = target
		for _, file := range files {
			if !seen[file.ObjectKey] {
				objects = append(objects, file)
				seen[file.ObjectKey] = true
			}
		}
	}
	return objects, updates, nil
}

func getPlatformFontStorageMigrationPreview(target StorageMigrationTarget) (*S3MigrationStats, error) {
	sourceType, targetType, err := platformFontMigrationTypes(target)
	if err != nil {
		return nil, err
	}
	db, manager := model.GetDB(), GetStorageManager()
	if db == nil || manager == nil {
		return nil, errors.New("字体存储服务未初始化")
	}
	stats := &S3MigrationStats{}
	var assets []*model.PlatformFontAsset
	err = platformFontMigrationScope(db, sourceType).FindInBatches(&assets, 100, func(_ *gorm.DB, _ int) error {
		for _, asset := range assets {
			objects, _, err := platformFontMigrationObjects(context.Background(), manager, asset, sourceType, targetType)
			if err != nil {
				return err
			}
			if len(objects) > 0 {
				stats.Pending++
			}
		}
		return nil
	}).Error
	stats.Total = stats.Pending
	return stats, err
}

func executePlatformFontStorageMigration(target StorageMigrationTarget, batchSize int, dryRun, deleteSource bool) (*S3MigrationStats, []S3MigrationItemResult, error) {
	sourceType, targetType, err := platformFontMigrationTypes(target)
	if err != nil {
		return nil, nil, err
	}
	db, manager := model.GetDB(), GetStorageManager()
	if db == nil || manager == nil {
		return nil, nil, errors.New("字体存储服务未初始化")
	}
	if !manager.HasRemote() {
		return nil, nil, fmt.Errorf("%w: S3 未启用或初始化失败", ErrS3MigrationS3NotReady)
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if batchSize > 1000 {
		batchSize = 1000
	}
	var assets []*model.PlatformFontAsset
	var batch []*model.PlatformFontAsset
	batchReady := errors.New("font migration batch ready")
	// Missing source files do not consume the execution batch or indefinitely
	// hide later candidates that the preview counted as pending.
	err = platformFontMigrationScope(db, sourceType).FindInBatches(&batch, batchSize, func(_ *gorm.DB, _ int) error {
		for _, asset := range batch {
			objects, _, readErr := platformFontMigrationObjects(context.Background(), manager, asset, sourceType, targetType)
			if readErr == nil && len(objects) == 0 {
				continue
			}
			assets = append(assets, asset)
			if len(assets) == batchSize {
				return batchReady
			}
		}
		return nil
	}).Error
	if err != nil && !errors.Is(err, batchReady) {
		return nil, nil, err
	}
	stats := &S3MigrationStats{Total: int64(len(assets)), Pending: int64(len(assets))}
	results := make([]S3MigrationItemResult, 0, len(assets))
	for _, asset := range assets {
		result := migratePlatformFontAsset(context.Background(), db, manager, asset, sourceType, targetType, dryRun, deleteSource)
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

func migratePlatformFontAsset(ctx context.Context, db *gorm.DB, manager *storage.Manager, asset *model.PlatformFontAsset, sourceType, targetType model.StorageType, dryRun, deleteSource bool) S3MigrationItemResult {
	result := S3MigrationItemResult{Kind: S3MigrationKindFonts, PrimaryID: asset.ID, RecordCount: 1, ObjectKey: asset.OriginalObjectKey}
	objects, updates, err := platformFontMigrationObjects(ctx, manager, asset, sourceType, targetType)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if len(objects) == 0 {
		result.Skipped, result.SkipReason = true, "no source font files"
		return result
	}
	if dryRun {
		result.Success = true
		return result
	}
	source, target := convertFontModelToBackend(sourceType), convertFontModelToBackend(targetType)
	createdKeys := []string{}
	rollback := func() {
		for _, key := range createdKeys {
			_ = manager.Delete(ctx, target, key)
		}
	}
	for _, object := range objects {
		if err := copyPlatformFontMigrationObject(ctx, manager, source, target, object.ObjectKey, &createdKeys); err != nil {
			rollback()
			result.Error = fmt.Sprintf("迁移字体文件失败: %v", err)
			return result
		}
	}
	// One conditional UPDATE commits all storage fields together, and refuses a
	// concurrently replaced package without deleting its source files.
	updated := db.Model(&model.PlatformFontAsset{}).Where("id = ? AND deleted_at IS NULL", asset.ID).
		Where("COALESCE(original_storage_type, '') = ? AND COALESCE(original_object_key, '') = ?", asset.OriginalStorageType, asset.OriginalObjectKey).
		Where("COALESCE(subset_storage_type, '') = ? AND COALESCE(subset_object_key, '') = ?", asset.SubsetStorageType, asset.SubsetObjectKey).
		Where("COALESCE(manifest_storage_type, '') = ? AND COALESCE(manifest_object_key, '') = ?", asset.ManifestStorageType, asset.ManifestObjectKey).
		Updates(updates)
	if updated.Error != nil || updated.RowsAffected != 1 {
		rollback()
		result.Error = fmt.Sprintf("更新字体数据库失败: %v", updated.Error)
		if updated.Error == nil {
			result.Error = "字体存储信息已变更"
		}
		return result
	}
	if deleteSource {
		for _, object := range objects {
			_ = DeletePlatformFontFile(sourceType, object.ObjectKey)
		}
	}
	result.Success = true
	return result
}

func copyPlatformFontMigrationObject(ctx context.Context, manager *storage.Manager, source, target storage.BackendType, key string, createdKeys *[]string) error {
	temp, err := os.CreateTemp("", "sealchat-font-migration-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Close(); err != nil {
		return err
	}
	if err := manager.DownloadToPath(ctx, source, key, tempPath); err != nil {
		return err
	}
	file, err := os.Open(tempPath)
	if err != nil {
		return err
	}
	digest := sha256.New()
	size, readErr := io.Copy(digest, file)
	closeErr := file.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	existed, err := manager.Exists(ctx, target, key)
	if err != nil {
		return err
	}
	if existed {
		reader, err := manager.OpenRead(ctx, target, key)
		if err != nil {
			return err
		}
		targetDigest := sha256.New()
		targetSize, readErr := io.Copy(targetDigest, reader)
		closeErr := reader.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if targetSize != size || !bytes.Equal(digest.Sum(nil), targetDigest.Sum(nil)) {
			return errors.New("字体目标对象已存在且内容不同")
		}
		return nil
	}
	if _, err := uploadPlatformFontByBackend(ctx, manager, target, storage.UploadInput{ObjectKey: key, LocalPath: tempPath, ContentType: detectPlatformFontSubsetContentType(key)}); err != nil {
		return err
	}
	*createdKeys = append(*createdKeys, key)
	reader, err := manager.OpenRead(ctx, target, key)
	if err != nil {
		return err
	}
	defer reader.Close()
	targetDigest := sha256.New()
	targetSize, err := io.Copy(targetDigest, reader)
	if err != nil {
		return err
	}
	if targetSize != size || !bytes.Equal(digest.Sum(nil), targetDigest.Sum(nil)) {
		return errors.New("字体目标文件内容不一致")
	}
	return nil
}

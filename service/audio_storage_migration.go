package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/service/storage"
	"sealchat/utils"
)

// Audio local keys are relative to the running AudioService root. They must
// never go through Manager's attachment/audio namespace routing.
type audioMigrationStore struct {
	manager *storage.Manager
	local   *localAudioStorage
}

func newAudioMigrationStore(manager *storage.Manager) (*audioMigrationStore, error) {
	svc := GetAudioService()
	if svc == nil || svc.storage == nil {
		return nil, errors.New("音频服务未初始化")
	}
	if manager == nil {
		return nil, errors.New("存储服务未初始化")
	}
	return &audioMigrationStore{manager: manager, local: svc.storage}, nil
}

func (s *audioMigrationStore) localPath(key string) (string, error) {
	clean := filepath.Clean(key)
	if strings.TrimSpace(key) == "" || clean == "." || filepath.IsAbs(clean) || strings.Contains(key, "\\") {
		return "", errors.New("非法音频 objectKey")
	}
	return s.local.fullPath(key)
}

func (s *audioMigrationStore) exists(ctx context.Context, backend storage.BackendType, key string) (bool, error) {
	if backend == storage.BackendS3 {
		return s.manager.Exists(ctx, backend, key)
	}
	path, err := s.localPath(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (s *audioMigrationStore) materialize(ctx context.Context, backend storage.BackendType, key string) (string, error) {
	f, err := os.CreateTemp("", "sealchat-audio-migration-*")
	if err != nil {
		return "", err
	}
	path := f.Name()
	err = f.Close()
	if err == nil && backend == storage.BackendS3 {
		err = s.manager.DownloadToPath(ctx, backend, key, path)
	} else if err == nil {
		var source string
		source, err = s.localPath(key)
		if err == nil {
			err = copyAudioMigrationFile(source, path)
		}
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func copyAudioMigrationFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	return errors.Join(copyErr, out.Close())
}

func persistLocalAudioMigrationFile(source, target string) (bool, error) {
	in, err := os.Open(source)
	if err != nil {
		return false, err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return false, err
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false, err
	}
	_, copyErr := io.Copy(out, in)
	return true, errors.Join(copyErr, out.Close())
}

func (s *audioMigrationStore) remove(ctx context.Context, backend storage.BackendType, key string) error {
	if backend == storage.BackendS3 {
		return s.manager.Delete(ctx, backend, key)
	}
	path, err := s.localPath(key)
	if err != nil {
		return err
	}
	if err = os.Remove(path); os.IsNotExist(err) {
		return nil
	}
	return err
}

type audioMigrationObject struct {
	label       string
	storageType model.StorageType
	objectKey   string
	size        int64
	tempPath    string
	sum         [sha256.Size]byte
	targetKey   string
	created     bool
	reuse       bool
}

func audioMigrationObjects(asset *model.AudioAsset) []audioMigrationObject {
	objects := []audioMigrationObject{{storageType: asset.StorageType, objectKey: asset.ObjectKey, size: asset.Size}}
	for _, v := range asset.Variants {
		objects = append(objects, audioMigrationObject{label: v.Label, storageType: v.StorageType, objectKey: v.ObjectKey, size: v.Size})
	}
	return objects
}

func audioNeedsStorageMigration(asset *model.AudioAsset, target storage.BackendType) bool {
	for _, object := range audioMigrationObjects(asset) {
		if convertModelToBackend(object.storageType) != target {
			return true
		}
	}
	return false
}

// Keyset scanning keeps both preview and candidate selection portable across
// JSON database implementations, with bounded memory even for mixed assets.
func scanAudioMigrationAssets(db *gorm.DB, visit func(*model.AudioAsset) bool) error {
	if db == nil {
		return errors.New("数据库未初始化")
	}
	cursor := ""
	for {
		var page []model.AudioAsset
		if err := db.Where("deleted_at IS NULL AND id > ?", cursor).Order("id ASC").Limit(200).Find(&page).Error; err != nil {
			return err
		}
		for i := range page {
			if !visit(&page[i]) {
				return nil
			}
		}
		if len(page) < 200 {
			return nil
		}
		cursor = page[len(page)-1].ID
	}
}

func audioStorageMigrationPreview(db *gorm.DB, target storage.BackendType) (*S3MigrationStats, error) {
	stats := &S3MigrationStats{}
	err := scanAudioMigrationAssets(db, func(asset *model.AudioAsset) bool {
		if audioNeedsStorageMigration(asset, target) {
			stats.Total++
			if asset.TranscodeStatus == model.AudioTranscodePending {
				stats.Skipped++
			} else {
				stats.Pending++
			}
		}
		return true
	})
	return stats, err
}

func executeAudioStorageMigration(target StorageMigrationTarget, batchSize int, dryRun, deleteSource bool) (*S3MigrationStats, []S3MigrationItemResult, error) {
	if batchSize <= 0 {
		batchSize = 100
	}
	if batchSize > 1000 {
		batchSize = 1000
	}
	backend, err := storageMigrationBackend(target)
	if err != nil {
		return nil, nil, err
	}
	manager := GetStorageManager()
	if manager == nil || !manager.HasRemote() {
		return nil, nil, fmt.Errorf("%w: S3 未启用或初始化失败", ErrS3MigrationS3NotReady)
	}
	var assets []*model.AudioAsset
	err = scanAudioMigrationAssets(model.GetDB(), func(asset *model.AudioAsset) bool {
		if asset.TranscodeStatus != model.AudioTranscodePending && audioNeedsStorageMigration(asset, backend) {
			assets = append(assets, asset)
		}
		return len(assets) < batchSize
	})
	if err != nil {
		return nil, nil, err
	}
	stats := &S3MigrationStats{Total: int64(len(assets)), Pending: int64(len(assets))}
	results := make([]S3MigrationItemResult, 0, len(assets))
	for _, asset := range assets {
		result := migrateAudioAssetToBackend(context.Background(), model.GetDB(), manager, asset, backend, dryRun, deleteSource)
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

func audioMigrationDigest(path string) (int64, [sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	f, err := os.Open(path)
	if err != nil {
		return 0, sum, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	copy(sum[:], h.Sum(nil))
	return n, sum, err
}

func (s *audioMigrationStore) verify(ctx context.Context, backend storage.BackendType, key string, source audioMigrationObject) error {
	exists, err := s.exists(ctx, backend, key)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("目标音频对象不存在")
	}
	path, err := s.materialize(ctx, backend, key)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	size, sum, err := audioMigrationDigest(path)
	if err != nil {
		return err
	}
	if size != source.size || sum != source.sum {
		return fmt.Errorf("音频目标对象内容冲突（大小或 SHA-256 不一致）: %s", key)
	}
	return nil
}

// Both migration and background transcode must commit only against their own
// snapshot. Never hold a DB transaction open across file/network operations.
func audioStorageSnapshotQuery(db *gorm.DB, asset *model.AudioAsset) *gorm.DB {
	return db.Model(&model.AudioAsset{}).Where("id = ? AND deleted_at IS NULL AND storage_type = ? AND object_key = ? AND transcode_status = ?",
		asset.ID, asset.StorageType, asset.ObjectKey, asset.TranscodeStatus)
}

func commitAudioStorageMigration(db *gorm.DB, snapshot *model.AudioAsset, updates map[string]any) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var current model.AudioAsset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", snapshot.ID).First(&current).Error; err != nil {
			return err
		}
		// Compare storage state in Go rather than using database-specific JSON SQL.
		// Metadata-only updates such as access tracking must not invalidate a transcode.
		if current.StorageType != snapshot.StorageType || current.ObjectKey != snapshot.ObjectKey ||
			current.TranscodeStatus != snapshot.TranscodeStatus || !reflect.DeepEqual(current.Variants, snapshot.Variants) {
			return errors.New("音频状态并发冲突")
		}
		res := audioStorageSnapshotQuery(tx, snapshot).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("音频状态并发冲突")
		}
		return nil
	})
}

func migrateAudioAssetToBackend(ctx context.Context, db *gorm.DB, manager *storage.Manager, candidate *model.AudioAsset, target storage.BackendType, dryRun, deleteSource bool) (result S3MigrationItemResult) {
	result.Kind = S3MigrationKindAudio
	if candidate == nil {
		result.Skipped, result.SkipReason = true, "nil asset"
		return
	}
	result.PrimaryID = candidate.ID
	var asset model.AudioAsset
	if err := db.Where("id = ? AND deleted_at IS NULL", candidate.ID).First(&asset).Error; err != nil {
		result.Error = err.Error()
		return
	}
	result.RecordCount = 1 + len(asset.Variants)
	if asset.TranscodeStatus == model.AudioTranscodePending {
		result.Skipped, result.SkipReason = true, "音频正在转码"
		return
	}
	if !audioNeedsStorageMigration(&asset, target) {
		result.Skipped, result.SkipReason = true, "already on target storage"
		return
	}
	store, err := newAudioMigrationStore(manager)
	if err != nil {
		result.Error = err.Error()
		return
	}
	if !manager.HasRemote() { // Both directions require the remote source or target.
		result.Error = "S3 未启用或初始化失败"
		return
	}
	objects := audioMigrationObjects(&asset)
	committed := false
	defer func() {
		for _, object := range objects {
			if object.tempPath != "" {
				_ = os.Remove(object.tempPath)
			}
			if !committed && object.created {
				// A reused object is never owned by this attempt.
				if err := store.remove(ctx, target, object.targetKey); err != nil {
					result.Error += fmt.Sprintf("; 清理新建目标失败: %v", err)
				}
			}
		}
	}()
	// Materialize every source before writing any target, including components
	// already on target: a mixed asset is complete only if all parts are readable.
	for i := range objects {
		o := &objects[i]
		o.tempPath, err = store.materialize(ctx, convertModelToBackend(o.storageType), o.objectKey)
		if err != nil {
			result.Error = fmt.Sprintf("读取音频源文件失败: %v", err)
			return
		}
		var size int64
		size, o.sum, err = audioMigrationDigest(o.tempPath)
		if err != nil || (o.size > 0 && size != o.size) {
			result.Error = fmt.Sprintf("音频源文件校验失败: 大小 got=%d want=%d, error=%v", size, o.size, err)
			return
		}
		o.size = size
		o.targetKey = o.objectKey
		if convertModelToBackend(o.storageType) == target {
			continue
		}
		o.targetKey = audioMigrationObjectKey(asset.ID, o.label, o.objectKey)
		if target == storage.BackendLocal {
			o.targetKey = audioMigrationLocalObjectKey(asset.ID, o.label, o.objectKey)
		}
		o.reuse, err = store.exists(ctx, target, o.targetKey)
		if err != nil {
			result.Error = fmt.Sprintf("检查音频目标对象失败: %v", err)
			return
		}
		if o.reuse {
			if err = store.verify(ctx, target, o.targetKey, *o); err != nil {
				result.Error = err.Error()
				return
			}
		} else if target == storage.BackendS3 {
			// FPutObject has overwrite semantics. Use a fresh, unguessable key for
			// new remote objects so a check/upload race cannot overwrite a known key.
			o.targetKey = audioMigrationObjectKey(asset.ID, "migration-"+utils.NewID(), o.targetKey)
			exists, checkErr := store.exists(ctx, target, o.targetKey)
			if checkErr != nil || exists {
				result.Error = fmt.Sprintf("检查新音频目标对象失败或冲突: %v", checkErr)
				return
			}
		}
	}
	if dryRun {
		result.Success = true
		result.ObjectKey = objects[0].targetKey
		return
	}
	for i := range objects {
		o := &objects[i]
		if convertModelToBackend(o.storageType) != target && !o.reuse {
			if target == storage.BackendLocal {
				var path string
				path, err = store.localPath(o.targetKey)
				if err == nil {
					o.created, err = persistLocalAudioMigrationFile(o.tempPath, path)
				}
			} else {
				_, err = manager.UploadWithBackend(ctx, target, storage.UploadInput{
					ObjectKey: o.targetKey, LocalPath: o.tempPath, ContentType: guessContentTypeFromFilename(o.objectKey),
				})
				o.created = err == nil
			}
			if err != nil {
				result.Error = fmt.Sprintf("写入音频目标存储失败: %v", err)
				return
			}
		}
		if err = store.verify(ctx, target, o.targetKey, *o); err != nil {
			result.Error = fmt.Sprintf("校验音频目标文件失败: %v", err)
			return
		}
	}
	variants := append(model.JSONList[model.AudioAssetVariant]{}, asset.Variants...)
	for i := range variants {
		variants[i].StorageType = convertBackendToModel(target)
		variants[i].ObjectKey = objects[i+1].targetKey
	}
	// A configuration save/activation during I/O must not make this snapshot
	// commit against a physical destination that will disappear after restart.
	if cfg := utils.GetConfig(); cfg != nil && manager.RequiresRestartFor(cfg.Storage) {
		result.Error = ErrStorageMigrationRestartRequired.Error()
		return
	}
	err = commitAudioStorageMigration(db, &asset, map[string]any{
		"storage_type": convertBackendToModel(target), "object_key": objects[0].targetKey, "variants": variants,
	})
	if err != nil {
		result.Error = fmt.Sprintf("更新音频数据库失败或并发冲突，请重试: %v", err)
		return
	}
	committed = true
	result.Success, result.ObjectKey = true, objects[0].targetKey
	if deleteSource {
		for _, object := range objects {
			source := convertModelToBackend(object.storageType)
			if source == target {
				continue
			}
			referenced, checkErr := audioMigrationObjectReferenced(db, source, object.objectKey)
			if checkErr == nil && !referenced {
				checkErr = store.remove(ctx, source, object.objectKey)
			}
			if checkErr != nil {
				// The committed target stays authoritative even if cleanup fails.
				result.Error += fmt.Sprintf("源音频清理警告: %v; ", checkErr)
			}
		}
	}
	return
}

func audioMigrationObjectReferenced(db *gorm.DB, backend storage.BackendType, key string) (bool, error) {
	referenced := false
	err := scanAudioMigrationAssets(db, func(asset *model.AudioAsset) bool {
		for _, object := range audioMigrationObjects(asset) {
			if convertModelToBackend(object.storageType) == backend && object.objectKey == key {
				referenced = true
				return false
			}
		}
		return true
	})
	return referenced, err
}

func audioMigrationObjectKey(assetID, label, existing string) string {
	name := filepath.Base(strings.ReplaceAll(existing, "\\", "/"))
	if name == "" || name == "." {
		name = "audio.ogg"
	}
	if label != "" {
		name = label + "-" + name
	}
	return storage.BuildAudioObjectKey(assetID, name)
}

func audioMigrationLocalObjectKey(assetID, label, existing string) string {
	dir, name := "original", assetID
	if label != "" {
		dir, name = "opus", assetID+"_"+label
	}
	// Match normal audio filenames (including the asset ID), so local playback
	// and the existing basename-based trash operation retain their semantics.
	name += filepath.Ext(existing)
	return dir + "/" + filepath.Base(storage.BuildAudioObjectKey(assetID, name))
}

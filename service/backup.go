package service

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/service/storage"
	"sealchat/utils"
)

// BackupInfo 备份文件信息
type BackupInfo struct {
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"createdAt"`
	Protected bool   `json:"protected"`
	Storage   string `json:"storage"`
}

var (
	ErrBackupRunning        = errors.New("backup is already running")
	ErrBackupUnsupported    = errors.New("backup only supported for sqlite")
	ErrBackupProtected      = errors.New("backup is protected by retention policy")
	ErrBackupInvalidStorage = errors.New("invalid backup storage")

	backupState struct {
		mu      sync.Mutex
		running bool
	}

	backupNow = time.Now
)

const (
	backupStorageLocal       = "local"
	backupStorageS3          = "s3"
	defaultBackupS3Prefix    = "backups"
	backupS3OperationTimeout = 30 * time.Second
)

type backupFile struct {
	Source string
	Name   string
}

func ExecuteBackup(cfg *utils.AppConfig) (*BackupInfo, error) {
	return executeBackup(cfg, false)
}

// ExecuteAutomaticBackup 执行自动备份，近期已有备份时跳过。
func ExecuteAutomaticBackup(cfg *utils.AppConfig) (bool, error) {
	info, err := executeBackup(cfg, true)
	return info != nil, err
}

func executeBackup(cfg *utils.AppConfig, respectMinInterval bool) (*BackupInfo, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	if !model.IsSQLite() {
		return nil, ErrBackupUnsupported
	}
	if !tryStartBackup() {
		return nil, ErrBackupRunning
	}
	defer finishBackup()

	backupDir := strings.TrimSpace(cfg.Backup.Path)
	if backupDir == "" {
		return nil, errors.New("backup path is empty")
	}
	now := backupNow()
	if respectMinInterval && cfg.Backup.MinIntervalMinutes > 0 {
		recent, err := hasRecentBackup(cfg.Backup, cfg.Backup.MinIntervalMinutes, now)
		if err != nil {
			return nil, err
		}
		if recent {
			return nil, nil
		}
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, err
	}

	dbPath, err := resolveSQLitePath(cfg.DSN)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}

	configPath := "config.yaml"
	if _, err := os.Stat(configPath); err != nil {
		return nil, err
	}

	model.FlushWAL()

	files := []backupFile{
		{Source: dbPath, Name: filepath.Base(dbPath)},
		{Source: configPath, Name: filepath.Base(configPath)},
	}
	if fileExists(dbPath + "-wal") {
		files = append(files, backupFile{Source: dbPath + "-wal", Name: filepath.Base(dbPath + "-wal")})
	}
	if fileExists(dbPath + "-shm") {
		files = append(files, backupFile{Source: dbPath + "-shm", Name: filepath.Base(dbPath + "-shm")})
	}

	timestamp := now.Format("20060102-150405")
	filename := fmt.Sprintf("backup-%s.zip", timestamp)
	targetPath := filepath.Join(backupDir, filename)
	tmpPath := targetPath + ".tmp"

	if err := writeBackupZip(tmpPath, files); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, err
	}
	localInfo := &BackupInfo{
		Filename:  filename,
		Size:      info.Size(),
		CreatedAt: info.ModTime().Unix(),
		Protected: false,
		Storage:   backupStorageLocal,
	}

	if !cfg.Backup.S3Enabled {
		pruneLocalBackups(cfg.Backup, now, filename)
		return localInfo, nil
	}

	manager := GetStorageManager()
	if manager == nil || !manager.HasRemote() {
		log.Printf("backup: S3 unavailable; local backup retained: %s", targetPath)
		pruneLocalBackups(cfg.Backup, now, filename)
		return localInfo, nil
	}

	objectKey := path.Join(normalizeBackupS3Prefix(cfg.Backup.S3Prefix), filename)
	if err := uploadBackupToS3(manager, targetPath, objectKey, info.Size()); err != nil {
		log.Printf("backup: S3 upload or verification failed; local backup retained: %s: %v", targetPath, err)
		pruneLocalBackups(cfg.Backup, now, filename)
		return localInfo, nil
	}
	remoteItems, err := listS3Backups(context.Background(), manager, cfg.Backup.S3Prefix)
	if err != nil {
		log.Printf("backup: S3 upload succeeded, but remote backups could not be listed; local ZIP retained and storage not switched to remote: %v", err)
		pruneLocalBackups(cfg.Backup, now, filename)
		return localInfo, nil
	}
	foundCurrent := false
	for _, item := range remoteItems {
		if item.Filename == filename {
			foundCurrent = true
			break
		}
	}
	if !foundCurrent {
		log.Printf("backup: S3 upload succeeded, but uploaded backup was not visible in remote listing; local ZIP retained and storage not switched to remote: %s", filename)
		pruneLocalBackups(cfg.Backup, now, filename)
		return localInfo, nil
	}

	removeErr := os.Remove(targetPath)
	if removeErr != nil {
		log.Printf("backup: S3 backup succeeded, but local copy could not be removed; local copy retained: %s: %v", targetPath, removeErr)
		pruneLocalBackups(cfg.Backup, now, filename)
	}

	if cfg.Backup.RetentionCount > 0 {
		if err := pruneS3Backups(context.Background(), manager, cfg.Backup, now); err != nil {
			log.Printf("backup: S3 cleanup failed: %v", err)
		}
	}
	return &BackupInfo{
		Filename:  filename,
		Size:      info.Size(),
		CreatedAt: info.ModTime().Unix(),
		Protected: false,
		Storage:   backupStorageS3,
	}, nil
}

func hasRecentBackup(cfg utils.BackupConfig, minIntervalMinutes int, now time.Time) (bool, error) {
	items, err := listBackups(strings.TrimSpace(cfg.Path))
	if err != nil {
		return false, err
	}
	if hasRecentBackupIn(items, minIntervalMinutes, now) {
		return true, nil
	}
	if !cfg.S3Enabled {
		return false, nil
	}

	manager := GetStorageManager()
	if manager == nil || !manager.HasRemote() {
		log.Printf("backup: S3 unavailable while checking recent backups; using local backups only")
		return false, nil
	}
	s3Items, err := listS3Backups(context.Background(), manager, cfg.S3Prefix)
	if err != nil {
		log.Printf("backup: failed to check recent S3 backups; using local backups only: %v", err)
		return false, nil
	}
	return hasRecentBackupIn(s3Items, minIntervalMinutes, now), nil
}

func ListBackups(cfg utils.BackupConfig) ([]BackupInfo, error) {
	backupDir := strings.TrimSpace(cfg.Path)
	if backupDir == "" {
		return nil, errors.New("backup path is empty")
	}
	items, err := listBackups(backupDir)
	if err != nil {
		return nil, err
	}
	now := backupNow()
	applyProtectedFlags(items, cfg.IntervalHours, cfg.RetentionCount, now)

	manager := GetStorageManager()
	if manager != nil && manager.HasRemote() {
		s3Items, listErr := listS3Backups(context.Background(), manager, cfg.S3Prefix)
		if listErr != nil {
			log.Printf("backup: failed to list S3 backups; returning local backups only: %v", listErr)
		} else {
			applyProtectedFlags(s3Items, cfg.IntervalHours, cfg.RetentionCount, now)
			items = append(items, s3Items...)
			sort.Slice(items, func(i, j int) bool {
				return items[i].CreatedAt > items[j].CreatedAt
			})
		}
	}
	return items, nil
}

func DeleteBackup(cfg utils.BackupConfig, filename, storageLocation string) error {
	storageLocation = strings.ToLower(strings.TrimSpace(storageLocation))
	if storageLocation == "" {
		storageLocation = backupStorageLocal
	}
	if storageLocation == backupStorageS3 {
		name, err := normalizeBackupFilename(filename)
		if err != nil {
			return err
		}
		manager := GetStorageManager()
		if manager == nil || !manager.HasRemote() {
			return errors.New("S3 storage is unavailable")
		}
		items, err := listS3Backups(context.Background(), manager, cfg.S3Prefix)
		if err != nil {
			return err
		}
		protected := protectedBackupSet(items, cfg.IntervalHours, cfg.RetentionCount, backupNow())
		if _, ok := protected[name]; ok {
			return ErrBackupProtected
		}
		deleteCtx, cancel := context.WithTimeout(context.Background(), backupS3OperationTimeout)
		defer cancel()
		return manager.Delete(deleteCtx, storage.BackendS3, path.Join(normalizeBackupS3Prefix(cfg.S3Prefix), name))
	}
	if storageLocation != backupStorageLocal {
		return fmt.Errorf("%w: %q", ErrBackupInvalidStorage, storageLocation)
	}

	backupDir := strings.TrimSpace(cfg.Path)
	if backupDir == "" {
		return errors.New("backup path is empty")
	}
	target, err := resolveBackupFilePath(backupDir, filename)
	if err != nil {
		return err
	}
	items, err := listBackups(backupDir)
	if err != nil {
		return err
	}
	protected := protectedBackupSet(items, cfg.IntervalHours, cfg.RetentionCount, backupNow())
	if _, ok := protected[filepath.Base(target)]; ok {
		return ErrBackupProtected
	}
	return os.Remove(target)
}

func tryStartBackup() bool {
	backupState.mu.Lock()
	defer backupState.mu.Unlock()
	if backupState.running {
		return false
	}
	backupState.running = true
	return true
}

func finishBackup() {
	backupState.mu.Lock()
	backupState.running = false
	backupState.mu.Unlock()
}

func resolveSQLitePath(dsn string) (string, error) {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return "", errors.New("dbUrl is empty")
	}
	lower := strings.ToLower(trimmed)
	if lower == ":memory:" || strings.HasPrefix(lower, "file::memory:") {
		return "", errors.New("sqlite memory db is not supported")
	}
	if strings.HasPrefix(lower, "file:") {
		pathPart := trimmed[len("file:"):]
		if idx := strings.Index(pathPart, "?"); idx >= 0 {
			pathPart = pathPart[:idx]
		}
		pathPart = strings.TrimPrefix(pathPart, "//")
		if pathPart == "" {
			return "", errors.New("sqlite file path is empty")
		}
		if decoded, err := url.PathUnescape(pathPart); err == nil {
			pathPart = decoded
		}
		return filepath.Clean(pathPart), nil
	}
	if idx := strings.Index(trimmed, "?"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return filepath.Clean(trimmed), nil
}

func resolveBackupFilePath(dir, filename string) (string, error) {
	name, err := normalizeBackupFilename(filename)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func normalizeBackupFilename(filename string) (string, error) {
	name := strings.TrimSpace(filename)
	if name == "" {
		return "", errors.New("filename is empty")
	}
	if strings.ContainsAny(name, "/\\") {
		return "", errors.New("invalid filename")
	}
	if !strings.HasPrefix(name, "backup-") || !strings.HasSuffix(name, ".zip") {
		return "", errors.New("invalid backup filename")
	}
	return name, nil
}

func listBackups(dir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, err
	}
	items := make([]BackupInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "backup-") || !strings.HasSuffix(name, ".zip") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		items = append(items, BackupInfo{
			Filename:  name,
			Size:      info.Size(),
			CreatedAt: info.ModTime().Unix(),
			Storage:   backupStorageLocal,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items, nil
}

func normalizeBackupS3Prefix(prefix string) string {
	trimmed := strings.Trim(strings.TrimSpace(prefix), "/")
	if trimmed == "" {
		return defaultBackupS3Prefix
	}
	normalized := strings.Trim(path.Clean(trimmed), "/")
	if normalized == "" || normalized == "." {
		return defaultBackupS3Prefix
	}
	return normalized
}

func listS3Backups(ctx context.Context, manager *storage.Manager, prefix string) ([]BackupInfo, error) {
	normalizedPrefix := normalizeBackupS3Prefix(prefix)
	objects, err := manager.ListS3Prefix(ctx, normalizedPrefix+"/")
	if err != nil {
		return nil, err
	}

	items := make([]BackupInfo, 0, len(objects))
	for _, object := range objects {
		name := path.Base(object.ObjectKey)
		if _, err := normalizeBackupFilename(name); err != nil {
			continue
		}
		if object.ObjectKey != path.Join(normalizedPrefix, name) {
			continue
		}
		items = append(items, BackupInfo{
			Filename:  name,
			Size:      object.Size,
			CreatedAt: object.ModifiedAt.Unix(),
			Storage:   backupStorageS3,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items, nil
}

func hasRecentBackupIn(items []BackupInfo, minIntervalMinutes int, now time.Time) bool {
	if len(items) == 0 {
		return false
	}
	minimumInterval := time.Duration(minIntervalMinutes) * time.Minute
	latest := time.Unix(items[0].CreatedAt, 0)
	return now.Before(latest.Add(minimumInterval))
}

func uploadBackupToS3(manager *storage.Manager, localPath, objectKey string, localSize int64) error {
	result, err := manager.UploadToS3(context.Background(), storage.UploadInput{
		ObjectKey:   objectKey,
		LocalPath:   localPath,
		ContentType: "application/zip",
	})
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("S3 upload returned no result")
	}

	existsCtx, cancel := context.WithTimeout(context.Background(), backupS3OperationTimeout)
	defer cancel()
	exists, err := manager.Exists(existsCtx, storage.BackendS3, objectKey)
	if err != nil {
		return fmt.Errorf("verify S3 object existence: %w", err)
	}
	if !exists {
		return errors.New("uploaded S3 backup does not exist")
	}
	if result.Size > 0 && result.Size != localSize {
		return fmt.Errorf("uploaded S3 backup size mismatch: got %d, want %d", result.Size, localSize)
	}
	return nil
}

func pruneLocalBackups(cfg utils.BackupConfig, now time.Time, currentFilename string) {
	if cfg.RetentionCount <= 0 {
		return
	}
	if err := pruneBackups(strings.TrimSpace(cfg.Path), cfg.IntervalHours, cfg.RetentionCount, now, currentFilename); err != nil {
		log.Printf("backup: local cleanup failed: %v", err)
	}
}

func pruneS3Backups(ctx context.Context, manager *storage.Manager, cfg utils.BackupConfig, now time.Time) error {
	if cfg.RetentionCount <= 0 {
		return nil
	}
	items, err := listS3Backups(ctx, manager, cfg.S3Prefix)
	if err != nil {
		return err
	}
	if len(items) <= cfg.RetentionCount {
		return nil
	}
	keep := retainedBackupSet(items, cfg.IntervalHours, cfg.RetentionCount, now)
	for _, item := range items {
		if _, ok := keep[item.Filename]; ok {
			continue
		}
		deleteCtx, cancel := context.WithTimeout(ctx, backupS3OperationTimeout)
		err := manager.Delete(deleteCtx, storage.BackendS3, path.Join(normalizeBackupS3Prefix(cfg.S3Prefix), item.Filename))
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func pruneBackups(dir string, intervalHours, retentionCount int, now time.Time, currentFilename string) error {
	if retentionCount <= 0 {
		return nil
	}
	items, err := listBackups(dir)
	if err != nil {
		return err
	}
	if len(items) <= retentionCount {
		return nil
	}
	keep := retainedBackupSet(items, intervalHours, retentionCount, now)
	if currentFilename != "" {
		keep[currentFilename] = struct{}{}
	}
	for _, item := range items {
		if _, ok := keep[item.Filename]; ok {
			continue
		}
		target := filepath.Join(dir, item.Filename)
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func applyProtectedFlags(items []BackupInfo, intervalHours, retentionCount int, now time.Time) {
	protected := protectedBackupSet(items, intervalHours, retentionCount, now)
	for i := range items {
		_, items[i].Protected = protected[items[i].Filename]
	}
}

func retainedBackupSet(items []BackupInfo, intervalHours, retentionCount int, now time.Time) map[string]struct{} {
	keep := make(map[string]struct{}, retentionCount)
	if retentionCount <= 0 || len(items) == 0 {
		return keep
	}

	keep[items[0].Filename] = struct{}{}
	for name := range protectedBackupSet(items, intervalHours, retentionCount, now) {
		keep[name] = struct{}{}
	}
	for _, item := range items {
		if len(keep) >= retentionCount {
			break
		}
		keep[item.Filename] = struct{}{}
	}
	return keep
}

func protectedBackupSet(items []BackupInfo, intervalHours, retentionCount int, now time.Time) map[string]struct{} {
	protected := make(map[string]struct{})
	protectedCount := historicalProtectedCount(retentionCount)
	if len(items) == 0 || protectedCount == 0 {
		return protected
	}

	interval := backupIntervalDuration(intervalHours)
	for bucket := 1; bucket <= protectedCount; bucket++ {
		for _, item := range items {
			if backupAgeBucket(item.CreatedAt, interval, now) != bucket {
				continue
			}
			protected[item.Filename] = struct{}{}
			break
		}
	}
	return protected
}

func historicalProtectedCount(retentionCount int) int {
	if retentionCount <= 1 {
		return 0
	}
	count := retentionCount / 3
	if count < 1 {
		count = 1
	}
	if count > retentionCount-1 {
		return retentionCount - 1
	}
	return count
}

func backupIntervalDuration(intervalHours int) time.Duration {
	if intervalHours <= 0 {
		intervalHours = 12
	}
	return time.Duration(intervalHours) * time.Hour
}

func backupAgeBucket(createdAt int64, interval time.Duration, now time.Time) int {
	createdAtTime := time.Unix(createdAt, 0)
	if createdAtTime.After(now) {
		return 0
	}
	return int(now.Sub(createdAtTime) / interval)
}

func writeBackupZip(targetPath string, files []backupFile) error {
	if len(files) == 0 {
		return errors.New("no files to backup")
	}
	out, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer out.Close()

	zipWriter := zip.NewWriter(out)
	defer zipWriter.Close()

	for _, file := range files {
		info, err := os.Stat(file.Source)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = file.Name
		header.Method = zip.Deflate
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		input, err := os.Open(file.Source)
		if err != nil {
			return err
		}
		if _, err := io.Copy(writer, input); err != nil {
			_ = input.Close()
			return err
		}
		if err := input.Close(); err != nil {
			return err
		}
	}
	return nil
}

package service

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

// BackupInfo 备份文件信息
type BackupInfo struct {
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"createdAt"`
}

var (
	ErrBackupRunning     = errors.New("backup is already running")
	ErrBackupUnsupported = errors.New("backup only supported for sqlite")

	backupState struct {
		mu      sync.Mutex
		running bool
	}
)

type backupFile struct {
	Source string
	Name   string
}

func ExecuteBackup(cfg *utils.AppConfig) (*BackupInfo, error) {
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

	timestamp := time.Now().Format("20060102-150405")
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

	if cfg.Backup.RetentionCount > 0 {
		if err := pruneBackups(backupDir, cfg.Backup.RetentionCount); err != nil {
			log.Printf("backup: cleanup failed: %v", err)
		}
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, err
	}
	return &BackupInfo{
		Filename:  filename,
		Size:      info.Size(),
		CreatedAt: info.ModTime().Unix(),
	}, nil
}

func ListBackups(cfg utils.BackupConfig) ([]BackupInfo, error) {
	backupDir := strings.TrimSpace(cfg.Path)
	if backupDir == "" {
		return nil, errors.New("backup path is empty")
	}
	return listBackups(backupDir)
}

func DeleteBackup(cfg utils.BackupConfig, filename string) error {
	backupDir := strings.TrimSpace(cfg.Path)
	if backupDir == "" {
		return errors.New("backup path is empty")
	}
	target, err := resolveBackupFilePath(backupDir, filename)
	if err != nil {
		return err
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
	return filepath.Join(dir, name), nil
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
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items, nil
}

func pruneBackups(dir string, retentionCount int) error {
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
	for i := retentionCount; i < len(items); i++ {
		target := filepath.Join(dir, items[i].Filename)
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
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

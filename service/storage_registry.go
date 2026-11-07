package service

import (
	"errors"
	"log"
	"path/filepath"
	"strings"
	"sync/atomic"

	"sealchat/service/storage"
	"sealchat/utils"
)

var objectStorage atomic.Pointer[storage.Manager]

var ErrStorageRuntimeReload = errors.New("对象存储运行配置应用失败")
var ErrStorageMigrationRestartRequired = errors.New("存储配置已修改，需要重启后再执行迁移")

func storageMigrationConfigCheck() error {
	cfg := utils.GetConfig()
	if cfg != nil && (StorageReloadRequiresRestart(cfg.Storage) || audioStorageReloadRequiresRestart(cfg.Audio)) {
		return ErrStorageMigrationRestartRequired
	}
	return nil
}

func audioStorageReloadRequiresRestart(cfg utils.AudioConfig) bool {
	svc := GetAudioService()
	if svc == nil || svc.storage == nil {
		return false
	}
	desired := cfg.StorageDir
	if strings.TrimSpace(desired) == "" {
		desired = "./static/audio"
	}
	return filepath.Clean(svc.storage.rootDir) != filepath.Clean(desired)
}

func InitStorageManager(cfg utils.StorageConfig) (*storage.Manager, error) {
	mgr, err := storage.NewManager(cfg)
	if err != nil {
		return nil, err
	}
	objectStorage.Store(mgr)
	log.Printf("[storage] 当前存储模式: %s", mgr.ActiveBackend())
	return mgr, nil
}

func GetStorageManager() *storage.Manager {
	return objectStorage.Load()
}

func StorageReloadRequiresRestart(cfg utils.StorageConfig) bool {
	mgr := GetStorageManager()
	return mgr != nil && mgr.RequiresRestartFor(cfg)
}

func PrepareStorageManager(cfg utils.StorageConfig) (*storage.Manager, error) {
	candidate, err := storage.NewManager(cfg)
	if err != nil {
		return nil, ErrStorageRuntimeReload
	}
	// Startup permits local fallback; runtime changes must keep the old manager
	// when an explicitly enabled remote cannot initialize.
	if cfg.S3.Enabled && candidate.RemoteInitError() != nil {
		return nil, ErrStorageRuntimeReload
	}
	return candidate, nil
}

func ActivateStorageManager(candidate *storage.Manager) {
	if candidate == nil {
		return
	}
	objectStorage.Store(candidate)
	log.Printf("[storage] 运行配置已热重载，当前存储模式: %s", candidate.ActiveBackend())
}

func ReloadStorageManager(cfg utils.StorageConfig) (*storage.Manager, error) {
	candidate, err := PrepareStorageManager(cfg)
	if err != nil {
		return nil, err
	}
	ActivateStorageManager(candidate)
	return candidate, nil
}

func GetStorageStatus() storage.Status {
	status := GetStorageManager().Status()
	status.PendingRestart = storageMigrationConfigCheck() != nil
	return status
}

func TestStorageS3(cfg utils.StorageConfig) error {
	cfg.S3.TTSEnabled = false
	return storage.TestS3(cfg)
}

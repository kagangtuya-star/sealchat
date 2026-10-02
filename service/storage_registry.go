package service

import (
	"log"

	"sealchat/service/storage"
	"sealchat/utils"
)

var objectStorage *storage.Manager

func InitStorageManager(cfg utils.StorageConfig) (*storage.Manager, error) {
	mgr, err := storage.NewManager(cfg)
	if err != nil {
		return nil, err
	}
	objectStorage = mgr
	log.Printf("[storage] 当前存储模式: %s", mgr.ActiveBackend())
	return mgr, nil
}

func GetStorageManager() *storage.Manager {
	return objectStorage
}

func GetStorageStatus() storage.Status {
	return GetStorageManager().Status()
}

func TestStorageS3(cfg utils.StorageConfig) error {
	return storage.TestS3(cfg)
}

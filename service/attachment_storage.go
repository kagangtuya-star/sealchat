package service

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/service/storage"
)

type AttachmentLocation struct {
	StorageType model.StorageType
	ObjectKey   string
	ExternalURL string
}

func PersistAttachmentFile(hash []byte, size int64, tempPath string, contentType string) (*AttachmentLocation, error) {
	manager := GetStorageManager()
	if manager == nil {
		return nil, errors.New("存储服务未初始化")
	}
	ctx := context.Background()
	targetBackend := manager.ActiveBackendForAttachment()
	if reused, ok, err := tryReuseAttachment(hash, size, targetBackend); err != nil {
		return nil, err
	} else if ok && reused != nil {
		_ = os.Remove(tempPath)
		return reused, nil
	}
	objectKey := storage.BuildAttachmentObjectKey(hex.EncodeToString(hash), size, time.Now())
	result, err := manager.UploadAttachment(ctx, storage.UploadInput{
		ObjectKey:   objectKey,
		LocalPath:   tempPath,
		ContentType: contentType,
	})
	if err != nil {
		return nil, err
	}
	if result.Backend == storage.BackendS3 {
		_ = os.Remove(tempPath)
	}
	return &AttachmentLocation{
		StorageType: convertBackendToModel(result.Backend),
		ObjectKey:   result.ObjectKey,
		ExternalURL: result.PublicURL,
	}, nil
}

func ResolveLocalAttachmentPath(objectKey string) (string, error) {
	manager := GetStorageManager()
	if manager == nil {
		return "", errors.New("存储服务未初始化")
	}
	return manager.ResolveLocalPath(objectKey)
}

func tryReuseAttachment(hash []byte, size int64, targetBackend storage.BackendType) (*AttachmentLocation, bool, error) {
	existing, err := model.AttachmentFindByHashAndSize(hash, size)
	if err != nil {
		return nil, false, err
	}
	if existing == nil || strings.TrimSpace(existing.ObjectKey) == "" {
		return nil, false, nil
	}
	existingBackend := convertModelToBackend(existing.StorageType)
	if existingBackend != targetBackend {
		return nil, false, nil
	}
	manager := GetStorageManager()
	if manager == nil {
		return nil, false, nil
	}
	ctx := context.Background()
	switch existingBackend {
	case storage.BackendS3:
		ok, err := manager.Exists(ctx, storage.BackendS3, existing.ObjectKey)
		if err != nil || !ok {
			return nil, false, nil
		}
	default:
		path, err := manager.ResolveLocalPath(existing.ObjectKey)
		if err != nil {
			return nil, false, nil
		}
		if _, err := os.Stat(path); err != nil {
			return nil, false, nil
		}
	}
	return &AttachmentLocation{
		StorageType: existing.StorageType,
		ObjectKey:   existing.ObjectKey,
		ExternalURL: existing.ExternalURL,
	}, true, nil
}

func convertBackendToModel(backend storage.BackendType) model.StorageType {
	if backend == storage.BackendS3 {
		return model.StorageS3
	}
	return model.StorageLocal
}

func convertModelToBackend(storageType model.StorageType) storage.BackendType {
	if storageType == model.StorageS3 {
		return storage.BackendS3
	}
	return storage.BackendLocal
}

func AttachmentPublicURL(att *model.AttachmentModel) string {
	if att == nil {
		return ""
	}
	if url := strings.TrimSpace(att.ExternalURL); url != "" {
		return url
	}
	manager := GetStorageManager()
	if manager == nil || strings.TrimSpace(att.ObjectKey) == "" {
		return ""
	}
	backend := convertModelToBackend(att.StorageType)
	if public := manager.PublicURL(backend, att.ObjectKey); public != "" {
		return public
	}
	return ""
}

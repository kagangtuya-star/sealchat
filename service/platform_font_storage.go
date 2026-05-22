package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sealchat/model"
	"sealchat/service/storage"
)

type PlatformFontLocation struct {
	StorageType model.StorageType
	ObjectKey   string
	PublicURL   string
}

var platformFontRuntimeBaseDirForTest string

func PersistPlatformFontFile(localPath, objectKey, contentType string) (*PlatformFontLocation, error) {
	manager := GetStorageManager()
	if manager == nil {
		return nil, errors.New("存储服务未初始化")
	}
	ctx := context.Background()
	targetBackend := manager.ActiveBackendForFont()
	result, err := uploadPlatformFontByBackend(ctx, manager, targetBackend, storage.UploadInput{
		ObjectKey:   objectKey,
		LocalPath:   localPath,
		ContentType: contentType,
	})
	if err != nil {
		return nil, err
	}
	if result.Backend == storage.BackendS3 {
		_ = os.Remove(localPath)
	}
	return &PlatformFontLocation{
		StorageType: convertFontBackendToModel(result.Backend),
		ObjectKey:   result.ObjectKey,
		PublicURL:   result.PublicURL,
	}, nil
}

func ResolveLocalPlatformFontPath(objectKey string) (string, error) {
	manager := GetStorageManager()
	if manager == nil {
		return "", errors.New("存储服务未初始化")
	}
	return manager.ResolveLocalPath(objectKey)
}

func DeletePlatformFontFile(storageType model.StorageType, objectKey string) error {
	if strings.TrimSpace(objectKey) == "" {
		return nil
	}
	manager := GetStorageManager()
	if manager == nil {
		return errors.New("存储服务未初始化")
	}
	return manager.Delete(context.Background(), convertFontModelToBackend(storageType), objectKey)
}

func DeletePlatformFontPrefix(storageType model.StorageType, objectKey string) error {
	if strings.TrimSpace(objectKey) == "" {
		return nil
	}
	manager := GetStorageManager()
	if manager == nil {
		return errors.New("存储服务未初始化")
	}
	return manager.DeletePrefix(context.Background(), convertFontModelToBackend(storageType), objectKey)
}

func uploadPlatformFontByBackend(
	ctx context.Context,
	manager *storage.Manager,
	targetBackend storage.BackendType,
	input storage.UploadInput,
) (*storage.UploadResult, error) {
	switch targetBackend {
	case storage.BackendS3:
		return manager.UploadToS3(ctx, input)
	default:
		return manager.Upload(ctx, input)
	}
}

func convertFontBackendToModel(backend storage.BackendType) model.StorageType {
	if backend == storage.BackendS3 {
		return model.StorageFontS3
	}
	return model.StorageFontLocal
}

func convertFontModelToBackend(storageType model.StorageType) storage.BackendType {
	if storageType == model.StorageFontS3 {
		return storage.BackendS3
	}
	return storage.BackendLocal
}

func SetPlatformFontRuntimeBaseDirForTest(dir string) func() {
	previous := platformFontRuntimeBaseDirForTest
	platformFontRuntimeBaseDirForTest = strings.TrimSpace(dir)
	return func() {
		platformFontRuntimeBaseDirForTest = previous
	}
}

func ResolveBundledPlatformFontRuntimeFile(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("runtime asset name is empty")
	}
	normalized := filepath.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
	if normalized == "." || strings.HasPrefix(normalized, "..") || strings.Contains(normalized, "../") {
		return "", errors.New("invalid runtime asset name")
	}

	roots := make([]string, 0, 4)
	if strings.TrimSpace(platformFontRuntimeBaseDirForTest) != "" {
		roots = append(roots, platformFontRuntimeBaseDirForTest)
	}
	if cwd, cwdErr := os.Getwd(); cwdErr == nil && strings.TrimSpace(cwd) != "" {
		roots = append(roots, cwd)
	}
	if exe, exeErr := os.Executable(); exeErr == nil && strings.TrimSpace(exe) != "" {
		exeDir := filepath.Dir(exe)
		roots = append(roots, exeDir)
		parent := filepath.Dir(exeDir)
		if parent != exeDir {
			roots = append(roots, parent)
		}
	}

	seen := map[string]struct{}{}
	var tried []string
	for _, root := range roots {
		root = filepath.Clean(root)
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		candidate := filepath.Join(root, "bin", "cn-font-split", normalized)
		tried = append(tried, candidate)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("platform font split runtime %q not found under bin/cn-font-split, tried: %s", normalized, strings.Join(tried, ", "))
}

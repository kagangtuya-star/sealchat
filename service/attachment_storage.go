package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/service/storage"
	"sealchat/utils"
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

func PersistAttachmentFileForceNew(hash []byte, size int64, tempPath string, contentType string, originalName string) (*AttachmentLocation, error) {
	manager := GetStorageManager()
	if manager == nil {
		return nil, errors.New("存储服务未初始化")
	}
	ctx := context.Background()
	objectKey := storage.BuildAttachmentReissueObjectKey(hex.EncodeToString(hash), size, originalName, time.Now())
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

func attachmentHistoricalUploadRoots() []string {
	roots := make([]string, 0, 4)
	seen := map[string]struct{}{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		roots = append(roots, clean)
	}

	cfg := utils.GetConfig()
	if cfg != nil && strings.TrimSpace(cfg.Storage.Local.UploadDir) != "" {
		add(cfg.Storage.Local.UploadDir)
	}
	add("./data/upload")
	add("./sealchat-data/upload")
	add("./data11/upload")
	return roots
}

func tryMaterializeFromHistoricalLocalRoots(att *model.AttachmentModel, tempPath string) bool {
	if att == nil {
		return false
	}
	candidates := make([]string, 0, 8)
	if objectKey := strings.TrimSpace(att.ObjectKey); objectKey != "" {
		clean := strings.TrimLeft(filepath.Clean(strings.ReplaceAll(objectKey, "\\", "/")), "/")
		relative := clean
		if strings.HasPrefix(clean, "attachments/") {
			relative = strings.TrimPrefix(clean, "attachments/")
		}
		if relative != "" && relative != "." {
			for _, root := range attachmentHistoricalUploadRoots() {
				candidates = append(candidates, filepath.Join(root, filepath.FromSlash(relative)))
			}
		}
	}
	if len(att.Hash) > 0 {
		legacyKey := fmt.Sprintf("%s_%d", hex.EncodeToString([]byte(att.Hash)), att.Size)
		for _, root := range attachmentHistoricalUploadRoots() {
			candidates = append(candidates, filepath.Join(root, legacyKey))
		}
	}
	if attachmentID := strings.TrimSpace(att.ID); attachmentID != "" {
		for _, root := range attachmentHistoricalUploadRoots() {
			candidates = append(candidates, filepath.Join(root, attachmentID))
		}
	}

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if err := copyLocalFileToPath(candidate, tempPath); err == nil {
			return true
		}
	}
	return false
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

func MaterializeAttachmentToTempFile(att *model.AttachmentModel) (string, error) {
	if att == nil {
		return "", errors.New("附件不存在")
	}
	manager := GetStorageManager()
	if manager == nil {
		return "", errors.New("存储服务未初始化")
	}

	pattern := "sealchat-attachment-*"
	if ext := strings.TrimSpace(filepath.Ext(att.Filename)); ext != "" {
		pattern += ext
	}
	tempFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}

	if objectKey := strings.TrimSpace(att.ObjectKey); objectKey != "" {
		backend := convertModelToBackend(att.StorageType)
		if err := manager.DownloadToPath(context.Background(), backend, objectKey, tempPath); err == nil {
			return tempPath, nil
		}
	}

	legacyKey := fmt.Sprintf("%s_%d", hex.EncodeToString([]byte(att.Hash)), att.Size)
	if strings.TrimSpace(legacyKey) != "" {
		if err := manager.DownloadToPath(context.Background(), storage.BackendLocal, legacyKey, tempPath); err == nil {
			return tempPath, nil
		}
	}

	if attachmentID := strings.TrimSpace(att.ID); attachmentID != "" {
		if err := manager.DownloadToPath(context.Background(), storage.BackendLocal, attachmentID, tempPath); err == nil {
			return tempPath, nil
		}
	}
	if tryMaterializeFromHistoricalLocalRoots(att, tempPath) {
		return tempPath, nil
	}

	exportURL := strings.TrimSpace(AttachmentExportURL(att))
	if exportURL == "" {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("附件 %s 缺少可读取的存储地址", strings.TrimSpace(att.ID))
	}
	if err := downloadAttachmentURLToPath(exportURL, tempPath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	return tempPath, nil
}

func copyLocalFileToPath(sourcePath string, targetPath string) error {
	input, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	output, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return nil
}

func downloadAttachmentURLToPath(targetURL string, tempPath string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(targetURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载附件失败: %s", resp.Status)
	}
	output, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	defer output.Close()
	if _, err := io.Copy(output, resp.Body); err != nil {
		return err
	}
	return nil
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

// AttachmentReadURL returns a browser-readable CDN or signed URL.
func AttachmentReadURL(ctx context.Context, att *model.AttachmentModel) string {
	if att == nil {
		return ""
	}
	manager := GetStorageManager()
	if manager != nil && strings.TrimSpace(att.ObjectKey) != "" {
		backend := convertModelToBackend(att.StorageType)
		if target := manager.ResolveReadURL(ctx, backend, att.ObjectKey); target != "" {
			return target
		}
	}
	return strings.TrimSpace(att.ExternalURL)
}

func AttachmentExportURL(att *model.AttachmentModel) string {
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
	return manager.ResolveAttachmentExportURL(context.Background(), convertModelToBackend(att.StorageType), att.ObjectKey)
}

package storage

import (
	"context"
	"fmt"
	"log"
	"mime"
	"path/filepath"
	"strings"

	"sealchat/utils"
)

type Manager struct {
	cfg           utils.StorageConfig
	local         *localBackend
	remote        *s3Backend
	remoteInitErr error
	preferred     BackendType
	localBaseURL  string
	remoteBaseURL string
}

func NewManager(cfg utils.StorageConfig) (*Manager, error) {
	local, err := newLocalBackend(cfg.Local.UploadDir, cfg.Local.AudioDir)
	if err != nil {
		return nil, err
	}
	mgr := &Manager{
		cfg:          cfg,
		local:        local,
		preferred:    BackendLocal,
		localBaseURL: strings.TrimRight(cfg.Local.BaseURL, "/"),
	}
	if cfg.S3.Enabled {
		if remote, err := newS3Backend(cfg.S3); err != nil {
			mgr.remoteInitErr = err
			log.Printf("[storage] 初始化 S3 失败，回退到本地：%v", err)
		} else {
			mgr.remote = remote
		}
	}
	mgr.preferred = mgr.decidePreferred()
	return mgr, nil
}

func (m *Manager) decidePreferred() BackendType {
	switch cfgMode := strings.ToLower(string(m.cfg.Mode)); cfgMode {
	case string(utils.StorageModeS3):
		if m.remote != nil {
			return BackendS3
		}
	case string(utils.StorageModeAuto):
		if m.remote != nil {
			return BackendS3
		}
	default:
		return BackendLocal
	}
	return BackendLocal
}

func (m *Manager) ActiveBackend() BackendType {
	return m.preferred
}

func (m *Manager) ActiveBackendForAttachment() BackendType {
	return m.activeBackendWithToggle(func(s3 utils.S3StorageConfig) bool {
		if s3.AttachmentsEnabled == nil {
			return true
		}
		return *s3.AttachmentsEnabled
	})
}

func (m *Manager) ActiveBackendForAudio() BackendType {
	return m.activeBackendWithToggle(func(s3 utils.S3StorageConfig) bool {
		if s3.AudioEnabled == nil {
			return true
		}
		return *s3.AudioEnabled
	})
}

func (m *Manager) activeBackendWithToggle(enabled func(utils.S3StorageConfig) bool) BackendType {
	if m == nil {
		return BackendLocal
	}
	mode := strings.ToLower(string(m.cfg.Mode))
	switch mode {
	case string(utils.StorageModeS3), string(utils.StorageModeAuto):
		if m.remote != nil && enabled(m.cfg.S3) {
			return BackendS3
		}
	}
	return BackendLocal
}

func (m *Manager) HasRemote() bool {
	return m.remote != nil
}

func (m *Manager) RemoteInitError() error {
	if m == nil {
		return nil
	}
	return m.remoteInitErr
}

func (m *Manager) Upload(ctx context.Context, input UploadInput) (*UploadResult, error) {
	if strings.TrimSpace(input.ObjectKey) == "" {
		return nil, fmt.Errorf("objectKey 不能为空")
	}
	input.ContentType = normalizeContentType(input.ContentType, input.ObjectKey)
	if m.preferred == BackendS3 && m.remote != nil {
		result, err := m.remote.upload(ctx, input)
		if err == nil {
			return result, nil
		}
		logS3Fallback(err)
	}
	return m.local.upload(input)
}

func (m *Manager) UploadAttachment(ctx context.Context, input UploadInput) (*UploadResult, error) {
	if strings.TrimSpace(input.ObjectKey) == "" {
		return nil, fmt.Errorf("objectKey 不能为空")
	}
	input.ContentType = normalizeContentType(input.ContentType, input.ObjectKey)
	target := m.ActiveBackendForAttachment()
	if target == BackendS3 && m.remote != nil {
		result, err := m.remote.upload(ctx, input)
		if err == nil {
			return result, nil
		}
		logS3Fallback(err)
	}
	return m.local.upload(input)
}

func (m *Manager) UploadToS3(ctx context.Context, input UploadInput) (*UploadResult, error) {
	if strings.TrimSpace(input.ObjectKey) == "" {
		return nil, fmt.Errorf("objectKey 不能为空")
	}
	if m.remote == nil {
		return nil, fmt.Errorf("未启用 S3 存储")
	}
	input.ContentType = normalizeContentType(input.ContentType, input.ObjectKey)
	return m.remote.upload(ctx, input)
}

func (m *Manager) Exists(ctx context.Context, backend BackendType, objectKey string) (bool, error) {
	switch backend {
	case BackendS3:
		if m.remote == nil {
			return false, fmt.Errorf("未启用 S3 存储")
		}
		return m.remote.exists(ctx, objectKey)
	default:
		return m.local.exists(objectKey)
	}
}

func (m *Manager) Delete(ctx context.Context, backend BackendType, objectKey string) error {
	switch backend {
	case BackendS3:
		if m.remote == nil {
			return nil
		}
		return m.remote.delete(ctx, objectKey)
	default:
		return m.local.delete(objectKey)
	}
}

func (m *Manager) PublicURL(backend BackendType, objectKey string) string {
	switch backend {
	case BackendS3:
		if m.remote == nil {
			return ""
		}
		return m.remote.publicURL(objectKey)
	case BackendLocal:
		if m.localBaseURL == "" {
			return ""
		}
		return fmt.Sprintf("%s/%s", m.localBaseURL, strings.TrimLeft(objectKey, "/"))
	default:
		return ""
	}
}

func (m *Manager) ResolveLocalPath(objectKey string) (string, error) {
	if m.local == nil {
		return "", fmt.Errorf("本地存储未初始化")
	}
	return m.local.resolvePath(objectKey)
}

func normalizeContentType(contentType, objectKey string) string {
	ct := strings.TrimSpace(strings.ToLower(contentType))
	if ct != "" && ct != "application/octet-stream" {
		return ct
	}
	ext := filepath.Ext(objectKey)
	if ext == "" {
		return "application/octet-stream"
	}
	if val := mime.TypeByExtension(ext); val != "" {
		return val
	}
	return "application/octet-stream"
}

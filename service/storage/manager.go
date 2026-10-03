package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// Status contains only non-secret settings captured when the manager started.
type Status struct {
	Configured    bool                   `json:"configured"`
	Initialized   bool                   `json:"initialized"`
	Enabled       bool                   `json:"enabled"`
	RemoteReady   bool                   `json:"remoteReady"`
	ActiveBackend BackendType            `json:"activeBackend"`
	Endpoint      string                 `json:"endpoint"`
	Region        string                 `json:"region"`
	Bucket        string                 `json:"bucket"`
	Modules       map[string]BackendType `json:"modules"`
	LastError     string                 `json:"lastError"`
}

func (m *Manager) Status() Status {
	status := Status{
		ActiveBackend: BackendLocal,
		Modules: map[string]BackendType{
			"attachments": BackendLocal, "audio": BackendLocal, "tts": BackendLocal,
			"theaterAttachments": BackendLocal, "theaterAudio": BackendLocal, "fonts": BackendLocal,
		},
	}
	if m == nil {
		status.LastError = "存储管理器尚未初始化"
		return status
	}
	s3 := m.cfg.S3
	status.Configured = strings.TrimSpace(s3.Endpoint) != "" && strings.TrimSpace(s3.Bucket) != ""
	status.Initialized = true
	status.Enabled = s3.Enabled
	status.RemoteReady = m.HasRemote()
	status.ActiveBackend = m.ActiveBackend()
	// Strip URL credentials, query parameters and fragments even from a malformed endpoint.
	endpoint := strings.TrimSpace(s3.Endpoint)
	if !strings.Contains(endpoint, "://") {
		endpoint = "//" + endpoint
	}
	if parsed, err := url.Parse(endpoint); err == nil {
		status.Endpoint = parsed.Host
	}
	status.Region = s3.Region
	status.Bucket = s3.Bucket
	status.Modules["attachments"] = m.ActiveBackendForAttachment()
	status.Modules["audio"] = m.ActiveBackendForAudio()
	status.Modules["tts"] = m.ActiveBackendForTTS()
	status.Modules["theaterAttachments"] = m.ActiveBackendForTheaterAttachment()
	status.Modules["theaterAudio"] = m.ActiveBackendForTheaterAudio()
	status.Modules["fonts"] = m.ActiveBackendForFont()
	if m.RemoteInitError() != nil {
		// SDK errors may contain URLs or credentials; never serialize the raw error.
		status.LastError = "S3 初始化失败，当前回退到本地；请检查配置并测试连接"
	} else if s3.TTSEnabled && m.remote != nil && !m.remote.privateReadVerified {
		status.LastError = "TTS 对象存储未启用：无法确认 Bucket/CDN 禁止匿名读取，当前 TTS 回退到本地"
	}
	return status
}

func NewManager(cfg utils.StorageConfig) (*Manager, error) {
	local, err := newLocalBackend(cfg.Local.UploadDir, cfg.Local.AudioDir, cfg.Local.FontDir)
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
		uploadTimeout := time.Duration(cfg.UploadTimeoutSeconds) * time.Second
		if remote, err := newS3Backend(cfg.S3, uploadTimeout); err != nil {
			mgr.remoteInitErr = err
			log.Printf("[storage] 初始化 S3 失败，回退到本地：%v", err)
		} else {
			mgr.remote = remote
		}
	}
	mgr.preferred = mgr.decidePreferred()
	return mgr, nil
}

// RequiresRestartFor compares storage identities against the running configuration.
func (m *Manager) RequiresRestartFor(cfg utils.StorageConfig) bool {
	if m == nil {
		return false
	}
	if m.cfg.Local.UploadDir != cfg.Local.UploadDir ||
		m.cfg.Local.AudioDir != cfg.Local.AudioDir ||
		m.cfg.Local.FontDir != cfg.Local.FontDir {
		return true
	}
	if m.cfg.S3.Enabled {
		return !cfg.S3.Enabled ||
			strings.TrimSpace(m.cfg.S3.Endpoint) != strings.TrimSpace(cfg.S3.Endpoint) ||
			strings.TrimSpace(m.cfg.S3.Bucket) != strings.TrimSpace(cfg.S3.Bucket)
	}
	return false
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

func (m *Manager) ActiveBackendForTTS() BackendType {
	if m == nil || m.remote == nil || !m.remote.privateReadVerified {
		return BackendLocal
	}
	return m.activeBackendWithToggle(func(s3 utils.S3StorageConfig) bool {
		return s3.TTSEnabled
	})
}

func (m *Manager) ActiveBackendForFont() BackendType {
	return m.activeBackendWithToggle(func(s3 utils.S3StorageConfig) bool {
		if s3.FontsEnabled == nil {
			return true
		}
		return *s3.FontsEnabled
	})
}

func (m *Manager) ActiveBackendForTheaterAttachment() BackendType {
	if m == nil || m.cfg.S3.TheaterEnabled == nil {
		return m.ActiveBackendForAttachment()
	}
	return m.activeBackendWithToggle(func(s3 utils.S3StorageConfig) bool {
		return *s3.TheaterEnabled
	})
}

func (m *Manager) ActiveBackendForTheaterAudio() BackendType {
	if m == nil || m.cfg.S3.TheaterEnabled == nil {
		return m.ActiveBackendForAudio()
	}
	return m.activeBackendWithToggle(func(s3 utils.S3StorageConfig) bool {
		return *s3.TheaterEnabled
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
	return m.uploadWithFallback(ctx, m.ActiveBackendForAttachment(), input)
}

func (m *Manager) UploadTTS(ctx context.Context, input UploadInput) (*UploadResult, error) {
	return m.uploadWithFallback(ctx, m.ActiveBackendForTTS(), input)
}

func (m *Manager) UploadTheaterAttachment(ctx context.Context, input UploadInput) (*UploadResult, error) {
	return m.uploadWithFallback(ctx, m.ActiveBackendForTheaterAttachment(), input)
}

func (m *Manager) uploadWithFallback(ctx context.Context, target BackendType, input UploadInput) (*UploadResult, error) {
	if strings.TrimSpace(input.ObjectKey) == "" {
		return nil, fmt.Errorf("objectKey 不能为空")
	}
	input.ContentType = normalizeContentType(input.ContentType, input.ObjectKey)
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

func (m *Manager) UploadWithBackend(ctx context.Context, backend BackendType, input UploadInput) (*UploadResult, error) {
	if strings.TrimSpace(input.ObjectKey) == "" {
		return nil, fmt.Errorf("objectKey 不能为空")
	}
	input.ContentType = normalizeContentType(input.ContentType, input.ObjectKey)
	switch backend {
	case BackendS3:
		if m.remote == nil {
			return nil, fmt.Errorf("未启用 S3 存储")
		}
		return m.remote.upload(ctx, input)
	default:
		return m.local.upload(input)
	}
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

func (m *Manager) DeletePrefix(ctx context.Context, backend BackendType, objectKey string) error {
	switch backend {
	case BackendS3:
		if m.remote == nil {
			return nil
		}
		return m.remote.deletePrefix(ctx, objectKey)
	default:
		return m.local.deletePrefix(objectKey)
	}
}

func (m *Manager) ListS3Prefix(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	if m == nil || m.remote == nil {
		return nil, fmt.Errorf("S3 存储未初始化")
	}
	return m.remote.listPrefix(ctx, prefix)
}

// OpenRead exposes private bytes to authorized server-side callers, never a URL.
func (m *Manager) OpenRead(ctx context.Context, backend BackendType, objectKey string) (io.ReadCloser, error) {
	if m == nil {
		return nil, fmt.Errorf("存储尚未初始化")
	}
	switch backend {
	case BackendLocal:
		path, err := m.ResolveLocalPath(objectKey)
		if err != nil {
			return nil, err
		}
		return os.Open(path)
	case BackendS3:
		if m.remote == nil {
			return nil, fmt.Errorf("未启用 S3 存储")
		}
		return m.remote.openRead(ctx, objectKey)
	default:
		return nil, fmt.Errorf("无效存储类型")
	}
}

func (m *Manager) DownloadToPath(ctx context.Context, backend BackendType, objectKey string, targetPath string) error {
	switch backend {
	case BackendS3:
		if m.remote == nil {
			return fmt.Errorf("未启用 S3 存储")
		}
		return m.remote.downloadToPath(ctx, objectKey, targetPath)
	default:
		return m.local.downloadToPath(objectKey, targetPath)
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

func (m *Manager) PresignedURL(ctx context.Context, backend BackendType, objectKey string) string {
	switch backend {
	case BackendS3:
		if m == nil || m.remote == nil {
			return ""
		}
		ttlSeconds := m.cfg.S3.PresignTTL
		if ttlSeconds <= 0 {
			ttlSeconds = m.cfg.PresignTTL
		}
		if ttlSeconds <= 0 {
			ttlSeconds = 900
		}
		return m.remote.presignedURL(ctx, objectKey, time.Duration(ttlSeconds)*time.Second)
	default:
		return ""
	}
}

func (m *Manager) ResolveAttachmentExportURL(ctx context.Context, backend BackendType, objectKey string) string {
	if target := m.PublicURL(backend, objectKey); target != "" {
		return target
	}
	return m.PresignedURL(ctx, backend, objectKey)
}

func (m *Manager) ResolveReadURL(ctx context.Context, backend BackendType, objectKey string) string {
	if m == nil {
		return ""
	}
	if backend != BackendS3 || m.remote == nil {
		return m.PublicURL(backend, objectKey)
	}
	if m.remote.publicExplicit {
		return m.remote.publicURL(objectKey)
	}
	if target := m.PresignedURL(ctx, backend, objectKey); target != "" {
		return target
	}
	return m.remote.publicURL(objectKey)
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

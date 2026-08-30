package audiolibrarybackend

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"sealchat/utils"
)

var (
	ErrUnavailable  = errors.New("audio library S3 不可用")
	ErrNotFound     = errors.New("audio library S3 对象不存在")
	ErrPermission   = errors.New("audio library S3 权限不足")
	ErrETagMismatch = errors.New("S3 对象已被外部修改")
)

type UploadInput struct {
	ObjectKey   string
	LocalPath   string
	ContentType string
}

type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
	StorageClass string
	ContentType  string
	VersionID    string
}

type ListResult struct {
	Objects     []ObjectInfo
	Prefixes    []string
	NextCursor  string
	IsTruncated bool
}

type Backend interface {
	Put(context.Context, UploadInput) (ObjectInfo, error)
	PutBytes(context.Context, string, string, []byte) (ObjectInfo, error)
	List(context.Context, string, string, int) (ListResult, error)
	Stat(context.Context, string) (ObjectInfo, error)
	Copy(context.Context, string, string, string) (ObjectInfo, error)
	UpdateContentType(context.Context, string, string, string) (ObjectInfo, error)
	DeleteObjects(context.Context, []string) error
	DeletePrefix(context.Context, string) error
	PresignedGetURL(context.Context, string) (string, error)
	BucketLabel() string
}

type Registry struct {
	mu          sync.Mutex
	initialized bool
	config      utils.StorageConfig
	backend     Backend
	initErr     error
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) Get(cfg utils.StorageConfig) (Backend, error) {
	if r == nil {
		return nil, ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.initialized && sameStorageConfig(r.config, cfg) {
		return r.backend, r.initErr
	}
	r.config = cfg
	r.initialized = true
	r.backend, r.initErr = NewS3Backend(cfg)
	return r.backend, r.initErr
}

func (r *Registry) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initialized = false
	r.config = utils.StorageConfig{}
	r.backend = nil
	r.initErr = nil
}

func sameStorageConfig(a, b utils.StorageConfig) bool {
	return a.UploadTimeoutSeconds == b.UploadTimeoutSeconds &&
		a.PresignTTL == b.PresignTTL &&
		sameS3Config(a.S3, b.S3)
}

func sameS3Config(a, b utils.S3StorageConfig) bool {
	return a.Enabled == b.Enabled &&
		boolPtrEqual(a.AttachmentsEnabled, b.AttachmentsEnabled) &&
		boolPtrEqual(a.AudioEnabled, b.AudioEnabled) &&
		boolPtrEqual(a.FontsEnabled, b.FontsEnabled) &&
		boolPtrEqual(a.TheaterEnabled, b.TheaterEnabled) &&
		a.Endpoint == b.Endpoint && a.Region == b.Region && a.Bucket == b.Bucket &&
		a.AccessKey == b.AccessKey && a.SecretKey == b.SecretKey && a.SessionToken == b.SessionToken &&
		a.ForcePathStyle == b.ForcePathStyle && a.BaseURL == b.BaseURL &&
		a.PublicBaseURL == b.PublicBaseURL && a.UseSSL == b.UseSSL &&
		a.PresignTTL == b.PresignTTL && a.MaxSizeMB == b.MaxSizeMB && a.LogLevel == b.LogLevel
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func unavailablef(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnavailable, fmt.Sprintf(format, args...))
}

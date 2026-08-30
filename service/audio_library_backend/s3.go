package audiolibrarybackend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"sealchat/utils"
)

const (
	defaultOperationTimeout = 20 * time.Second
	defaultPresignTTL       = 15 * time.Minute
	v2CursorPrefix          = "v2:"
)

type S3Backend struct {
	client           *minio.Client
	core             *minio.Core
	bucket           string
	operationTimeout time.Duration
	presignTTL       time.Duration
}

func NewS3Backend(cfg utils.StorageConfig) (*S3Backend, error) {
	if !cfg.S3.Enabled {
		return nil, unavailablef("未启用 S3 存储")
	}
	if cfg.S3.AudioEnabled != nil && !*cfg.S3.AudioEnabled {
		return nil, unavailablef("未启用 S3 音频存储")
	}
	if strings.TrimSpace(cfg.S3.Endpoint) == "" || strings.TrimSpace(cfg.S3.Bucket) == "" {
		return nil, unavailablef("S3 配置不完整")
	}
	endpoint, secure := normalizeEndpoint(cfg.S3.Endpoint, cfg.S3.UseSSL)
	if !cfg.S3.ForcePathStyle {
		endpoint = stripBucketHost(endpoint, cfg.S3.Bucket)
	}
	opts := &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.S3.AccessKey, cfg.S3.SecretKey, cfg.S3.SessionToken),
		Secure:       secure,
		Region:       strings.TrimSpace(cfg.S3.Region),
		BucketLookup: bucketLookupForDirectEndpoint(cfg.S3.ForcePathStyle),
	}
	client, err := minio.New(endpoint, opts)
	if err != nil {
		return nil, unavailablef("初始化 S3 client 失败: %v", err)
	}
	timeout := time.Duration(cfg.UploadTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultOperationTimeout
	}
	ttl := time.Duration(cfg.S3.PresignTTL) * time.Second
	if ttl <= 0 {
		ttl = time.Duration(cfg.PresignTTL) * time.Second
	}
	if ttl <= 0 {
		ttl = defaultPresignTTL
	}
	return &S3Backend{
		client:           client,
		core:             &minio.Core{Client: client},
		bucket:           cfg.S3.Bucket,
		operationTimeout: timeout,
		presignTTL:       ttl,
	}, nil
}

func (s *S3Backend) Put(ctx context.Context, input UploadInput) (ObjectInfo, error) {
	if s == nil || s.client == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	if strings.TrimSpace(input.ObjectKey) == "" {
		return ObjectInfo{}, fmt.Errorf("objectKey 不能为空")
	}
	if strings.TrimSpace(input.LocalPath) == "" {
		return ObjectInfo{}, fmt.Errorf("local path 不能为空")
	}
	opCtx, cancel := s.operationContext(ctx)
	defer cancel()
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	info, err := s.client.FPutObject(opCtx, s.bucket, input.ObjectKey, input.LocalPath, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return ObjectInfo{}, normalizeError(err)
	}
	return ObjectInfo{Key: input.ObjectKey, Size: info.Size, ETag: info.ETag, VersionID: info.VersionID}, nil
}

func (s *S3Backend) PutBytes(ctx context.Context, objectKey, contentType string, payload []byte) (ObjectInfo, error) {
	if s == nil || s.client == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	if strings.TrimSpace(objectKey) == "" {
		return ObjectInfo{}, fmt.Errorf("objectKey 不能为空")
	}
	opCtx, cancel := s.operationContext(ctx)
	defer cancel()
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	info, err := s.client.PutObject(opCtx, s.bucket, objectKey, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return ObjectInfo{}, normalizeError(err)
	}
	return ObjectInfo{Key: objectKey, Size: info.Size, ETag: info.ETag, VersionID: info.VersionID}, nil
}

func (s *S3Backend) List(ctx context.Context, prefix, cursor string, limit int) (ListResult, error) {
	if s == nil || s.core == nil {
		return ListResult{}, ErrUnavailable
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ListResult{}, ctx.Err()
	default:
	}
	toListResult := func(objects []minio.ObjectInfo, commonPrefixes []minio.CommonPrefix, nextCursor string, truncated bool) ListResult {
		items := make([]ObjectInfo, 0, len(objects))
		for _, object := range objects {
			items = append(items, ObjectInfo{
				Key: object.Key, Size: object.Size, LastModified: object.LastModified,
				ETag: object.ETag, StorageClass: object.StorageClass,
				ContentType: object.ContentType, VersionID: object.VersionID,
			})
		}
		prefixes := make([]string, 0, len(commonPrefixes))
		for _, item := range commonPrefixes {
			prefixes = append(prefixes, item.Prefix)
		}
		return ListResult{Objects: items, Prefixes: prefixes, NextCursor: nextCursor, IsTruncated: truncated}
	}

	rawCursor := strings.TrimSpace(cursor)
	if strings.HasPrefix(rawCursor, v2CursorPrefix) {
		v2, v2Err := s.core.ListObjectsV2(s.bucket, prefix, "", strings.TrimPrefix(rawCursor, v2CursorPrefix), "/", limit)
		if v2Err != nil {
			return ListResult{}, normalizeError(v2Err)
		}
		nextCursor := ""
		if v2.NextContinuationToken != "" {
			nextCursor = v2CursorPrefix + v2.NextContinuationToken
		}
		return toListResult(v2.Contents, v2.CommonPrefixes, nextCursor, v2.IsTruncated), nil
	}

	// COS documents and implements bucket listing as List Objects V1
	// (marker/NextMarker). Keep this choice inside the dedicated audio client;
	// the shared S3 compatibility backend remains unchanged.
	result, err := s.core.ListObjects(s.bucket, prefix, rawCursor, "/", limit)
	if err != nil {
		return ListResult{}, normalizeError(err)
	}

	// A few S3-compatible gateways intermittently return an empty V1 page for
	// a valid prefix while V2 returns the actual CommonPrefixes. Retry with V2
	// only for the first page; cursor is tagged so later pages use same API.
	if rawCursor == "" && len(result.Contents) == 0 && len(result.CommonPrefixes) == 0 {
		v2, v2Err := s.core.ListObjectsV2(s.bucket, prefix, "", "", "/", limit)
		if v2Err == nil && (len(v2.Contents) > 0 || len(v2.CommonPrefixes) > 0) {
			nextCursor := ""
			if v2.NextContinuationToken != "" {
				nextCursor = v2CursorPrefix + v2.NextContinuationToken
			}
			return toListResult(v2.Contents, v2.CommonPrefixes, nextCursor, v2.IsTruncated), nil
		}
	}
	return toListResult(result.Contents, result.CommonPrefixes, result.NextMarker, result.IsTruncated), nil
}

func (s *S3Backend) Stat(ctx context.Context, objectKey string) (ObjectInfo, error) {
	if s == nil || s.client == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	opCtx, cancel := s.operationContext(ctx)
	defer cancel()
	object, err := s.client.StatObject(opCtx, s.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, normalizeError(err)
	}
	return ObjectInfo{
		Key: object.Key, Size: object.Size, LastModified: object.LastModified,
		ETag: object.ETag, StorageClass: object.StorageClass,
		ContentType: object.ContentType, VersionID: object.VersionID,
	}, nil
}

func (s *S3Backend) Copy(ctx context.Context, sourceKey, destinationKey, expectedETag string) (ObjectInfo, error) {
	if s == nil || s.core == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	opCtx, cancel := s.operationContext(ctx)
	defer cancel()
	source := minio.CopySrcOptions{Bucket: s.bucket, Object: sourceKey, MatchETag: strings.TrimSpace(expectedETag)}
	if _, err := s.core.CopyObject(opCtx, s.bucket, sourceKey, s.bucket, destinationKey, nil, source, minio.PutObjectOptions{}); err != nil {
		if strings.TrimSpace(expectedETag) != "" {
			response := minio.ToErrorResponse(err)
			if response.StatusCode == http.StatusPreconditionFailed || response.Code == "PreconditionFailed" {
				return ObjectInfo{}, ErrETagMismatch
			}
		}
		return ObjectInfo{}, normalizeError(err)
	}
	return s.Stat(opCtx, destinationKey)
}

func (s *S3Backend) UpdateContentType(ctx context.Context, objectKey, contentType, expectedETag string) (ObjectInfo, error) {
	if s == nil || s.core == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return ObjectInfo{}, fmt.Errorf("Content-Type 不能为空")
	}
	opCtx, cancel := s.operationContext(ctx)
	defer cancel()
	current, err := s.Stat(opCtx, objectKey)
	if err != nil {
		return ObjectInfo{}, err
	}
	if strings.TrimSpace(expectedETag) != "" && trimETag(current.ETag) != trimETag(expectedETag) {
		return ObjectInfo{}, ErrETagMismatch
	}
	if _, err := s.core.CopyObject(opCtx, s.bucket, objectKey, s.bucket, objectKey, map[string]string{
		"Content-Type":             contentType,
		"x-amz-metadata-directive": "REPLACE",
	}, minio.CopySrcOptions{Bucket: s.bucket, Object: objectKey, MatchETag: strings.TrimSpace(expectedETag)}, minio.PutObjectOptions{}); err != nil {
		if strings.TrimSpace(expectedETag) != "" {
			response := minio.ToErrorResponse(err)
			if response.StatusCode == http.StatusPreconditionFailed || response.Code == "PreconditionFailed" {
				return ObjectInfo{}, ErrETagMismatch
			}
		}
		return ObjectInfo{}, normalizeError(err)
	}
	return s.Stat(opCtx, objectKey)
}

func (s *S3Backend) DeleteObjects(ctx context.Context, keys []string) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	if len(keys) == 0 {
		return nil
	}
	opCtx, cancel := s.operationContext(ctx)
	defer cancel()
	objects := make(chan minio.ObjectInfo, len(keys))
	go func() {
		defer close(objects)
		for _, key := range keys {
			if strings.TrimSpace(key) != "" {
				objects <- minio.ObjectInfo{Key: key}
			}
		}
	}()
	for result := range s.client.RemoveObjects(opCtx, s.bucket, objects, minio.RemoveObjectsOptions{}) {
		if result.Err != nil {
			return normalizeError(result.Err)
		}
	}
	return nil
}

func (s *S3Backend) DeletePrefix(ctx context.Context, prefix string) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	prefix = strings.TrimLeft(prefix, "/")
	if prefix == "" {
		return fmt.Errorf("禁止删除 bucket 根目录")
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	opCtx, cancel := s.operationContext(ctx)
	defer cancel()
	batch := make([]string, 0, 1000)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := s.deleteObjectsWithContext(opCtx, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}
	for object := range s.client.ListObjects(opCtx, s.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true, MaxKeys: 1000}) {
		if object.Err != nil {
			return normalizeError(object.Err)
		}
		batch = append(batch, object.Key)
		if len(batch) == 1000 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func (s *S3Backend) deleteObjectsWithContext(ctx context.Context, keys []string) error {
	objects := make(chan minio.ObjectInfo, len(keys))
	go func() {
		defer close(objects)
		for _, key := range keys {
			objects <- minio.ObjectInfo{Key: key}
		}
	}()
	for result := range s.client.RemoveObjects(ctx, s.bucket, objects, minio.RemoveObjectsOptions{}) {
		if result.Err != nil {
			return normalizeError(result.Err)
		}
	}
	return nil
}

func (s *S3Backend) PresignedGetURL(ctx context.Context, objectKey string) (string, error) {
	if s == nil || s.client == nil {
		return "", ErrUnavailable
	}
	if strings.TrimSpace(objectKey) == "" {
		return "", fmt.Errorf("objectKey 不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := s.client.PresignedGetObject(ctx, s.bucket, objectKey, s.presignTTL, nil)
	if err != nil || target == nil {
		if err == nil {
			err = fmt.Errorf("生成 S3 预签名 URL 失败")
		}
		return "", normalizeError(err)
	}
	return target.String(), nil
}

func (s *S3Backend) BucketLabel() string {
	if s == nil {
		return ""
	}
	return s.bucket
}

func (s *S3Backend) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := s.operationTimeout
	if timeout <= 0 {
		timeout = defaultOperationTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func trimETag(value string) string {
	return strings.Trim(strings.TrimSpace(value), "\"")
}

func normalizeEndpoint(endpoint string, useSSL bool) (string, bool) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return endpoint, useSSL
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return trimmed, useSSL
		}
		return u.Host, u.Scheme == "https"
	}
	return trimmed, useSSL
}

// Some COS configurations store bucket-qualified endpoints while the client
// also receives bucket separately. Remove duplicate host label before the
// dedicated client performs explicit virtual-host lookup.
func stripBucketHost(endpoint, bucket string) string {
	endpoint = strings.TrimSpace(endpoint)
	bucket = strings.TrimSpace(bucket)
	if endpoint == "" || bucket == "" {
		return endpoint
	}
	if strings.HasPrefix(endpoint, bucket+".") {
		return strings.TrimPrefix(endpoint, bucket+".")
	}
	return endpoint
}

// The audio library owns a dedicated S3 client. Do not leave lookup mode on
// MinIO's Auto default: custom S3 endpoints (for example COS) are otherwise
// sent path-style even when pathStyle is disabled.
func bucketLookupForDirectEndpoint(forcePathStyle bool) minio.BucketLookupType {
	if forcePathStyle {
		return minio.BucketLookupPath
	}
	return minio.BucketLookupDNS
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrPermission) || errors.Is(err, ErrETagMismatch) {
		return err
	}
	response := minio.ToErrorResponse(err)
	if response.StatusCode == http.StatusNotFound || response.Code == "NoSuchKey" || response.Code == "NoSuchObject" {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusUnauthorized || response.Code == "AccessDenied" {
		return fmt.Errorf("%w: %v", ErrPermission, err)
	}
	return err
}

var _ Backend = (*S3Backend)(nil)

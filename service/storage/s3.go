package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"sealchat/utils"
)

type s3Backend struct {
	client              *minio.Client
	listCompatClient    *minio.Client
	bucket              string
	publicBaseURL       string
	publicExplicit      bool
	forcePathStyle      bool
	uploadTimeout       time.Duration
	privateReadVerified bool
	presignURLFunc      func(context.Context, string, time.Duration) (string, error)
}

const defaultS3UploadTimeout = 20 * time.Second

var ErrTTSPrivateReadUnverified = errors.New("S3 连接正常，但 TTS 结果要求私有对象存储；当前 Bucket/CDN 无法确认禁止匿名读取")

// TestS3 uses the same bucket write/read/delete self-check as initialization,
// without replacing the running manager or persisting configuration.
func TestS3(cfg utils.StorageConfig) error {
	remote, err := newS3Backend(cfg.S3, time.Duration(cfg.UploadTimeoutSeconds)*time.Second)
	if err != nil {
		return err
	}
	if cfg.S3.TTSEnabled && !remote.privateReadVerified {
		return ErrTTSPrivateReadUnverified
	}
	return nil
}

func newS3Backend(cfg utils.S3StorageConfig, uploadTimeout time.Duration) (*s3Backend, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("S3 配置不完整")
	}
	endpoint, secure := normalizeEndpoint(cfg.Endpoint, cfg.UseSSL)
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken),
		Secure: secure,
		Region: strings.TrimSpace(cfg.Region),
	}
	if cfg.ForcePathStyle {
		opts.BucketLookup = minio.BucketLookupPath
	}
	client, err := minio.New(endpoint, opts)
	if err != nil {
		return nil, err
	}
	if err := verifyS3ReadWrite(client, cfg.Bucket); err != nil {
		return nil, fmt.Errorf("S3 自检失败: %w", err)
	}
	var listCompatClient *minio.Client
	if !cfg.ForcePathStyle {
		compatEndpoint := stripBucketHost(endpoint, cfg.Bucket)
		compatOpts := &minio.Options{
			Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken),
			Secure:       secure,
			Region:       strings.TrimSpace(cfg.Region),
			BucketLookup: minio.BucketLookupDNS,
		}
		if compatClient, compatErr := minio.New(compatEndpoint, compatOpts); compatErr == nil {
			listCompatClient = compatClient
		}
	}
	if uploadTimeout <= 0 {
		uploadTimeout = defaultS3UploadTimeout
	}
	publicBase := strings.TrimSpace(cfg.PublicBaseURL)
	if publicBase == "" {
		// Match StorageConfig.normalize's legacy public URL fallback.
		publicBase = strings.TrimSpace(cfg.BaseURL)
	}
	publicExplicit := publicBase != ""
	directBase := derivePublicURL(endpoint, secure, cfg.Bucket, cfg.ForcePathStyle)
	if publicBase == "" {
		publicBase = directBase
	}
	backend := &s3Backend{
		client:           client,
		listCompatClient: listCompatClient,
		bucket:           cfg.Bucket,
		publicBaseURL:    strings.TrimRight(publicBase, "/"),
		publicExplicit:   publicExplicit,
		forcePathStyle:   cfg.ForcePathStyle,
		uploadTimeout:    uploadTimeout,
	}
	if cfg.TTSEnabled {
		publicBases := []string{strings.TrimRight(directBase, "/")}
		if publicExplicit && backend.publicBaseURL != publicBases[0] {
			publicBases = append(publicBases, backend.publicBaseURL)
		}
		backend.privateReadVerified = backend.verifyTTSPrivateRead(publicBases)
	}
	return backend, nil
}

func (s *s3Backend) upload(ctx context.Context, input UploadInput) (*UploadResult, error) {
	if strings.TrimSpace(input.ObjectKey) == "" {
		return nil, fmt.Errorf("objectKey 不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	uploadTimeout := s.uploadTimeout
	if uploadTimeout <= 0 {
		uploadTimeout = defaultS3UploadTimeout
	}
	uploadCtx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()
	opts := minio.PutObjectOptions{
		ContentType: strings.TrimSpace(input.ContentType),
	}
	if opts.ContentType == "" {
		opts.ContentType = "application/octet-stream"
	}
	info, err := s.client.FPutObject(uploadCtx, s.bucket, input.ObjectKey, input.LocalPath, opts)
	if err != nil {
		return nil, err
	}
	return &UploadResult{
		Backend:   BackendS3,
		ObjectKey: input.ObjectKey,
		Size:      info.Size,
		PublicURL: s.publicURL(input.ObjectKey),
	}, nil
}

func (s *s3Backend) exists(ctx context.Context, objectKey string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	statTimeout := s.uploadTimeout
	if statTimeout <= 0 {
		statTimeout = defaultS3UploadTimeout
	}
	statCtx, cancel := context.WithTimeout(ctx, statTimeout)
	_, err := s.client.StatObject(statCtx, s.bucket, objectKey, minio.StatObjectOptions{})
	cancel()
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *s3Backend) delete(ctx context.Context, objectKey string) error {
	err := s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		resp := minio.ToErrorResponse(err)
		if resp.StatusCode == 404 {
			return nil
		}
		return err
	}
	return nil
}

func (s *s3Backend) deletePrefix(ctx context.Context, prefix string) error {
	if strings.TrimSpace(prefix) == "" {
		return nil
	}
	objects := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    strings.TrimLeft(prefix, "/"),
		Recursive: true,
	})
	for object := range objects {
		if object.Err != nil {
			resp := minio.ToErrorResponse(object.Err)
			if resp.StatusCode == 404 {
				continue
			}
			return object.Err
		}
		if err := s.client.RemoveObject(ctx, s.bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
			resp := minio.ToErrorResponse(err)
			if resp.StatusCode == 404 {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *s3Backend) listPrefixOnce(ctx context.Context, client *minio.Client, normalizedPrefix string, useV1 bool) ([]ObjectInfo, error) {
	if client == nil {
		return nil, fmt.Errorf("S3 listing client is nil")
	}
	if s == nil {
		return nil, fmt.Errorf("S3 存储未初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	listTimeout := s.uploadTimeout
	if listTimeout <= 0 {
		listTimeout = defaultS3UploadTimeout
	}
	listCtx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	objects := client.ListObjects(listCtx, s.bucket, minio.ListObjectsOptions{
		Prefix:    normalizedPrefix,
		Recursive: true,
		UseV1:     useV1,
	})
	items := make([]ObjectInfo, 0)
	for object := range objects {
		if object.Err != nil {
			return nil, object.Err
		}
		if !strings.HasPrefix(object.Key, normalizedPrefix) {
			continue
		}
		items = append(items, ObjectInfo{
			ObjectKey:  object.Key,
			Size:       object.Size,
			ModifiedAt: object.LastModified,
		})
	}
	return items, nil
}

func isS3ListAddressingCompatError(err error) bool {
	if err == nil {
		return false
	}
	resp := minio.ToErrorResponse(err)
	switch resp.Code {
	case "NoSuchKey", "NoSuchObject":
		return true
	case "NoSuchBucket", "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch":
		return false
	}
	if resp.StatusCode == 404 && strings.Contains(strings.ToLower(err.Error()), "specified key does not exist") {
		return true
	}
	return false
}

func (s *s3Backend) listPrefixV1First(ctx context.Context, client *minio.Client, prefix string) ([]ObjectInfo, error) {
	v1Items, v1Err := s.listPrefixOnce(ctx, client, prefix, true)
	if v1Err != nil {
		return nil, v1Err
	}
	if len(v1Items) > 0 {
		return v1Items, nil
	}
	v2Items, v2Err := s.listPrefixOnce(ctx, client, prefix, false)
	if v2Err == nil && len(v2Items) > 0 {
		return v2Items, nil
	}
	return v1Items, nil
}

func (s *s3Backend) listPrefixCompat(ctx context.Context, normalizedPrefix string) ([]ObjectInfo, error) {
	if s == nil {
		return nil, fmt.Errorf("S3 存储未初始化")
	}
	items, err := s.listPrefixV1First(ctx, s.listCompatClient, normalizedPrefix)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		return items, nil
	}

	bucketPrefix := strings.Trim(strings.TrimSpace(s.bucket), "/")
	if bucketPrefix == "" {
		return items, nil
	}
	physicalPrefix := bucketPrefix + "/" + normalizedPrefix
	physicalItems, err := s.listPrefixV1First(ctx, s.listCompatClient, physicalPrefix)
	if err != nil || len(physicalItems) == 0 {
		return items, nil
	}

	physicalRoot := bucketPrefix + "/"
	logicalItems := make([]ObjectInfo, 0, len(physicalItems))
	for _, item := range physicalItems {
		logicalKey, ok := strings.CutPrefix(item.ObjectKey, physicalRoot)
		if !ok || !strings.HasPrefix(logicalKey, normalizedPrefix) {
			continue
		}
		logicalItems = append(logicalItems, ObjectInfo{
			ObjectKey:  logicalKey,
			Size:       item.Size,
			ModifiedAt: item.ModifiedAt,
		})
	}
	return logicalItems, nil
}

func (s *s3Backend) listPrefix(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	if s == nil {
		return nil, fmt.Errorf("S3 存储未初始化")
	}
	normalizedPrefix := strings.TrimLeft(strings.TrimSpace(prefix), "/")

	v1Items, v1Err := s.listPrefixOnce(ctx, s.client, normalizedPrefix, true)
	if v1Err != nil {
		if !isS3ListAddressingCompatError(v1Err) || s.listCompatClient == nil {
			return nil, v1Err
		}
		return s.listPrefixCompat(ctx, normalizedPrefix)
	}
	if len(v1Items) > 0 {
		return v1Items, nil
	}

	v2Items, v2Err := s.listPrefixOnce(ctx, s.client, normalizedPrefix, false)
	if v2Err == nil && len(v2Items) > 0 {
		return v2Items, nil
	}
	if isS3ListAddressingCompatError(v2Err) && s.listCompatClient != nil {
		return s.listPrefixCompat(ctx, normalizedPrefix)
	}
	return v1Items, nil
}

func (s *s3Backend) publicURL(objectKey string) string {
	if s.publicBaseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", s.publicBaseURL, strings.TrimLeft(objectKey, "/"))
}

func (s *s3Backend) presignedURL(ctx context.Context, objectKey string, ttl time.Duration) string {
	if s == nil || strings.TrimSpace(objectKey) == "" {
		return ""
	}
	if s.presignURLFunc != nil {
		target, err := s.presignURLFunc(ctx, objectKey, ttl)
		if err == nil {
			return strings.TrimSpace(target)
		}
		return ""
	}
	if s.client == nil || ttl <= 0 {
		return ""
	}
	target, err := s.client.PresignedGetObject(ctx, s.bucket, objectKey, ttl, nil)
	if err != nil || target == nil {
		return ""
	}
	return target.String()
}

func (s *s3Backend) openRead(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("S3 存储未初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	statTimeout := s.uploadTimeout
	if statTimeout <= 0 {
		statTimeout = defaultS3UploadTimeout
	}
	statCtx, cancel := context.WithTimeout(ctx, statTimeout)
	// GetObject is lazy: fail before installing the response stream if missing.
	_, err := s.client.StatObject(statCtx, s.bucket, objectKey, minio.StatObjectOptions{})
	cancel()
	if err != nil {
		return nil, err
	}
	// Streaming uses the caller's context, without the metadata deadline.
	return s.client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
}

func (s *s3Backend) downloadToPath(ctx context.Context, objectKey string, targetPath string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("S3 存储未初始化")
	}
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return fmt.Errorf("objectKey 不能为空")
	}
	reader, err := s.client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	output, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := io.Copy(output, reader); err != nil {
		return err
	}
	return nil
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

func derivePublicURL(endpoint string, secure bool, bucket string, pathStyle bool) string {
	protocol := "https"
	if !secure {
		protocol = "http"
	}
	if pathStyle {
		return fmt.Sprintf("%s://%s/%s", protocol, endpoint, bucket)
	}
	return fmt.Sprintf("%s://%s.%s", protocol, bucket, endpoint)
}

func logS3Fallback(err error) {
	if err == nil {
		return
	}
	log.Printf("[storage] S3 操作失败，已回退到本地: %v", err)
}

func (s *s3Backend) verifyTTSPrivateRead(publicBases []string) bool {
	if s == nil || s.client == nil || len(publicBases) == 0 {
		return false
	}
	rnd := make([]byte, 12)
	if _, err := rand.Read(rnd); err != nil {
		return false
	}
	// Match the exact key shape used by persisted TTS job_audio objects so
	// prefix/path policies cannot treat the privacy probe more strictly.
	key := "tts-private/resources/" + hex.EncodeToString(rnd)
	payload := []byte("sealchat-tts-privacy-check")
	probeTimeout := s.uploadTimeout
	if probeTimeout <= 0 {
		probeTimeout = defaultS3UploadTimeout
	}
	putCtx, putCancel := context.WithTimeout(context.Background(), probeTimeout)
	_, err := s.client.PutObject(putCtx, s.bucket, key, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{ContentType: "text/plain"})
	putCancel()
	if err != nil {
		return false
	}
	defer func() {
		deleteCtx, deleteCancel := context.WithTimeout(context.Background(), probeTimeout)
		defer deleteCancel()
		_ = s.client.RemoveObject(deleteCtx, s.bucket, key, minio.RemoveObjectOptions{})
	}()
	verified := true
	for _, publicBase := range publicBases {
		if !verifyS3PrivateRead(publicBase, key, payload) {
			verified = false
		}
	}
	return verified
}

func verifyS3PrivateRead(publicBaseURL, objectKey string, payload []byte) bool {
	target, err := url.Parse(publicBaseURL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil || target.RawQuery != "" || target.ForceQuery || target.Fragment != "" {
		return false
	}
	target.Path = strings.TrimRight(target.Path, "/") + "/" + objectKey
	target.RawPath = ""
	req, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return false
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		// Redirects may lead to login pages or signed URLs; neither proves privacy.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return true
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(len(payload))+1))
		if readErr == nil && bytes.Equal(data, payload) {
			return false // The TTS privacy probe is anonymously readable.
		}
	}
	// Missing, mismatched or unreachable objects do not establish private reads.
	return false
}

func verifyS3ReadWrite(client *minio.Client, bucket string) error {
	if client == nil {
		return fmt.Errorf("minio client is nil")
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return fmt.Errorf("bucket is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	payload := []byte("sealchat-s3-healthcheck")
	rnd := make([]byte, 12)
	if _, err := rand.Read(rnd); err != nil {
		return fmt.Errorf("rand: %w", err)
	}
	key := path.Clean(path.Join("sealchat", "_healthcheck", fmt.Sprintf("%d-%s.txt", time.Now().UnixNano(), hex.EncodeToString(rnd))))

	putInfo, err := client.PutObject(ctx, bucket, key, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	defer func() {
		_ = client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	}()

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		obj, err := client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
		if err != nil {
			lastErr = fmt.Errorf("get: %w", err)
		} else {
			limited := io.LimitReader(obj, int64(len(payload))+1)
			data, readErr := io.ReadAll(limited)
			_ = obj.Close()
			if readErr != nil {
				lastErr = fmt.Errorf("read: %w", readErr)
			} else if len(data) != len(payload) || !bytes.Equal(data, payload) {
				lastErr = fmt.Errorf("read mismatch: got=%d want=%d", len(data), len(payload))
			} else {
				lastErr = nil
				break
			}
		}
		if attempt < 2 {
			time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
		}
	}
	if lastErr != nil {
		return lastErr
	}

	if err := client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if putInfo.Size != int64(len(payload)) && putInfo.Size != 0 {
		// Some S3-compatible backends may not return size reliably, so only flag obviously wrong values.
		return fmt.Errorf("unexpected put size: %d", putInfo.Size)
	}
	return nil
}

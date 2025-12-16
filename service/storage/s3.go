package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"sealchat/utils"
)

type s3Backend struct {
	client         *minio.Client
	bucket         string
	publicBaseURL  string
	forcePathStyle bool
}

func newS3Backend(cfg utils.S3StorageConfig) (*s3Backend, error) {
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
	publicBase := strings.TrimSpace(cfg.PublicBaseURL)
	if publicBase == "" {
		publicBase = derivePublicURL(endpoint, secure, cfg.Bucket, cfg.ForcePathStyle)
	}
	return &s3Backend{
		client:         client,
		bucket:         cfg.Bucket,
		publicBaseURL:  strings.TrimRight(publicBase, "/"),
		forcePathStyle: cfg.ForcePathStyle,
	}, nil
}

func (s *s3Backend) upload(ctx context.Context, input UploadInput) (*UploadResult, error) {
	if strings.TrimSpace(input.ObjectKey) == "" {
		return nil, fmt.Errorf("objectKey 不能为空")
	}
	opts := minio.PutObjectOptions{
		ContentType: strings.TrimSpace(input.ContentType),
	}
	if opts.ContentType == "" {
		opts.ContentType = "application/octet-stream"
	}
	info, err := s.client.FPutObject(ctx, s.bucket, input.ObjectKey, input.LocalPath, opts)
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
	_, err := s.client.StatObject(ctx, s.bucket, objectKey, minio.StatObjectOptions{})
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

func (s *s3Backend) publicURL(objectKey string) string {
	if s.publicBaseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", s.publicBaseURL, strings.TrimLeft(objectKey, "/"))
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

package storage

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type BackendType string

const (
	BackendLocal BackendType = "local"
	BackendS3    BackendType = "s3"
)

type UploadInput struct {
	ObjectKey   string
	LocalPath   string
	ContentType string
}

type UploadResult struct {
	Backend   BackendType
	ObjectKey string
	Size      int64
	PublicURL string
}

var unsafeNamePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func BuildAttachmentObjectKey(hashHex string, size int64, now time.Time) string {
	cleanHash := sanitizeName(hashHex)
	if cleanHash == "" {
		cleanHash = fmt.Sprintf("%d", now.UnixNano())
	}
	if size <= 0 {
		size = 0
	}
	return path.Clean(path.Join("attachments",
		now.UTC().Format("2006"),
		now.UTC().Format("01"),
		fmt.Sprintf("%s_%d", cleanHash, size),
	))
}

func BuildAudioObjectKey(assetID string, originalName string) string {
	name := sanitizeName(filepath.Base(originalName))
	if name == "" {
		name = "audio"
	}
	return path.Clean(path.Join("audio", sanitizeName(assetID), name))
}

func sanitizeName(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	return unsafeNamePattern.ReplaceAllString(trimmed, "_")
}

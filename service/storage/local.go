package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type localBackend struct {
	attachmentRoot string
	audioRoot      string
}

func newLocalBackend(uploadDir, audioDir string) (*localBackend, error) {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "./data/upload"
	}
	if strings.TrimSpace(audioDir) == "" {
		audioDir = "./static/audio"
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建附件目录失败: %w", err)
	}
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建音频目录失败: %w", err)
	}
	return &localBackend{
		attachmentRoot: uploadDir,
		audioRoot:      audioDir,
	}, nil
}

func (l *localBackend) resolvePath(objectKey string) (string, error) {
	clean := filepath.Clean(objectKey)
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("非法 object key")
	}
	switch {
	case strings.HasPrefix(clean, "attachments/"):
		return filepath.Join(l.attachmentRoot, strings.TrimPrefix(clean, "attachments/")), nil
	case strings.HasPrefix(clean, "audio/"):
		return filepath.Join(l.audioRoot, strings.TrimPrefix(clean, "audio/")), nil
	default:
		return filepath.Join(l.attachmentRoot, clean), nil
	}
}

func (l *localBackend) upload(input UploadInput) (*UploadResult, error) {
	target, err := l.resolvePath(input.ObjectKey)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(target); err == nil {
		_ = os.Remove(input.LocalPath)
		info, _ := os.Stat(target)
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		return &UploadResult{
			Backend:   BackendLocal,
			ObjectKey: input.ObjectKey,
			Size:      size,
		}, nil
	}
	if err := os.Rename(input.LocalPath, target); err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	return &UploadResult{
		Backend:   BackendLocal,
		ObjectKey: input.ObjectKey,
		Size:      info.Size(),
	}, nil
}

func (l *localBackend) exists(objectKey string) (bool, error) {
	target, err := l.resolvePath(objectKey)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (l *localBackend) delete(objectKey string) error {
	target, err := l.resolvePath(objectKey)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

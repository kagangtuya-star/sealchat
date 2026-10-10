package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sealchat/utils"
)

type localBackend struct {
	attachmentRoot string
	audioRoot      string
	fontRoot       string
}

func newLocalBackend(uploadDir, audioDir, fontDir string) (*localBackend, error) {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "./data/upload"
	}
	if strings.TrimSpace(audioDir) == "" {
		audioDir = "./static/audio"
	}
	if strings.TrimSpace(fontDir) == "" {
		fontDir = "./data/fonts"
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建附件目录失败: %w", err)
	}
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建音频目录失败: %w", err)
	}
	if err := os.MkdirAll(fontDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建字体目录失败: %w", err)
	}
	return &localBackend{
		attachmentRoot: uploadDir,
		audioRoot:      audioDir,
		fontRoot:       fontDir,
	}, nil
}

func (l *localBackend) resolvePath(objectKey string) (string, error) {
	clean := filepath.Clean(objectKey)
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("非法 object key")
	}
	switch {
	case strings.HasPrefix(clean, "tts-private/"):
		// A sibling of the upload root, never below an attachment static route.
		return filepath.Join(filepath.Dir(l.attachmentRoot), ".sealchat-tts-private", strings.TrimPrefix(clean, "tts-private/")), nil
	case strings.HasPrefix(clean, "attachments/"):
		return filepath.Join(l.attachmentRoot, strings.TrimPrefix(clean, "attachments/")), nil
	case strings.HasPrefix(clean, "audio/"):
		return filepath.Join(l.audioRoot, strings.TrimPrefix(clean, "audio/")), nil
	case strings.HasPrefix(clean, "fonts/"):
		return filepath.Join(l.fontRoot, strings.TrimPrefix(clean, "fonts/")), nil
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
	if err := utils.MoveFile(input.LocalPath, target); err != nil {
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

func (l *localBackend) deletePrefix(objectKey string) error {
	target, err := l.resolvePath(objectKey)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *localBackend) downloadToPath(objectKey string, targetPath string) error {
	source, err := l.resolvePath(objectKey)
	if err != nil {
		return err
	}
	input, err := os.Open(source)
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

func (l *localBackend) resolveLegacyAttachmentPath(fileName string) (string, error) {
	fileName = strings.TrimSpace(filepath.Base(fileName))
	if fileName == "" || fileName == "." {
		return "", fmt.Errorf("非法旧附件文件名")
	}
	return filepath.Join(l.attachmentRoot, fileName), nil
}

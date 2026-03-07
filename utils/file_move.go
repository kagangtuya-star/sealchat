package utils

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

var moveFileRename = os.Rename

// MoveFile 优先尝试 rename；若遇到跨设备移动(EXDEV)则降级为“复制到目标目录临时文件后再替换”。
func MoveFile(src, dst string) error {
	if err := moveFileRename(src, dst); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	return moveFileAcrossDevices(src, dst)
}

func moveFileAcrossDevices(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(dst), ".move-*")
	if err != nil {
		return fmt.Errorf("创建跨设备移动临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmpFile.Chmod(info.Mode().Perm()); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("跨设备移动写入目标文件失败: %w", err)
	}
	cleanup = false

	if err := os.Remove(src); err != nil {
		return err
	}
	return nil
}

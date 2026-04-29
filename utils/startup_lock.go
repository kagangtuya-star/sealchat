package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const startupLockFileName = "sealchat.lock"

var ErrStartupLockExists = errors.New("startup lock exists")

var startupLockProcessExists = defaultStartupLockProcessExists

type StartupLockInfo struct {
	PID       int    `json:"pid"`
	Exe       string `json:"exe,omitempty"`
	CWD       string `json:"cwd,omitempty"`
	Version   string `json:"version,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
}

type StartupLock struct {
	Path string
}

func StartupLockPathForExecutable(exePath string) (string, error) {
	if exePath == "" {
		return "", errors.New("executable path is empty")
	}
	return filepath.Join(filepath.Dir(exePath), startupLockFileName), nil
}

func BuildStartupLockInfo(version string) StartupLockInfo {
	exe, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
		exe = resolved
	}
	cwd, _ := os.Getwd()
	return StartupLockInfo{
		PID:       os.Getpid(),
		Exe:       exe,
		CWD:       cwd,
		Version:   version,
		StartedAt: time.Now().Format(time.RFC3339),
	}
}

func AcquireStartupLock(lockPath string, info StartupLockInfo) (*StartupLock, error) {
	if lockPath == "" {
		return nil, errors.New("startup lock path is empty")
	}
	file, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			if stale, staleErr := startupLockIsStale(lockPath); stale && staleErr == nil {
				if removeErr := os.Remove(lockPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
					return nil, fmt.Errorf("%w: %s: remove stale lock: %v", ErrStartupLockExists, lockPath, removeErr)
				}
				file, err = os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
				if err == nil {
					goto writeLock
				}
			}
			return nil, fmt.Errorf("%w: %s", ErrStartupLockExists, lockPath)
		}
		return nil, err
	}

writeLock:
	defer file.Close()

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		_ = os.Remove(lockPath)
		return nil, err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = os.Remove(lockPath)
		return nil, err
	}
	return &StartupLock{Path: lockPath}, nil
}

func startupLockIsStale(lockPath string) (bool, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false, err
	}
	var info StartupLockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return false, err
	}
	if info.PID <= 0 || info.PID == os.Getpid() {
		return false, nil
	}
	exists, err := startupLockProcessExists(info.PID)
	if err != nil {
		return false, err
	}
	return !exists, nil
}

func AcquireBinaryDirStartupLock(version string) (*StartupLock, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
		exe = resolved
	}
	lockPath, err := StartupLockPathForExecutable(exe)
	if err != nil {
		return nil, err
	}
	return AcquireStartupLock(lockPath, BuildStartupLockInfo(version))
}

func (l *StartupLock) Release() error {
	if l == nil || l.Path == "" {
		return nil
	}
	err := os.Remove(l.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
